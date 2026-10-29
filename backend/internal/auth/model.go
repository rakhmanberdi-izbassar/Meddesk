// Package auth owns credential verification, server-side sessions, and HTTP authorization.
package auth

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

const (
	RoleEmployee  = "EMPLOYEE"
	RoleEngineer  = "IT_ENGINEER"
	RoleAdmin     = "ADMIN"
	AccountSchool = "SCHOOL"
	SessionCookie = "meddesk_session"
	CSRFCookie    = "meddesk_csrf"
	CSRFHeader    = "X-CSRF-Token"
	CookiePath    = "/api/v1"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrValidation         = errors.New("invalid authentication input")
)

// User deliberately excludes credentials and session secrets.
type User struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Role           string `json:"role"`
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
	IsActive       bool   `json:"is_active"`
	AccountScope   string `json:"account_scope,omitempty"`
}

type Credential struct {
	User         User
	PasswordHash string
}
type Session struct {
	User      User
	CSRFHash  []byte
	ExpiresAt time.Time
}
type LoginResult struct {
	User      User
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type Repository interface {
	FindUserByEmail(context.Context, string) (Credential, error)
	FindSession(context.Context, []byte) (Session, error)
	CreateSession(context.Context, Credential, []byte, []byte, []byte, time.Time) (User, error)
	RevokeSession(context.Context, []byte) error
}

// TrustedProxyCIDRs only authorizes the immediate peer; X-Forwarded-For is read
// from right to left, stopping at the first untrusted address.
type Options struct {
	AllowedOrigins    []string
	CookieSecure      bool
	SessionTTL        time.Duration
	TrustedProxyCIDRs []netip.Prefix
}

type userContextKey struct{}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userContextKey{}).(User)
	return u, ok
}
