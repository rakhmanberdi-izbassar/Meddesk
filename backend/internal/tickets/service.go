package tickets

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/meddesk/meddesk/backend/internal/analytics"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
)

type Service struct {
	repo      Store
	knowledge KnowledgeService
}

func NewService(repo Store, knowledgeService KnowledgeService) *Service {
	return &Service{repo, knowledgeService}
}
func active(u auth.User) bool {
	return u.IsActive && (u.Role == auth.RoleEmployee || u.Role == auth.RoleEngineer || u.Role == auth.RoleAdmin)
}
func staff(u auth.User) bool {
	return active(u) && (u.Role == auth.RoleEngineer || u.Role == auth.RoleAdmin)
}
func validPriority(v string) bool {
	return v == "LOW" || v == "MEDIUM" || v == "HIGH" || v == "CRITICAL"
}
func validStatus(v string) bool {
	return v == New || v == InProgress || v == Waiting || v == Resolved || v == Closed
}
func validBody(v string, max int, empty bool) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max || (!empty && strings.TrimSpace(v) == "") {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func ids(values []string, max int) ([]string, error) {
	if len(values) > max {
		return nil, ErrInvalidInput
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.ToLower(v)
		if !httpx.ValidUUID(v) || seen[v] {
			return nil, ErrInvalidInput
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}
func normalizeCreate(in CreateInput) (CreateInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if in.Priority == "" {
		in.Priority = "MEDIUM"
	}
	if !httpx.ValidText(in.Title, 250) || !validBody(in.Description, 20000, false) || !validPriority(in.Priority) {
		return in, ErrInvalidInput
	}
	if in.CategoryID != nil {
		v := strings.ToLower(*in.CategoryID)
		if !httpx.ValidUUID(v) {
			return in, ErrInvalidInput
		}
		in.CategoryID = &v
	}
	var err error
	in.ArticleIDs, err = ids(in.ArticleIDs, 10)
	if err != nil {
		return in, err
	}
	in.AttachmentIDs, err = ids(in.AttachmentIDs, 10)
	return in, err
}
func validPage(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 100000
}
func (s *Service) List(ctx context.Context, actor auth.User, f Filter) (Page, error) {
	if !active(actor) {
		return Page{}, ErrForbidden
	}
	if !validPage(f.Limit, f.Offset) {
		return Page{}, ErrInvalidInput
	}
	f, err := normalizeFilter(f)
	if err != nil {
		return Page{}, err
	}
	return s.repo.List(ctx, actor, f)
}
func normalizeFilter(f Filter) (Filter, error) {
	f.Search = strings.TrimSpace(f.Search)
	if (f.Search != "" && !httpx.ValidText(f.Search, 100)) || (f.Status != "" && !validStatus(f.Status)) || (f.Priority != "" && !validPriority(f.Priority)) {
		return f, ErrInvalidInput
	}
	for _, v := range []string{f.AssigneeID, f.DepartmentID, f.CategoryID} {
		if v != "" && !httpx.ValidUUID(v) {
			return f, ErrInvalidInput
		}
	}
	// Ticket reports keep their historical all-time default. Date ranges are
	// explicit pairs rather than the analytics dashboard's rolling defaults.
	if (f.From == "") != (f.To == "") {
		return f, ErrInvalidInput
	}
	if err := analytics.ValidateDates(f.From, f.To, f.Timezone, time.Now()); err != nil {
		return f, ErrInvalidInput
	}
	return f, nil
}
func (s *Service) Get(ctx context.Context, actor auth.User, id string) (Detail, error) {
	if !active(actor) {
		return Detail{}, ErrForbidden
	}
	if !httpx.ValidUUID(id) {
		return Detail{}, ErrInvalidInput
	}
	return s.repo.Detail(ctx, actor, id)
}
func (s *Service) Assignees(ctx context.Context, actor auth.User) ([]Assignee, error) {
	if !staff(actor) {
		return nil, ErrForbidden
	}
	return s.repo.Assignees(ctx, actor)
}
func (s *Service) Suggestions(ctx context.Context, actor auth.User, q, categoryID string) (knowledge.ArticlePage, error) {
	if !active(actor) {
		return knowledge.ArticlePage{}, ErrForbidden
	}
	q = strings.TrimSpace(q)
	if (q != "" && !httpx.ValidText(q, 100)) || (categoryID != "" && !httpx.ValidUUID(categoryID)) {
		return knowledge.ArticlePage{}, ErrInvalidInput
	}
	yes := true
	return s.knowledge.List(ctx, actor, knowledge.Filter{Search: q, CategoryID: categoryID, PublicOnly: true, Published: &yes, Limit: 5})
}
func authorize(ctx context.Context, tx MutationStore, actor auth.User) (auth.User, error) {
	if err := tx.Lock(ctx); err != nil {
		return auth.User{}, err
	}
	u, err := tx.Actor(ctx, actor.ID)
	if err != nil {
		return auth.User{}, err
	}
	if !active(u) {
		return auth.User{}, ErrForbidden
	}
	return u, nil
}
func (s *Service) Create(ctx context.Context, actor auth.User, in CreateInput) (Ticket, error) {
	if !active(actor) {
		return Ticket{}, ErrForbidden
	}
	useDefaultPriority := in.Priority == ""
	in, err := normalizeCreate(in)
	if err != nil {
		return Ticket{}, err
	}
	var out Ticket
	err = s.repo.Transaction(ctx, func(tx MutationStore) error {
		u, err := authorize(ctx, tx, actor)
		if err != nil {
			return err
		}
		if useDefaultPriority {
			in.Priority, err = tx.DefaultPriority(ctx)
			if err != nil {
				return err
			}
			if !validPriority(in.Priority) {
				return ErrInvalidInput
			}
		}
		if in.CategoryID != nil {
			ok, err := tx.Category(ctx, *in.CategoryID)
			if err != nil {
				return err
			}
			if !ok {
				return ErrInactiveCategory
			}
		}
		for _, id := range in.ArticleIDs {
			if err := tx.PublicArticle(ctx, id); err != nil {
				return err
			}
		}
		for _, id := range in.AttachmentIDs {
			if err := tx.CheckMedia(ctx, id, u.ID, u.Role == auth.RoleAdmin); err != nil {
				return err
			}
		}
		out, err = tx.Create(ctx, u, in)
		if err != nil {
			return err
		}
		for _, id := range in.ArticleIDs {
			if err := tx.LinkArticle(ctx, out.ID, id); err != nil {
				return err
			}
		}
		for _, id := range in.AttachmentIDs {
			if err := tx.LinkAttachment(ctx, out.ID, id); err != nil {
				return err
			}
		}
		if err := tx.History(ctx, out.ID, nil, New, u.ID, ""); err != nil {
			return err
		}
		if err := tx.Audit(ctx, u.ID, "ticket.created", out.ID, map[string]any{"version": out.Version, "priority": out.Priority, "status": out.Status}); err != nil {
			return err
		}
		out, err = tx.Get(ctx, u, out.ID)
		return err
	})
	return out, err
}
func (s *Service) mutate(ctx context.Context, actor auth.User, id string, version int32, staffOnly bool, fn func(MutationStore, auth.User, Ticket) error) (Ticket, error) {
	if !active(actor) || (staffOnly && !staff(actor)) {
		return Ticket{}, ErrForbidden
	}
	if !httpx.ValidUUID(id) || version < 1 {
		return Ticket{}, ErrInvalidInput
	}
	var out Ticket
	err := s.repo.Transaction(ctx, func(tx MutationStore) error {
		u, err := authorize(ctx, tx, actor)
		if err != nil {
			return err
		}
		if staffOnly && !staff(u) {
			return ErrForbidden
		}
		locked, err := tx.Ticket(ctx, id)
		if err != nil {
			return err
		}
		if !staff(u) && locked.RequesterID != u.ID {
			return ErrNotFound
		}
		if locked.Version != version || locked.Version == 2147483647 {
			return ErrVersionConflict
		}
		out, err = tx.Get(ctx, u, id)
		if err != nil {
			return err
		}
		if err := fn(tx, u, out); err != nil {
			return err
		}
		out, err = tx.Get(ctx, u, id)
		return err
	})
	return out, err
}
func allowedTransition(from, to string) bool {
	switch from {
	case New:
		return to == InProgress
	case InProgress:
		return to == Waiting || to == Resolved
	case Waiting:
		return to == InProgress || to == Resolved
	case Resolved:
		return to == InProgress || to == Closed
	}
	return false
}
func (s *Service) Update(ctx context.Context, actor auth.User, id string, in UpdateInput) (Ticket, error) {
	if in.Priority == nil && !in.AssigneeID.Present && in.Status == nil {
		return Ticket{}, ErrInvalidInput
	}
	if (in.Priority != nil && !validPriority(*in.Priority)) || (in.Status != nil && !validStatus(*in.Status)) || (in.AssigneeID.Value != nil && !httpx.ValidUUID(*in.AssigneeID.Value)) || (in.Reason != nil && !validBody(*in.Reason, 2000, true)) {
		return Ticket{}, ErrInvalidInput
	}
	return s.mutate(ctx, actor, id, in.Version, true, func(tx MutationStore, u auth.User, t Ticket) error {
		if t.Status == Closed {
			return ErrClosed
		}
		priority, assignee, status, resolved := t.Priority, t.AssigneeID, t.Status, t.ResolvedAt
		if in.Priority != nil {
			priority = *in.Priority
		}
		if in.AssigneeID.Present {
			assignee = in.AssigneeID.Value
			if assignee != nil {
				a, err := tx.Actor(ctx, *assignee)
				if err != nil {
					return err
				}
				if !staff(a) {
					return ErrForbidden
				}
			}
		}
		reason := ""
		if in.Reason != nil {
			reason = strings.TrimSpace(*in.Reason)
		}
		if in.Status != nil && *in.Status != status {
			if !allowedTransition(status, *in.Status) {
				return ErrTransition
			}
			status = *in.Status
			if status == Resolved {
				now := time.Now().UTC()
				resolved = &now
			} else if status != Closed {
				resolved = nil
			}
			if err := tx.History(ctx, id, &t.Status, status, u.ID, reason); err != nil {
				return err
			}
		}
		if err := tx.Update(ctx, id, priority, assignee, status, resolved); err != nil {
			return err
		}
		return tx.Audit(ctx, u.ID, "ticket.updated", id, map[string]any{"version": t.Version + 1, "status": status, "priority": priority, "assignee_id": assignee})
	})
}
func (s *Service) Comment(ctx context.Context, actor auth.User, id string, in CommentInput) (CommentResult, error) {
	in.Body = strings.TrimSpace(in.Body)
	if !validBody(in.Body, 10000, false) {
		return CommentResult{}, ErrInvalidInput
	}
	if in.IsInternal && !staff(actor) {
		return CommentResult{}, ErrForbidden
	}
	var comment Comment
	t, err := s.mutate(ctx, actor, id, in.Version, false, func(tx MutationStore, u auth.User, t Ticket) error {
		if t.Status == Closed {
			return ErrClosed
		}
		if in.IsInternal && !staff(u) {
			return ErrForbidden
		}
		var err error
		comment, err = tx.Comment(ctx, id, u, in)
		if err != nil {
			return err
		}
		if err := tx.Bump(ctx, id); err != nil {
			return err
		}
		return tx.Audit(ctx, u.ID, "ticket.commented", id, map[string]any{"version": t.Version + 1, "comment_id": comment.ID, "is_internal": in.IsInternal})
	})
	return CommentResult{Ticket: t, Comment: comment}, err
}
func (s *Service) Attach(ctx context.Context, actor auth.User, id string, in AttachmentInput) (Ticket, error) {
	values, err := ids(in.AttachmentIDs, 10)
	if err != nil || len(values) == 0 {
		return Ticket{}, ErrInvalidInput
	}
	return s.mutate(ctx, actor, id, in.Version, false, func(tx MutationStore, u auth.User, t Ticket) error {
		if t.Status == Closed {
			return ErrClosed
		}
		seen := map[string]bool{}
		for _, v := range t.AttachmentIDs {
			seen[v] = true
		}
		added := 0
		for _, v := range values {
			if seen[v] {
				continue
			}
			if err := tx.CheckMedia(ctx, v, u.ID, u.Role == auth.RoleAdmin); err != nil {
				return err
			}
			if len(seen) >= 10 {
				return ErrInvalidInput
			}
			seen[v] = true
			added++
			if err := tx.LinkAttachment(ctx, id, v); err != nil {
				return err
			}
		}
		if added == 0 {
			return ErrConflict
		}
		if err := tx.Bump(ctx, id); err != nil {
			return err
		}
		return tx.Audit(ctx, u.ID, "ticket.attachments_added", id, map[string]any{"version": t.Version + 1, "attachment_count": added})
	})
}
func (s *Service) Confirm(ctx context.Context, actor auth.User, id string, in VersionInput) (Ticket, error) {
	return s.mutate(ctx, actor, id, in.Version, false, func(tx MutationStore, u auth.User, t Ticket) error {
		if t.RequesterID != u.ID {
			return ErrForbidden
		}
		if t.Status != Resolved {
			return ErrTransition
		}
		return changeStatus(ctx, tx, u, t, Closed, "")
	})
}
func (s *Service) Reopen(ctx context.Context, actor auth.User, id string, in ReopenInput) (Ticket, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if actor.Role != auth.RoleAdmin {
		return Ticket{}, ErrForbidden
	}
	if !validBody(in.Reason, 2000, false) {
		return Ticket{}, ErrInvalidInput
	}
	return s.mutate(ctx, actor, id, in.Version, true, func(tx MutationStore, u auth.User, t Ticket) error {
		if u.Role != auth.RoleAdmin {
			return ErrForbidden
		}
		if t.Status != Closed {
			return ErrTransition
		}
		return changeStatus(ctx, tx, u, t, InProgress, in.Reason)
	})
}
func changeStatus(ctx context.Context, tx MutationStore, u auth.User, t Ticket, to, reason string) error {
	resolved := t.ResolvedAt
	if to != Closed && to != Resolved {
		resolved = nil
	}
	if err := tx.Update(ctx, t.ID, t.Priority, t.AssigneeID, to, resolved); err != nil {
		return err
	}
	if err := tx.History(ctx, t.ID, &t.Status, to, u.ID, reason); err != nil {
		return err
	}
	return tx.Audit(ctx, u.ID, "ticket.status_changed", t.ID, map[string]any{"version": t.Version + 1, "from_status": t.Status, "to_status": to})
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func normalizeArticle(in ArticleInput) (ArticleInput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Slug = strings.TrimSpace(in.Slug)
	in.Description = strings.TrimSpace(in.Description)
	in.Symptoms = strings.TrimSpace(in.Symptoms)
	if !httpx.ValidText(in.Title, 250) || len(in.Slug) > 200 || !slugPattern.MatchString(in.Slug) || !validBody(in.Description, 2000, false) || !validBody(in.Symptoms, 10000, false) || !httpx.ValidUUID(in.CategoryID) || len(in.SolutionSteps) < 1 || len(in.SolutionSteps) > 100 || len(in.Tags) > 10 {
		return in, ErrInvalidInput
	}
	for i, v := range in.SolutionSteps {
		v = strings.TrimSpace(v)
		if !validBody(v, 5000, false) {
			return in, ErrInvalidInput
		}
		in.SolutionSteps[i] = v
	}
	tags := make([]string, 0, len(in.Tags))
	seen := map[string]bool{}
	for _, v := range in.Tags {
		v = strings.TrimSpace(v)
		if !httpx.ValidText(v, 60) {
			return in, ErrInvalidInput
		}
		key := strings.ToLower(v)
		if !seen[key] {
			tags = append(tags, v)
			seen[key] = true
		}
	}
	in.Tags = tags
	return in, nil
}
func (s *Service) Convert(ctx context.Context, actor auth.User, id string, in ArticleInput) (knowledge.Article, error) {
	in, err := normalizeArticle(in)
	if err != nil {
		return knowledge.Article{}, err
	}
	var article knowledge.Article
	_, err = s.mutate(ctx, actor, id, in.Version, true, func(tx MutationStore, u auth.User, t Ticket) error {
		if t.Status != Resolved && t.Status != Closed {
			return ErrTransition
		}
		if t.ConvertedArticleID != nil {
			return ErrAlreadyConverted
		}
		ok, err := tx.Category(ctx, in.CategoryID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInactiveCategory
		}
		article, err = tx.Convert(ctx, id, u.ID, in)
		if err != nil {
			return err
		}
		if err := tx.Audit(ctx, u.ID, "ticket.converted", id, map[string]any{"version": t.Version + 1, "article_id": article.ID}); err != nil {
			return err
		}
		return tx.Audit(ctx, u.ID, "article.created", article.ID, map[string]any{"source_ticket_id": id, "is_published": false})
	})
	return article, err
}

func (s *Service) Departments(ctx context.Context, actor auth.User) ([]Department, error) {
	if !staff(actor) {
		return nil, ErrForbidden
	}
	return s.repo.Departments(ctx, actor)
}
