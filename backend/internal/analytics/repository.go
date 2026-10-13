package analytics

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/platform/db"
	"time"
)

type Database interface {
	db.DBTX
	Begin(context.Context) (pgx.Tx, error)
}
type Repository struct{ database Database }

func NewRepository(d Database) *Repository { return &Repository{d} }

// BeginSnapshot starts a read-only, repeatable-read transaction on production
// pools. A pgx.Tx caller supplies the parent isolation for its nested savepoint.
func BeginSnapshot(ctx context.Context, d Database) (pgx.Tx, error) {
	if db, ok := d.(interface {
		BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	}); ok {
		return db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	}
	return d.Begin(ctx)
}
func uid(s string) pgtype.UUID { var id pgtype.UUID; _ = id.Scan(s); return id }
func uuid(v pgtype.UUID) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", v.Bytes[:4], v.Bytes[4:6], v.Bytes[6:8], v.Bytes[8:10], v.Bytes[10:])
}
func uuidPointer(v pgtype.UUID) *string {
	if !v.Valid {
		return nil
	}
	s := uuid(v)
	return &s
}
func authorize(ctx context.Context, q *db.Queries, u auth.User) error {
	a, err := q.ReportingActor(ctx, uid(u.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if !a.IsActive || (a.Role != auth.RoleEngineer && a.Role != auth.RoleAdmin) {
		return ErrForbidden
	}
	return nil
}
func (r *Repository) Report(ctx context.Context, u auth.User, f Filter, now time.Time) (Report, error) {
	tx, err := BeginSnapshot(ctx, r.database)
	if err != nil {
		return Report{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err := authorize(ctx, q, u); err != nil {
		return Report{}, err
	}
	zone := f.Timezone
	if zone == "" {
		v, err := q.GetApplicationSettings(ctx)
		if err != nil {
			return Report{}, err
		}
		zone = v.ReportingTimezone
	}
	period, err := ResolvePeriod(f.From, f.To, zone, now)
	if err != nil {
		return Report{}, err
	}
	start, end := pgtype.Timestamptz{Time: period.StartAt, Valid: true}, pgtype.Timestamptz{Time: period.EndAt, Valid: true}
	dept, category, assignee := uid(f.DepartmentID), uid(f.CategoryID), uid(f.AssigneeID)
	out := Report{Period: period, Departments: []DepartmentCount{}, Categories: []CategoryCount{}, CommonIssues: []CategoryCount{}, PopularArticles: []Article{}, Trend: []Trend{}}
	ts, err := q.AnalyticsTicketSummary(ctx, db.AnalyticsTicketSummaryParams{StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category, AssigneeID: assignee})
	if err != nil {
		return Report{}, err
	}
	out.Summary = Summary{TicketsTotal: ts.Total, TicketsOpen: ts.Open, TicketsResolved: ts.Resolved, TicketsClosed: ts.Closed}
	if ts.Resolved > 0 {
		out.Summary.AverageResolutionSeconds = &ts.AverageResolutionSeconds
	}
	fs, err := q.AnalyticsFeedbackSummary(ctx, db.AnalyticsFeedbackSummaryParams{StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category})
	if err != nil {
		return Report{}, err
	}
	out.Summary.FeedbackAttempts = fs.Attempts
	out.Summary.HelpfulAttempts = fs.Helpful
	if fs.Attempts > 0 {
		rate := float64(fs.Helpful) / float64(fs.Attempts)
		out.Summary.SelfServiceRate = &rate
	}
	ds, err := q.AnalyticsDepartments(ctx, db.AnalyticsDepartmentsParams{StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category, AssigneeID: assignee})
	if err != nil {
		return Report{}, err
	}
	for _, v := range ds {
		out.Departments = append(out.Departments, DepartmentCount{Department{uuid(v.ID), v.Name}, v.Total, v.Open, v.Resolved})
	}
	cs, err := q.AnalyticsCategories(ctx, db.AnalyticsCategoriesParams{StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category, AssigneeID: assignee})
	if err != nil {
		return Report{}, err
	}
	for _, v := range cs {
		out.Categories = append(out.Categories, CategoryCount{uuidPointer(v.ID), v.NameKk, v.NameRu, v.Total, v.Open, v.Resolved})
	}
	n := len(out.Categories)
	if n > 10 {
		n = 10
	}
	out.CommonIssues = append(out.CommonIssues, out.Categories[:n]...)
	ps, err := q.AnalyticsPopularArticles(ctx, db.AnalyticsPopularArticlesParams{StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category})
	if err != nil {
		return Report{}, err
	}
	for _, v := range ps {
		out.PopularArticles = append(out.PopularArticles, Article{uuid(v.ID), v.Title, v.ViewCount, v.HelpfulCount, v.NotHelpfulCount})
	}
	tt, err := q.AnalyticsTicketTrend(ctx, db.AnalyticsTicketTrendParams{Timezone: zone, StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category, AssigneeID: assignee})
	if err != nil {
		return Report{}, err
	}
	ft, err := q.AnalyticsFeedbackTrend(ctx, db.AnalyticsFeedbackTrendParams{Timezone: zone, StartAt: start, EndAt: end, DepartmentID: dept, CategoryID: category})
	if err != nil {
		return Report{}, err
	}
	byDate := map[string]Trend{}
	for _, v := range tt {
		byDate[v.Day] = Trend{Date: v.Day, TicketsCreated: v.Created, TicketsResolved: v.Resolved}
	}
	for _, v := range ft {
		day := byDate[v.Day]
		day.Date = v.Day
		day.FeedbackAttempts = v.Attempts
		day.HelpfulAttempts = v.Helpful
		byDate[v.Day] = day
	}
	loc, _ := time.LoadLocation(zone)
	for day := period.StartAt.In(loc); day.Before(period.EndAt); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		point := byDate[date]
		point.Date = date
		out.Trend = append(out.Trend, point)
	}
	return out, tx.Commit(ctx)
}
func (r *Repository) Filters(ctx context.Context, u auth.User) (Filters, error) {
	tx, err := BeginSnapshot(ctx, r.database)
	if err != nil {
		return Filters{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	q := db.New(tx)
	if err := authorize(ctx, q, u); err != nil {
		return Filters{}, err
	}
	out := Filters{Departments: []Department{}, Categories: []Category{}, Assignees: []Assignee{}}
	ds, err := q.AnalyticsFilterDepartments(ctx)
	if err != nil {
		return Filters{}, err
	}
	for _, v := range ds {
		out.Departments = append(out.Departments, Department{uuid(v.ID), v.Name})
	}
	cs, err := q.AnalyticsFilterCategories(ctx)
	if err != nil {
		return Filters{}, err
	}
	for _, v := range cs {
		out.Categories = append(out.Categories, Category{uuid(v.ID), v.NameKk, v.NameRu})
	}
	as, err := q.AnalyticsFilterAssignees(ctx)
	if err != nil {
		return Filters{}, err
	}
	for _, v := range as {
		out.Assignees = append(out.Assignees, Assignee{uuid(v.ID), v.DisplayName})
	}
	return out, tx.Commit(ctx)
}
