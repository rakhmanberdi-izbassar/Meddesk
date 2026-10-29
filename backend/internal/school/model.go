// Package school owns a private workspace, isolated from clinic records and roles.
package school

import "errors"

var ErrForbidden = errors.New("school access denied")
var ErrInvalid = errors.New("invalid school input")
var ErrDuplicate = errors.New("school account email already used")
var ErrConflict = errors.New("school record changed")
var ErrNotFound = errors.New("school record not found")

type AssetInput struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Location        string `json:"location"`
	Model           string `json:"model"`
	InventoryNumber string `json:"inventory_number"`
	IPAddress       string `json:"ip_address"`
	Notes           string `json:"notes"`
	Version         int32  `json:"version"`
}
type TicketInput struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Requester     string `json:"requester"`
	Room          string `json:"room"`
	Category      string `json:"category"`
	Priority      string `json:"priority"`
	LessonBlocked bool   `json:"lesson_blocked"`
	AssetID       string `json:"asset_id"`
}
type UpdateInput struct {
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	Resolution  string `json:"resolution"`
	PublicReply string `json:"public_reply"`
	Version     int32  `json:"version"`
}
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

type MemberInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}
type MemberUpdate struct {
	DisplayName string  `json:"display_name"`
	IsActive    *bool   `json:"is_active"`
	Password    *string `json:"password,omitempty"`
	Version     int32   `json:"version"`
}
type RequestInput struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Room          string `json:"room"`
	Category      string `json:"category"`
	Priority      string `json:"priority"`
	LessonBlocked bool   `json:"lesson_blocked"`
}
