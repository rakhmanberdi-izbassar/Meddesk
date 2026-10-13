package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
)

type Database interface {
	db.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct {
	database Database
	q        *db.Queries
}

func NewRepository(database Database) *Repository { return &Repository{database, db.New(database)} }
func uid(s string) pgtype.UUID                    { var id pgtype.UUID; _ = id.Scan(s); return id }
func uuidString(v pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", v.Bytes[:4], v.Bytes[4:6], v.Bytes[6:8], v.Bytes[8:10], v.Bytes[10:])
}
func optionalUUID(v *string) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return uid(*v)
}
func uuidPointer(v pgtype.UUID) *string {
	if !v.Valid {
		return nil
	}
	s := uuidString(v)
	return &s
}
func timePointer(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
func textPointer(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
func timestamp(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}
func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "23503":
			return ErrConflict
		case "23514", "22001", "22003":
			return ErrInvalidInput
		}
	}
	return err
}
func ticketDTO(row db.TicketDetailsRow) Ticket {
	articles := make([]string, 0, len(row.ArticleIds))
	for _, v := range row.ArticleIds {
		articles = append(articles, uuidString(v))
	}
	attachments := make([]string, 0, len(row.AttachmentIds))
	for _, v := range row.AttachmentIds {
		attachments = append(attachments, uuidString(v))
	}
	return Ticket{ID: uuidString(row.ID), Number: row.Number.Int64, Title: row.Title, Description: row.Description, RequesterID: uuidString(row.RequesterID), RequesterName: row.RequesterName, DepartmentID: uuidString(row.DepartmentID), DepartmentName: row.DepartmentName, CategoryID: uuidPointer(row.CategoryID), Priority: row.Priority, Status: row.Status, AssigneeID: uuidPointer(row.AssigneeID), AssigneeName: row.AssigneeName, Version: row.Version, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, ResolvedAt: timePointer(row.ResolvedAt), ArticleIDs: articles, AttachmentIDs: attachments, ConvertedArticleID: uuidPointer(row.ConvertedArticleID)}
}
func getTicket(ctx context.Context, q *db.Queries, actor auth.User, id string) (Ticket, error) {
	row, err := q.TicketDetails(ctx, db.TicketDetailsParams{ActorID: uid(actor.ID), ID: uid(id)})
	return ticketDTO(row), mapError(err)
}
func (r *Repository) List(ctx context.Context, actor auth.User, f Filter) (Page, error) {
	tx, err := analytics.BeginSnapshot(ctx, r.database)
	if err != nil {
		return Page{}, err
	}
	defer rollbackSnapshot(tx)
	q := db.New(tx)
	start, end := pgtype.Timestamptz{}, pgtype.Timestamptz{}
	if f.From != "" || f.To != "" {
		zone := f.Timezone
		if zone == "" {
			v, err := q.GetApplicationSettings(ctx)
			if err != nil {
				return Page{}, err
			}
			zone = v.ReportingTimezone
		}
		period, err := resolveFilterPeriod(f, zone, time.Now())
		if err != nil {
			return Page{}, err
		}
		start, end = timestamp(&period.StartAt), timestamp(&period.EndAt)
	}
	p := db.TicketListParams{ActorID: uid(actor.ID), Mine: f.Mine, Search: f.Search, Status: f.Status, Priority: f.Priority, AssigneeID: uid(f.AssigneeID), DepartmentID: uid(f.DepartmentID), CategoryID: uid(f.CategoryID), StartAt: start, EndAt: end, PageLimit: int32(f.Limit), PageOffset: int32(f.Offset)}
	ids, err := q.TicketList(ctx, p)
	if err != nil {
		return Page{}, err
	}
	total, err := q.TicketCount(ctx, db.TicketCountParams{ActorID: p.ActorID, Mine: p.Mine, Search: p.Search, Status: p.Status, Priority: p.Priority, AssigneeID: p.AssigneeID, DepartmentID: p.DepartmentID, CategoryID: p.CategoryID, StartAt: start, EndAt: end})
	if err != nil {
		return Page{}, err
	}
	out := Page{Items: make([]Ticket, 0, len(ids)), Total: total, Limit: f.Limit, Offset: f.Offset}
	for _, v := range ids {
		t, err := getTicket(ctx, q, actor, uuidString(v))
		if err != nil {
			return Page{}, err
		}
		out.Items = append(out.Items, t)
	}
	return out, tx.Commit(ctx)
}
func rollbackSnapshot(tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(cleanup)
}
func resolveFilterPeriod(f Filter, zone string, now time.Time) (*analytics.Period, error) {
	if f.From == "" && f.To == "" {
		return nil, nil
	}
	if f.From == "" || f.To == "" {
		return nil, ErrInvalidInput
	}
	period, err := analytics.ResolvePeriod(f.From, f.To, zone, now)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return &period, nil
}

// ExportSnapshot checks current authority, settings, count and bulk-selected
// report fields inside one read-only repeatable-read transaction.
func (r *Repository) ExportSnapshot(ctx context.Context, actor auth.User, f Filter, now time.Time) (ExportSnapshot, error) {
	tx, err := analytics.BeginSnapshot(ctx, r.database)
	if err != nil {
		return ExportSnapshot{}, err
	}
	defer rollbackSnapshot(tx)
	q := db.New(tx)
	current, err := q.TicketExportActor(ctx, uid(actor.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ExportSnapshot{}, ErrForbidden
	}
	if err != nil {
		return ExportSnapshot{}, err
	}
	if !current.IsActive || (current.Role != auth.RoleEngineer && current.Role != auth.RoleAdmin) {
		return ExportSnapshot{}, ErrForbidden
	}
	config, err := q.GetApplicationSettings(ctx)
	if err != nil {
		return ExportSnapshot{}, err
	}
	zone := f.Timezone
	if zone == "" {
		zone = config.ReportingTimezone
	}
	period, err := resolveFilterPeriod(f, zone, now)
	if err != nil {
		return ExportSnapshot{}, err
	}
	start, end := pgtype.Timestamptz{}, pgtype.Timestamptz{}
	if period != nil {
		start, end = timestamp(&period.StartAt), timestamp(&period.EndAt)
	}
	p := db.TicketCountParams{ActorID: uid(actor.ID), Mine: f.Mine, Search: f.Search, Status: f.Status, Priority: f.Priority, AssigneeID: uid(f.AssigneeID), DepartmentID: uid(f.DepartmentID), CategoryID: uid(f.CategoryID), StartAt: start, EndAt: end}
	total, err := q.TicketCount(ctx, p)
	if err != nil {
		return ExportSnapshot{}, err
	}
	if total > MaxExportRows {
		return ExportSnapshot{}, ErrExportLimit
	}
	rows, err := q.TicketExportRows(ctx, db.TicketExportRowsParams{ActorID: p.ActorID, Mine: p.Mine, Search: p.Search, Status: p.Status, Priority: p.Priority, AssigneeID: p.AssigneeID, DepartmentID: p.DepartmentID, CategoryID: p.CategoryID, StartAt: start, EndAt: end, ExportLimit: MaxExportRows + 1})
	if err != nil {
		return ExportSnapshot{}, err
	}
	if int64(len(rows)) != total || len(rows) > MaxExportRows {
		return ExportSnapshot{}, ErrExportLimit
	}
	out := ExportSnapshot{ClinicName: config.ClinicName, Timezone: zone, Period: period, Rows: make([]ExportRow, 0, len(rows))}
	if f.DepartmentID != "" || f.CategoryID != "" || f.AssigneeID != "" {
		labels, err := q.TicketExportFilterLabels(ctx, db.TicketExportFilterLabelsParams{DepartmentID: p.DepartmentID, CategoryID: p.CategoryID, AssigneeID: p.AssigneeID})
		if err != nil {
			return ExportSnapshot{}, err
		}
		out.FilterLabels = ExportFilterLabels{DepartmentName: labels.DepartmentName, CategoryKK: labels.CategoryKk, CategoryRU: labels.CategoryRu, AssigneeName: labels.AssigneeName}
	}
	for _, row := range rows {
		out.Rows = append(out.Rows, ExportRow{Number: row.Number.Int64, Title: row.Title, RequesterName: row.RequesterName, DepartmentName: row.DepartmentName, CategoryKK: row.CategoryKk, CategoryRU: row.CategoryRu, Priority: row.Priority, Status: row.Status, AssigneeName: row.AssigneeName, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, ResolvedAt: timePointer(row.ResolvedAt)})
	}
	return out, tx.Commit(ctx)
}
func (r *Repository) Detail(ctx context.Context, actor auth.User, id string) (Detail, error) {
	t, err := getTicket(ctx, r.q, actor, id)
	if err != nil {
		return Detail{}, err
	}
	comments, err := r.q.TicketComments(ctx, db.TicketCommentsParams{ActorID: uid(actor.ID), TicketID: uid(id)})
	if err != nil {
		return Detail{}, err
	}
	history, err := r.q.TicketHistory(ctx, db.TicketHistoryParams{ActorID: uid(actor.ID), TicketID: uid(id)})
	if err != nil {
		return Detail{}, err
	}
	out := Detail{Ticket: t, Comments: make([]Comment, 0, len(comments)), History: make([]History, 0, len(history))}
	for _, v := range comments {
		out.Comments = append(out.Comments, Comment{ID: uuidString(v.ID), AuthorID: uuidString(v.AuthorID), AuthorName: v.AuthorName, Body: v.Body, IsInternal: v.IsInternal, CreatedAt: v.CreatedAt.Time})
	}
	for _, v := range history {
		out.History = append(out.History, History{ID: v.ID, FromStatus: textPointer(v.FromStatus), ToStatus: v.ToStatus, ActorName: v.ActorName, Reason: v.Reason, CreatedAt: v.CreatedAt.Time})
	}
	return out, nil
}
func (r *Repository) Assignees(ctx context.Context, actor auth.User) ([]Assignee, error) {
	rows, err := r.q.TicketAssignees(ctx, uid(actor.ID))
	if err != nil {
		return nil, err
	}
	out := make([]Assignee, 0, len(rows))
	for _, v := range rows {
		out = append(out, Assignee{ID: uuidString(v.ID), DisplayName: v.DisplayName})
	}
	return out, nil
}
func (r *Repository) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	tx, err := r.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err := fn(&mutationRepository{db.New(tx)}); err != nil {
		return mapError(err)
	}
	return mapError(tx.Commit(ctx))
}

type mutationRepository struct{ q *db.Queries }

func (r *mutationRepository) Lock(ctx context.Context) error { return r.q.LockUserManagement(ctx) }
func (r *mutationRepository) DefaultPriority(ctx context.Context) (string, error) {
	settings, err := r.q.GetApplicationSettings(ctx)
	return settings.DefaultTicketPriority, mapError(err)
}
func (r *mutationRepository) Actor(ctx context.Context, id string) (auth.User, error) {
	v, err := r.q.TicketActorForUpdate(ctx, uid(id))
	return auth.User{ID: uuidString(v.ID), Email: v.Email, DisplayName: v.DisplayName, Role: v.Role, DepartmentID: uuidString(v.DepartmentID), DepartmentName: v.DepartmentName, IsActive: v.IsActive && v.DepartmentActive}, mapError(err)
}
func (r *mutationRepository) Ticket(ctx context.Context, id string) (Ticket, error) {
	v, err := r.q.TicketLock(ctx, uid(id))
	return Ticket{ID: uuidString(v.ID), RequesterID: uuidString(v.RequesterID), Version: v.Version}, mapError(err)
}
func (r *mutationRepository) Get(ctx context.Context, actor auth.User, id string) (Ticket, error) {
	return getTicket(ctx, r.q, actor, id)
}
func (r *mutationRepository) Category(ctx context.Context, id string) (bool, error) {
	v, err := r.q.KnowledgeGetCategory(ctx, uid(id))
	return v.IsActive, mapError(err)
}
func (r *mutationRepository) PublicArticle(ctx context.Context, id string) error {
	_, err := r.q.TicketCheckPublicArticle(ctx, uid(id))
	return mapError(err)
}
func (r *mutationRepository) CheckMedia(ctx context.Context, id, actor string, admin bool) error {
	_, err := r.q.TicketCheckMedia(ctx, db.TicketCheckMediaParams{ID: uid(id), UploaderID: uid(actor), Column3: admin})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	return err
}
func (r *mutationRepository) Create(ctx context.Context, actor auth.User, in CreateInput) (Ticket, error) {
	id, err := r.q.TicketCreate(ctx, db.TicketCreateParams{Title: in.Title, Description: in.Description, RequesterID: uid(actor.ID), DepartmentID: uid(actor.DepartmentID), CategoryID: optionalUUID(in.CategoryID), Priority: in.Priority})
	if err != nil {
		return Ticket{}, mapError(err)
	}
	return getTicket(ctx, r.q, actor, uuidString(id))
}
func (r *mutationRepository) Update(ctx context.Context, id, priority string, assignee *string, status string, resolved *time.Time) error {
	return r.q.TicketUpdate(ctx, db.TicketUpdateParams{ID: uid(id), Priority: priority, AssigneeID: optionalUUID(assignee), Status: status, ResolvedAt: timestamp(resolved)})
}
func (r *mutationRepository) Bump(ctx context.Context, id string) error {
	return r.q.TicketBump(ctx, uid(id))
}
func (r *mutationRepository) LinkArticle(ctx context.Context, id, article string) error {
	return r.q.TicketLinkArticle(ctx, db.TicketLinkArticleParams{TicketID: uid(id), ArticleID: uid(article)})
}
func (r *mutationRepository) LinkAttachment(ctx context.Context, id, media string) error {
	return r.q.TicketLinkAttachment(ctx, db.TicketLinkAttachmentParams{TicketID: uid(id), MediaID: uid(media)})
}
func (r *mutationRepository) Comment(ctx context.Context, id string, actor auth.User, in CommentInput) (Comment, error) {
	v, err := r.q.TicketCreateComment(ctx, db.TicketCreateCommentParams{TicketID: uid(id), AuthorID: uid(actor.ID), Body: in.Body, IsInternal: in.IsInternal})
	return Comment{ID: uuidString(v.ID), AuthorID: uuidString(v.AuthorID), AuthorName: actor.DisplayName, Body: v.Body, IsInternal: v.IsInternal, CreatedAt: v.CreatedAt.Time}, mapError(err)
}
func (r *mutationRepository) History(ctx context.Context, id string, from *string, to, actor, reason string) error {
	var previous pgtype.Text
	if from != nil {
		previous = pgtype.Text{String: *from, Valid: true}
	}
	return r.q.TicketCreateHistory(ctx, db.TicketCreateHistoryParams{TicketID: uid(id), FromStatus: previous, ToStatus: to, ActorID: uid(actor), Reason: reason})
}
func (r *mutationRepository) Audit(ctx context.Context, actor, action, id string, metadata map[string]any) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	kind := "ticket"
	if action == "article.created" {
		kind = "knowledge_article"
	}
	return r.q.InsertManagementAudit(ctx, db.InsertManagementAuditParams{ActorID: uid(actor), Action: action, EntityType: kind, EntityID: uid(id), Metadata: data})
}
func (r *mutationRepository) Convert(ctx context.Context, ticket, actor string, in ArticleInput) (knowledge.Article, error) {
	steps, err := json.Marshal(in.SolutionSteps)
	if err != nil {
		return knowledge.Article{}, err
	}
	id, err := r.q.KnowledgeCreateArticle(ctx, db.KnowledgeCreateArticleParams{Title: in.Title, Slug: in.Slug, Description: in.Description, CategoryID: uid(in.CategoryID), Symptoms: in.Symptoms, SolutionSteps: steps, AuthorID: uid(actor), IsPublished: false})
	if err != nil {
		return knowledge.Article{}, mapError(err)
	}
	for _, tag := range in.Tags {
		tagID, err := r.q.KnowledgeEnsureTag(ctx, tag)
		if err != nil {
			return knowledge.Article{}, err
		}
		if err := r.q.KnowledgeLinkTag(ctx, db.KnowledgeLinkTagParams{ArticleID: id, TagID: tagID}); err != nil {
			return knowledge.Article{}, err
		}
	}
	if err := r.LinkArticle(ctx, ticket, uuidString(id)); err != nil {
		return knowledge.Article{}, err
	}
	if err := r.q.TicketMarkConverted(ctx, db.TicketMarkConvertedParams{ID: uid(ticket), ConvertedArticleID: id}); err != nil {
		return knowledge.Article{}, err
	}
	rows, err := r.q.KnowledgeArticleDetails(ctx, db.KnowledgeArticleDetailsParams{Ids: []pgtype.UUID{id}, PublicOnly: false})
	if err != nil {
		return knowledge.Article{}, err
	}
	if len(rows) != 1 {
		return knowledge.Article{}, ErrNotFound
	}
	v := rows[0]
	return knowledge.Article{ID: uuidString(v.ID), Version: v.Version, Title: v.Title, Slug: v.Slug, Description: v.Description, CategoryID: uuidString(v.CategoryID), CategoryNameKK: v.CategoryNameKk, CategoryNameRU: v.CategoryNameRu, CategoryActive: v.CategoryActive, Symptoms: v.Symptoms, SolutionSteps: in.SolutionSteps, Tags: v.Tags, AuthorID: uuidString(v.AuthorID), AuthorName: v.AuthorName, ImageIDs: []string{}, IsPublished: false, CreatedAt: v.CreatedAt.Time, UpdatedAt: v.UpdatedAt.Time}, nil
}

func (r *Repository) Departments(ctx context.Context, actor auth.User) ([]Department, error) {
	rows, err := r.q.TicketDepartments(ctx, uid(actor.ID))
	if err != nil {
		return nil, err
	}
	out := make([]Department, 0, len(rows))
	for _, v := range rows {
		out = append(out, Department{ID: uuidString(v.ID), Name: v.Name})
	}
	return out, nil
}
