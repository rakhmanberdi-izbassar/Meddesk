// Package seed provisions synthetic development accounts, never production users.
package seed

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meddesk/meddesk/backend/db/seeds"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

type Account struct {
	Email      string
	Name       string
	Role       string
	Department string
	Password   string
}

func Accounts(environment string, get func(string) string) ([]Account, error) {
	if environment != "development" {
		return nil, errors.New("demo seed requires explicit APP_ENV=development")
	}
	accounts := []Account{
		{"employee@meddesk.test", "Демо қызметкер", auth.RoleEmployee, "Демо: қабылдау бөлімі", get("DEMO_EMPLOYEE_PASSWORD")},
		{"engineer@meddesk.test", "Демо IT маманы", auth.RoleEngineer, "Демо: IT бөлімі", get("DEMO_ENGINEER_PASSWORD")},
		{"admin@meddesk.test", "Демо әкімші", auth.RoleAdmin, "Демо: әкімшілік", get("DEMO_ADMIN_PASSWORD")},
	}
	for _, account := range accounts {
		// Do not include the value or an account credential in failure output.
		if len(account.Password) < 12 || len(account.Password) > 72 {
			return nil, errors.New("all demo password variables must contain 12 to 72 UTF-8 bytes")
		}
	}
	return accounts, nil
}

// Run preserves existing accounts and never resets a password, role, or activation state.
// Application settings are managed by migrations and administrators, not this seed.
func Run(ctx context.Context, pool *pgxpool.Pool, environment string, accounts []Account) (int, error) {
	if environment != "development" || len(accounts) != 3 {
		return 0, errors.New("invalid development seed configuration")
	}
	hashes := make([]string, len(accounts))
	for i, account := range accounts {
		hash, err := auth.HashPassword(account.Password)
		if err != nil {
			return 0, errors.New("invalid development password")
		}
		hashes[i] = hash
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	queries := db.New(tx)
	if err := queries.LockUserManagement(ctx); err != nil {
		return 0, err
	}
	// Strip only the trusted file's outer transaction markers: the entire seed is atomic.
	referenceSQL := strings.TrimSpace(seeds.DevelopmentSQL)
	referenceSQL = strings.Replace(referenceSQL, "BEGIN;", "", 1)
	referenceSQL = strings.TrimSuffix(strings.TrimSpace(referenceSQL), "COMMIT;")
	if _, err := tx.Exec(ctx, referenceSQL); err != nil {
		return 0, err
	}
	created := 0
	for i, account := range accounts {
		_, err := queries.AuthFindUserByEmail(ctx, account.Email)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		departments, err := queries.ListManagedDepartments(ctx, db.ListManagedDepartmentsParams{Search: account.Department, PageLimit: 100})
		if err != nil {
			return 0, err
		}
		found := false
		for _, department := range departments {
			if department.Name != account.Department || !department.IsActive {
				continue
			}
			_, err := queries.CreateManagedUser(ctx, db.CreateManagedUserParams{Email: account.Email, DisplayName: account.Name, PasswordHash: hashes[i], Role: account.Role, DepartmentID: department.ID})
			if err != nil {
				return 0, err
			}
			found = true
			created++
			break
		}
		if !found {
			return 0, errors.New("synthetic department unavailable")
		}
	}
	if _, err := tx.Exec(ctx, seeds.KnowledgeSQL); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return created, nil
}
