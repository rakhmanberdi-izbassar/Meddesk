package school

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"strings"
)

func (s *Service) Members(ctx context.Context, actor string, limit, offset int32) (Page[db.SchoolMembersRow], error) {
	out := Page[db.SchoolMembersRow]{Items: []db.SchoolMembersRow{}, Limit: limit, Offset: offset}
	if err := pagination(limit, offset); err != nil {
		return out, err
	}
	err := s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var err error
		out.Items, err = q.SchoolMembers(ctx, db.SchoolMembersParams{PageLimit: limit, PageOffset: offset})
		if err != nil {
			return err
		}
		out.Total, err = q.SchoolMemberCount(ctx)
		return err
	})
	return out, err
}
func (s *Service) CreateMember(ctx context.Context, actor string, in MemberInput) (db.SchoolCreateMemberRow, error) {
	var out db.SchoolCreateMemberRow
	email, err := auth.NormalizeEmail(in.Email)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if err != nil || !text(in.DisplayName, 150, false) {
		return out, ErrInvalid
	}
	err = s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			return ErrInvalid
		}
		out, err = q.SchoolCreateMember(ctx, db.SchoolCreateMemberParams{Email: email, DisplayName: in.DisplayName, PasswordHash: hash})
		if err != nil {
			var pgerr *pgconn.PgError
			if errors.As(err, &pgerr) && pgerr.Code == "23505" {
				return ErrDuplicate
			}
			return err
		}
		id, _ := uuid(actor)
		return q.SchoolAccountEvent(ctx, db.SchoolAccountEventParams{ActorID: id, UserID: out.ID, Action: "created"})
	})
	return out, err
}
func (s *Service) UpdateMember(ctx context.Context, actor, id string, in MemberUpdate) (db.SchoolUpdateMemberRow, error) {
	var out db.SchoolUpdateMemberRow
	key, err := uuid(id)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if err != nil || !text(in.DisplayName, 150, false) || in.IsActive == nil || in.Version < 1 {
		return out, ErrInvalid
	}
	err = s.repo.authorized(ctx, actor, func(q *db.Queries) error {
		var hash pgtype.Text
		if in.Password != nil {
			value, err := auth.HashPassword(*in.Password)
			if err != nil {
				return ErrInvalid
			}
			hash = pgtype.Text{String: value, Valid: true}
		}
		var err error
		out, err = q.SchoolUpdateMember(ctx, db.SchoolUpdateMemberParams{ID: key, DisplayName: in.DisplayName, IsActive: *in.IsActive, PasswordHash: hash, ExpectedVersion: in.Version})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		// Reset and deactivation revoke existing sessions in the same transaction.
		if hash.Valid || !*in.IsActive {
			if err = q.RevokeManagedUserSessions(ctx, key); err != nil {
				return err
			}
		}
		actorID, _ := uuid(actor)
		return q.SchoolAccountEvent(ctx, db.SchoolAccountEventParams{ActorID: actorID, UserID: key, Action: "updated"})
	})
	return out, err
}
func (s *Service) MyTickets(ctx context.Context, actor string, limit, offset int32) (Page[db.SchoolMyTicketsRow], error) {
	out := Page[db.SchoolMyTicketsRow]{Items: []db.SchoolMyTicketsRow{}, Limit: limit, Offset: offset}
	if err := pagination(limit, offset); err != nil {
		return out, err
	}
	err := s.repo.member(ctx, actor, func(q *db.Queries, _ string) error {
		id, _ := uuid(actor)
		var err error
		out.Items, err = q.SchoolMyTickets(ctx, db.SchoolMyTicketsParams{ActorID: id, PageLimit: limit, PageOffset: offset})
		if err != nil {
			return err
		}
		out.Total, err = q.SchoolMyTicketCount(ctx, id)
		return err
	})
	return out, err
}
func (s *Service) MyTicket(ctx context.Context, actor, id string) (db.SchoolMyTicketRow, error) {
	var out db.SchoolMyTicketRow
	key, err := uuid(id)
	if err != nil {
		return out, err
	}
	err = s.repo.member(ctx, actor, func(q *db.Queries, _ string) error {
		actorID, _ := uuid(actor)
		var err error
		out, err = q.SchoolMyTicket(ctx, db.SchoolMyTicketParams{ActorID: actorID, TicketID: key})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	return out, err
}
func (s *Service) CreateRequest(ctx context.Context, actor string, in RequestInput) (db.SchoolMyTicketRow, error) {
	var out db.SchoolMyTicketRow
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Room = strings.TrimSpace(in.Room)
	if !text(in.Title, 250, false) || !body(in.Description, 10000, false) || !text(in.Room, 200, false) || !oneOf(in.Category, "COMPUTERS", "PRINTING", "NETWORK", "SURVEILLANCE", "CLASSROOM", "ACCOUNTS", "OTHER") || !oneOf(in.Priority, "LOW", "MEDIUM", "HIGH", "CRITICAL") {
		return out, ErrInvalid
	}
	err := s.repo.member(ctx, actor, func(q *db.Queries, name string) error {
		id, _ := uuid(actor)
		ticket, err := q.SchoolCreateTicket(ctx, db.SchoolCreateTicketParams{Title: in.Title, Description: in.Description, Requester: name, RequesterID: id, Room: in.Room, Category: in.Category, Priority: in.Priority, LessonBlocked: in.LessonBlocked})
		if err != nil {
			return err
		}
		if err = history(ctx, q, actor, ticket.ID, "ticket", "created", ticket.Version); err != nil {
			return err
		}
		out, err = q.SchoolMyTicket(ctx, db.SchoolMyTicketParams{ActorID: id, TicketID: ticket.ID})
		return err
	})
	return out, err
}
