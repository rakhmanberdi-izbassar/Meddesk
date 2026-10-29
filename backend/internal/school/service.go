package school

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"net/netip"
	"strings"
)

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo} }
func (s *Service) Access(ctx context.Context, actor string) (bool, error) {
	return s.repo.Access(ctx, actor)
}
func oneOf(value string, values ...string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}
func text(value string, max int, optional bool) bool {
	return optional && value == "" || httpx.ValidText(value, max)
}
func body(value string, max int, optional bool) bool {
	if optional && value == "" {
		return true
	}
	if strings.TrimSpace(value) == "" || len([]rune(value)) > max {
		return false
	}
	for _, c := range value {
		if c == 0x7f || c < 32 && c != '\n' && c != '\t' && c != '\r' {
			return false
		}
	}
	return strings.ToValidUTF8(value, "") == value
}
func transition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case "NEW":
		return to == "IN_PROGRESS"
	case "IN_PROGRESS":
		return oneOf(to, "WAITING_FOR_USER", "RESOLVED")
	case "WAITING_FOR_USER":
		return oneOf(to, "IN_PROGRESS", "RESOLVED")
	case "RESOLVED":
		return oneOf(to, "CLOSED", "IN_PROGRESS")
	}
	return false
}
func pagination(limit, offset int32) error {
	if limit < 1 || limit > 100 || offset < 0 || offset > 1000000 {
		return ErrInvalid
	}
	return nil
}
func (s *Service) Assets(ctx context.Context, actor, search string, limit, offset int32) (Page[db.SchoolAsset], error) {
	out := Page[db.SchoolAsset]{Items: []db.SchoolAsset{}, Limit: limit, Offset: offset}
	search = strings.TrimSpace(search)
	if search != "" && !text(search, 100, false) {
		return out, ErrInvalid
	}
	if err := pagination(limit, offset); err != nil {
		return out, err
	}
	err := s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var err error
		out.Items, err = q.SchoolAssets(ctx, db.SchoolAssetsParams{Search: search, PageLimit: limit, PageOffset: offset})
		if err != nil {
			return err
		}
		out.Total, err = q.SchoolAssetCount(ctx, search)
		return err
	})
	return out, err
}
func (s *Service) Tickets(ctx context.Context, actor, status, assetFilter string, limit, offset int32) (Page[db.SchoolTicket], error) {
	out := Page[db.SchoolTicket]{Items: []db.SchoolTicket{}, Limit: limit, Offset: offset}
	if assetFilter != "" {
		if _, err := uuid(assetFilter); err != nil {
			return out, ErrInvalid
		}
	}
	if err := pagination(limit, offset); err != nil {
		return out, err
	}
	if status != "" && !oneOf(status, "NEW", "IN_PROGRESS", "WAITING_FOR_USER", "RESOLVED", "CLOSED") {
		return out, ErrInvalid
	}
	err := s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var err error
		out.Items, err = q.SchoolTickets(ctx, db.SchoolTicketsParams{Status: status, AssetFilter: assetFilter, PageLimit: limit, PageOffset: offset})
		if err != nil {
			return err
		}
		out.Total, err = q.SchoolTicketCount(ctx, db.SchoolTicketCountParams{Status: status, AssetFilter: assetFilter})
		return err
	})
	return out, err
}
func history(ctx context.Context, q *db.Queries, actor string, id pgtype.UUID, kind, action string, version int32) error {
	user, _ := uuid(actor)
	return q.SchoolHistory(ctx, db.SchoolHistoryParams{ActorID: user, EntityID: id, EntityType: kind, Action: action, Version: version})
}
func (s *Service) SaveAsset(ctx context.Context, actor, id string, in AssetInput) (db.SchoolAsset, error) {
	var out db.SchoolAsset
	in.Name = strings.TrimSpace(in.Name)
	in.Location = strings.TrimSpace(in.Location)
	in.Model = strings.TrimSpace(in.Model)
	in.InventoryNumber = strings.TrimSpace(in.InventoryNumber)
	in.Notes = strings.TrimSpace(in.Notes)
	if !text(in.Name, 150, false) || !text(in.Location, 200, false) || !text(in.Model, 200, true) || !text(in.InventoryNumber, 100, true) || !body(in.Notes, 3000, true) || !oneOf(in.Kind, "COMPUTER", "PRINTER", "CAMERA", "RECORDER", "ROUTER", "SWITCH", "PROJECTOR", "OTHER") {
		return out, ErrInvalid
	}
	var ip *netip.Addr
	if in.IPAddress != "" {
		v, err := netip.ParseAddr(strings.TrimSpace(in.IPAddress))
		if err != nil || v.Zone() != "" {
			return out, ErrInvalid
		}
		ip = &v
	}
	var key pgtype.UUID
	if id != "" {
		var err error
		key, err = uuid(id)
		if err != nil || in.Version < 1 {
			return out, ErrInvalid
		}
	}
	err := s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var err error
		action := "created"
		if id == "" {
			out, err = q.SchoolCreateAsset(ctx, db.SchoolCreateAssetParams{Name: in.Name, Kind: in.Kind, Location: in.Location, Model: in.Model, InventoryNumber: in.InventoryNumber, IpAddress: ip, Notes: in.Notes})
		} else {
			action = "updated"
			out, err = q.SchoolUpdateAsset(ctx, db.SchoolUpdateAssetParams{ID: key, Name: in.Name, Kind: in.Kind, Location: in.Location, Model: in.Model, InventoryNumber: in.InventoryNumber, IpAddress: ip, Notes: in.Notes, ExpectedVersion: in.Version})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		return history(ctx, q, actor, out.ID, "asset", action, out.Version)
	})
	return out, err
}
func (s *Service) CreateTicket(ctx context.Context, actor string, in TicketInput) (db.SchoolTicket, error) {
	var out db.SchoolTicket
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Requester = strings.TrimSpace(in.Requester)
	in.Room = strings.TrimSpace(in.Room)
	if !text(in.Title, 250, false) || !body(in.Description, 10000, false) || !text(in.Requester, 150, false) || !text(in.Room, 200, false) || !oneOf(in.Category, "COMPUTERS", "PRINTING", "NETWORK", "SURVEILLANCE", "CLASSROOM", "ACCOUNTS", "OTHER") || !oneOf(in.Priority, "LOW", "MEDIUM", "HIGH", "CRITICAL") {
		return out, ErrInvalid
	}
	var asset pgtype.UUID
	if in.AssetID != "" {
		var err error
		asset, err = uuid(in.AssetID)
		if err != nil {
			return out, err
		}
	}
	err := s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var err error
		out, err = q.SchoolCreateTicket(ctx, db.SchoolCreateTicketParams{Title: in.Title, Description: in.Description, Requester: in.Requester, Room: in.Room, Category: in.Category, Priority: in.Priority, LessonBlocked: in.LessonBlocked, AssetID: asset})
		if err != nil {
			return err
		}
		return history(ctx, q, actor, out.ID, "ticket", "created", out.Version)
	})
	return out, err
}
func (s *Service) UpdateTicket(ctx context.Context, actor, id string, in UpdateInput) (db.SchoolTicket, error) {
	var out db.SchoolTicket
	key, err := uuid(id)
	if err != nil {
		return out, err
	}
	in.Resolution = strings.TrimSpace(in.Resolution)
	in.PublicReply = strings.TrimSpace(in.PublicReply)
	if in.Version < 1 || !oneOf(in.Status, "NEW", "IN_PROGRESS", "WAITING_FOR_USER", "RESOLVED", "CLOSED") || !oneOf(in.Priority, "LOW", "MEDIUM", "HIGH", "CRITICAL") || !body(in.PublicReply, 5000, true) || !body(in.Resolution, 5000, true) {
		return out, ErrInvalid
	}
	err = s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		current, err := q.SchoolTicketForUpdate(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current.Version != in.Version {
			return ErrConflict
		}
		if current.Status == "CLOSED" || !transition(current.Status, in.Status) || oneOf(in.Status, "RESOLVED", "CLOSED") && in.Resolution == "" {
			return ErrInvalid
		}
		out, err = q.SchoolUpdateTicket(ctx, db.SchoolUpdateTicketParams{ID: key, Status: in.Status, Priority: in.Priority, Resolution: in.Resolution, PublicReply: in.PublicReply, ExpectedVersion: in.Version})
		if err != nil {
			return err
		}
		return history(ctx, q, actor, out.ID, "ticket", "updated", out.Version)
	})
	return out, err
}

func (s *Service) Asset(ctx context.Context, actor, id string) (db.SchoolAsset, error) {
	var out db.SchoolAsset
	key, err := uuid(id)
	if err != nil {
		return out, err
	}
	err = s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var err error
		out, err = q.SchoolGetAsset(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	return out, err
}
