// Package bootstrap provisions the first administrator through an operator-only CLI.
package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
)

var (
	ErrInvalidInput       = errors.New("valid email, names and a 12–72 byte UTF-8 password are required")
	ErrNotEmpty           = errors.New("administrator bootstrap refused: users already exist")
	ErrInactiveDepartment = errors.New("administrator bootstrap refused: department is inactive")
	ErrStorage            = errors.New("administrator bootstrap failed; no credentials were logged; check stored state before retrying")
)

type Input struct {
	Email, DisplayName, Department, Password string
}

type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func normalize(input Input) (Input, error) {
	email, err := auth.NormalizeEmail(input.Email)
	input.Email = email
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Department = strings.TrimSpace(input.Department)
	if err != nil || !httpx.ValidText(input.DisplayName, 150) || !httpx.ValidText(input.Department, 150) || !utf8.ValidString(input.Password) || len(input.Password) < 12 || len(input.Password) > 72 {
		return Input{}, ErrInvalidInput
	}
	return input, nil
}

// Run never resets or promotes an existing user. The shared management lock and
// table lock make the empty-users check atomic even against concurrent inserts.
func Run(ctx context.Context, pool Beginner, input Input) error {
	input, err := normalize(input)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return ErrInvalidInput
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ErrStorage
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err := q.LockUserManagement(ctx); err != nil {
		return ErrStorage
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return ErrStorage
	}
	count, err := q.CountManagedUsers(ctx, db.CountManagedUsersParams{})
	if err != nil {
		return ErrStorage
	}
	if count != 0 {
		return ErrNotEmpty
	}

	var departmentID pgtype.UUID
	var active bool
	createdDepartment := false
	err = tx.QueryRow(ctx, `SELECT id, is_active FROM departments WHERE lower(btrim(name))=lower($1) FOR UPDATE`, input.Department).Scan(&departmentID, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		department, createErr := q.CreateManagedDepartment(ctx, input.Department)
		if createErr != nil {
			return ErrStorage
		}
		departmentID, active = department.ID, department.IsActive
		createdDepartment = true
	} else if err != nil {
		return ErrStorage
	}
	if !active {
		return ErrInactiveDepartment
	}
	userID, err := q.CreateManagedUser(ctx, db.CreateManagedUserParams{
		Email: input.Email, DisplayName: input.DisplayName, PasswordHash: hash,
		Role: auth.RoleAdmin, DepartmentID: departmentID,
	})
	if err != nil {
		return ErrStorage
	}
	metadata, err := json.Marshal(map[string]any{"role": auth.RoleAdmin, "department_id": departmentID})
	if err != nil {
		return ErrStorage
	}
	if err := q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{
		ActorID: userID, Action: "user.created", EntityType: "user", EntityID: userID, Metadata: metadata,
	}); err != nil {
		return ErrStorage
	}
	if createdDepartment {
		if err := q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{
			ActorID: userID, Action: "department.created", EntityType: "department", EntityID: departmentID, Metadata: []byte(`{"is_active":true}`),
		}); err != nil {
			return ErrStorage
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ErrStorage
	}
	return nil
}
