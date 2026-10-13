package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const testPassword = "synthetic-password"

type memoryRepository struct {
	credential               Credential
	sessions                 map[string]Session
	credentialErr, createErr error
	oldHash                  []byte
}

func (m *memoryRepository) FindUserByEmail(_ context.Context, email string) (Credential, error) {
	if m.credentialErr != nil {
		return Credential{}, m.credentialErr
	}
	if email != m.credential.User.Email {
		return Credential{}, ErrUnauthenticated
	}
	return m.credential, nil
}
func (m *memoryRepository) FindSession(_ context.Context, hash []byte) (Session, error) {
	session, ok := m.sessions[string(hash)]
	if !ok {
		return Session{}, ErrUnauthenticated
	}
	session.User = m.credential.User
	return session, nil
}
func (m *memoryRepository) CreateSession(_ context.Context, credential Credential, hash, csrf, old []byte, expires time.Time) (User, error) {
	if m.createErr != nil {
		return User{}, m.createErr
	}
	m.oldHash = old
	delete(m.sessions, string(old))
	m.sessions[string(hash)] = Session{User: credential.User, CSRFHash: csrf, ExpiresAt: expires}
	return m.credential.User, nil
}
func (m *memoryRepository) RevokeSession(_ context.Context, hash []byte) error {
	if _, ok := m.sessions[string(hash)]; !ok {
		return ErrUnauthenticated
	}
	delete(m.sessions, string(hash))
	return nil
}

func fixtureService(t *testing.T) (*Service, *memoryRepository) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryRepository{credential: Credential{User: User{ID: "00000000-0000-4000-8000-000000000001", Email: "employee@example.invalid", DisplayName: "Synthetic user", Role: RoleEmployee, DepartmentID: "00000000-0000-4000-8000-000000000002", DepartmentName: "Synthetic department", IsActive: true}, PasswordHash: string(hash)}, sessions: map[string]Session{}}
	return &Service{repo: repo, ttl: 8 * time.Hour, now: time.Now, dummyHash: hash}, repo
}

func TestHashPasswordUTF8LimitsAndCost(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("a", 73), strings.Repeat("қ", 37), "invalid-utf8-\xff"} {
		if _, err := HashPassword(password); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected validation for %d bytes", len(password))
		}
	}
	hash, err := HashPassword(strings.Repeat("қ", 36))
	if err != nil {
		t.Fatal(err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != 12 {
		t.Fatalf("bcrypt cost=%d,error=%v", cost, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(strings.Repeat("қ", 36))); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	email, err := NormalizeEmail("  Employee@Example.invalid  ")
	if err != nil || email != "employee@example.invalid" {
		t.Fatalf("normalization failed: %q,%v", email, err)
	}
	for _, input := range []string{"", "missing-at", "user@localhost", "Name <user@example.invalid>", "user @example.invalid", "a@b@example.invalid", "user@.invalid", "a\xff@example.invalid"} {
		if _, err := NormalizeEmail(input); !errors.Is(err, ErrValidation) {
			t.Fatalf("accepted invalid email %q", input)
		}
	}
}

func TestLoginHashedTokensRotationAndLogout(t *testing.T) {
	s, repo := fixtureService(t)
	first, err := s.Login(context.Background(), " Employee@Example.invalid ", testPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Token) != 43 || len(first.CSRFToken) != 43 || first.Token == first.CSRFToken {
		t.Fatal("session tokens must be independent 256-bit values")
	}
	hash := sha256.Sum256([]byte(first.Token))
	stored, ok := repo.sessions[string(hash[:])]
	csrfHash := sha256.Sum256([]byte(first.CSRFToken))
	if !ok || string(stored.CSRFHash) != string(csrfHash[:]) {
		t.Fatal("only expected token hashes should be persisted")
	}
	repo.credential.User.Role = RoleEngineer
	second, err := s.Login(context.Background(), repo.credential.User.Email, testPassword, first.Token)
	if err != nil || second.User.Role != RoleEngineer {
		t.Fatalf("fresh user metadata not returned: %v", err)
	}
	if _, err := s.Authenticate(context.Background(), first.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("rotation did not revoke old token")
	}
	if _, err := s.Authenticate(context.Background(), second.Token); err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(context.Background(), second.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(context.Background(), second.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("logout did not revoke token")
	}
}

func TestLoginCredentialFailures(t *testing.T) {
	for _, tc := range []struct {
		name, email, password string
		inactive              bool
		failure               error
		expected              error
	}{
		{"wrong password", "employee@example.invalid", "incorrect-password", false, nil, ErrInvalidCredentials},
		{"missing account", "missing@example.invalid", testPassword, false, nil, ErrInvalidCredentials},
		{"inactive", "employee@example.invalid", testPassword, true, nil, ErrInvalidCredentials},
		{"short password", "employee@example.invalid", "short", false, nil, ErrValidation},
		{"long password", "employee@example.invalid", strings.Repeat("a", 73), false, nil, ErrValidation},
		{"repository error", "employee@example.invalid", testPassword, false, errors.New("db unavailable"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo := fixtureService(t)
			repo.credential.User.IsActive = !tc.inactive
			repo.credentialErr = tc.failure
			_, err := s.Login(context.Background(), tc.email, tc.password, "")
			if tc.expected != nil && !errors.Is(err, tc.expected) {
				t.Fatalf("expected %v got %v", tc.expected, err)
			}
			if tc.failure != nil && !errors.Is(err, tc.failure) {
				t.Fatal(err)
			}
			if len(repo.sessions) != 0 {
				t.Fatal("failed login created a session")
			}
		})
	}
}

func TestAuthenticateExpiryDeactivationAndFreshRole(t *testing.T) {
	s, repo := fixtureService(t)
	login, err := s.Login(context.Background(), repo.credential.User.Email, testPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "arbitrary", strings.Repeat("a", 44), strings.Repeat("!", 43)} {
		if _, err := s.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("malformed token accepted: %q", token)
		}
	}
	repo.credential.User.Role = RoleAdmin
	session, err := s.Authenticate(context.Background(), login.Token)
	if err != nil || session.User.Role != RoleAdmin {
		t.Fatal("role not refreshed")
	}
	repo.credential.User.IsActive = false
	if _, err := s.Authenticate(context.Background(), login.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("deactivated user accepted")
	}
	repo.credential.User.IsActive = true
	s.now = func() time.Time { return login.ExpiresAt }
	if _, err := s.Authenticate(context.Background(), login.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired session accepted")
	}
}

func TestLoginRechecksCredentialStateBeforeSessionCreation(t *testing.T) {
	s, repo := fixtureService(t)
	repo.createErr = ErrInvalidCredentials
	if _, err := s.Login(context.Background(), repo.credential.User.Email, testPassword, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("concurrent account mutation must reject login")
	}
	if len(repo.sessions) != 0 {
		t.Fatal("rejected login created a session")
	}
}
