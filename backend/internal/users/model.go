// Package users manages employee accounts and authenticated profile changes.
package users

import (
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
)

var (
	ErrInvalidInput       = errors.New("invalid user input")
	ErrForbidden          = errors.New("user operation forbidden")
	ErrNotFound           = errors.New("user or department not found")
	ErrConflict           = errors.New("email already exists")
	ErrInactiveDepartment = errors.New("active users require an active department")
	ErrLastAdministrator  = errors.New("the last active administrator must remain active")
)

type Filter struct {
	Search, Role  string
	Active        *bool
	Limit, Offset int
}
type Page struct {
	Items  []auth.User `json:"items"`
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
type CreateInput struct {
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	DepartmentID string `json:"department_id"`
}
type UpdateInput struct {
	Email        string  `json:"email"`
	DisplayName  string  `json:"display_name"`
	Role         string  `json:"role"`
	DepartmentID string  `json:"department_id"`
	IsActive     *bool   `json:"is_active"`
	Password     *string `json:"password,omitempty"`
}
type ProfileInput struct {
	DisplayName string `json:"display_name"`
}

type AccountMutation struct {
	Email, DisplayName, Role, DepartmentID string
	IsActive                               bool
	PasswordHash                           *string
}
