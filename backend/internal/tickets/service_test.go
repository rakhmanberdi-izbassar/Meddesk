package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/meddesk/meddesk/backend/internal/auth"
	"github.com/meddesk/meddesk/backend/internal/knowledge"
)

const actorID = "11111111-1111-4111-8111-111111111111"
const engineerID = "22222222-2222-4222-8222-222222222222"
const ticketID = "33333333-3333-4333-8333-333333333333"
const mediaID = "44444444-4444-4444-8444-444444444444"
const categoryID = "55555555-5555-4555-8555-555555555555"

func employee() auth.User {
	return auth.User{ID: actorID, Role: auth.RoleEmployee, IsActive: true, DepartmentID: categoryID, DisplayName: "Employee"}
}
func engineer() auth.User {
	return auth.User{ID: engineerID, Role: auth.RoleEngineer, IsActive: true, DepartmentID: categoryID, DisplayName: "Engineer"}
}

type fakeStore struct {
	tx             *fakeMutation
	transactions   int
	exports        int
	exportFilter   Filter
	exportSnapshot *ExportSnapshot
	exportErr      error
}

func (s *fakeStore) ExportSnapshot(_ context.Context, actor auth.User, f Filter, _ time.Time) (ExportSnapshot, error) {
	s.exports++
	s.exportFilter = f
	if s.exportErr != nil {
		return ExportSnapshot{}, s.exportErr
	}
	if !staff(s.tx.users[actor.ID]) {
		return ExportSnapshot{}, ErrForbidden
	}
	if s.exportSnapshot != nil {
		return *s.exportSnapshot, nil
	}
	return ExportSnapshot{ClinicName: "MedDesk", Timezone: "Asia/Almaty", Rows: []ExportRow{}}, nil
}

func (s *fakeStore) List(context.Context, auth.User, Filter) (Page, error)     { return Page{}, nil }
func (s *fakeStore) Detail(context.Context, auth.User, string) (Detail, error) { return Detail{}, nil }
func (s *fakeStore) Assignees(context.Context, auth.User) ([]Assignee, error) {
	return []Assignee{}, nil
}
func (s *fakeStore) Departments(context.Context, auth.User) ([]Department, error) {
	return []Department{}, nil
}
func (s *fakeStore) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	s.transactions++
	old := s.tx.ticket
	old.ArticleIDs = append([]string{}, old.ArticleIDs...)
	old.AttachmentIDs = append([]string{}, old.AttachmentIDs...)
	history, audits := s.tx.histories, s.tx.audits
	err := fn(s.tx)
	if err != nil {
		s.tx.ticket = old
		s.tx.histories = history
		s.tx.audits = audits
	}
	return err
}

type fakeMutation struct {
	users             map[string]auth.User
	ticket            Ticket
	lock              bool
	histories, audits int
	auditErr          error
	mediaErr          error
	categoryActive    bool
	createdActor      auth.User
}

func (f *fakeMutation) Lock(context.Context) error                      { f.lock = true; return nil }
func (f *fakeMutation) DefaultPriority(context.Context) (string, error) { return "HIGH", nil }
func (f *fakeMutation) Actor(_ context.Context, id string) (auth.User, error) {
	if !f.lock {
		panic("actor read before management lock")
	}
	u, ok := f.users[id]
	if !ok {
		return u, ErrNotFound
	}
	return u, nil
}
func (f *fakeMutation) Ticket(context.Context, string) (Ticket, error)         { return f.ticket, nil }
func (f *fakeMutation) Get(context.Context, auth.User, string) (Ticket, error) { return f.ticket, nil }
func (f *fakeMutation) Category(context.Context, string) (bool, error)         { return f.categoryActive, nil }
func (f *fakeMutation) PublicArticle(context.Context, string) error            { return nil }
func (f *fakeMutation) CheckMedia(context.Context, string, string, bool) error { return f.mediaErr }
func (f *fakeMutation) Create(_ context.Context, u auth.User, in CreateInput) (Ticket, error) {
	f.createdActor = u
	f.ticket = Ticket{ID: ticketID, RequesterID: u.ID, DepartmentID: u.DepartmentID, Version: 1, Priority: in.Priority, Status: New}
	return f.ticket, nil
}
func (f *fakeMutation) Update(_ context.Context, _ string, p string, a *string, s string, r *time.Time) error {
	f.ticket.Priority = p
	f.ticket.AssigneeID = a
	f.ticket.Status = s
	f.ticket.ResolvedAt = r
	f.ticket.Version++
	return nil
}
func (f *fakeMutation) Bump(context.Context, string) error { f.ticket.Version++; return nil }
func (f *fakeMutation) LinkArticle(_ context.Context, _, id string) error {
	f.ticket.ArticleIDs = append(f.ticket.ArticleIDs, id)
	return nil
}
func (f *fakeMutation) LinkAttachment(_ context.Context, _, id string) error {
	f.ticket.AttachmentIDs = append(f.ticket.AttachmentIDs, id)
	return nil
}
func (f *fakeMutation) Comment(_ context.Context, _ string, u auth.User, in CommentInput) (Comment, error) {
	return Comment{ID: mediaID, AuthorID: u.ID, Body: in.Body, IsInternal: in.IsInternal}, nil
}
func (f *fakeMutation) History(context.Context, string, *string, string, string, string) error {
	f.histories++
	return nil
}
func (f *fakeMutation) Convert(_ context.Context, _, actor string, in ArticleInput) (knowledge.Article, error) {
	f.ticket.ConvertedArticleID = &mediaIDValue
	f.ticket.Version++
	return knowledge.Article{ID: mediaID, AuthorID: actor, Slug: in.Slug}, nil
}

var mediaIDValue = mediaID

func (f *fakeMutation) Audit(context.Context, string, string, string, map[string]any) error {
	f.audits++
	return f.auditErr
}
func fixture() (*Service, *fakeStore) {
	e, it := employee(), engineer()
	store := &fakeStore{tx: &fakeMutation{users: map[string]auth.User{e.ID: e, it.ID: it}, ticket: Ticket{ID: ticketID, RequesterID: e.ID, Version: 1, Status: New, Priority: "MEDIUM", ArticleIDs: []string{}, AttachmentIDs: []string{}}, categoryActive: true}}
	return NewService(store, nil), store
}
func str(v string) *string { return &v }
func TestTransitionPolicy(t *testing.T) {
	allowed := map[string]bool{New + ":" + InProgress: true, InProgress + ":" + Waiting: true, InProgress + ":" + Resolved: true, Waiting + ":" + InProgress: true, Waiting + ":" + Resolved: true, Resolved + ":" + InProgress: true, Resolved + ":" + Closed: true}
	for _, from := range []string{New, InProgress, Waiting, Resolved, Closed} {
		for _, to := range []string{New, InProgress, Waiting, Resolved, Closed} {
			if got := allowedTransition(from, to); got != allowed[from+":"+to] {
				t.Errorf("%s→%s: %v", from, to, got)
			}
		}
	}
}
func TestOptionalAssignmentPreservesOmittedAndNull(t *testing.T) {
	for _, tc := range []struct {
		json    string
		present bool
		value   *string
	}{{`{"version":1}`, false, nil}, {`{"version":1,"assignee_id":null}`, true, nil}, {`{"version":1,"assignee_id":"` + engineerID + `"}`, true, str(engineerID)}} {
		var in UpdateInput
		if err := json.Unmarshal([]byte(tc.json), &in); err != nil {
			t.Fatal(err)
		}
		if in.AssigneeID.Present != tc.present {
			t.Fatalf("presence lost: %s", tc.json)
		}
		if tc.value != nil && (in.AssigneeID.Value == nil || *in.AssigneeID.Value != *tc.value) {
			t.Fatalf("assignment lost: %s", tc.json)
		}
	}
}
func TestMutationReauthorizesActorAndOwnership(t *testing.T) {
	s, store := fixture()
	current := engineer()
	current.Role = auth.RoleEmployee
	store.tx.users[current.ID] = current
	_, err := s.Update(context.Background(), engineer(), ticketID, UpdateInput{Version: 1, Priority: str("HIGH")})
	if !errors.Is(err, ErrForbidden) || store.tx.ticket.Version != 1 {
		t.Fatalf("stale role accepted: %v", err)
	}
	store.tx.users[engineerID] = engineer()
	other := employee()
	other.ID = mediaID
	store.tx.users[other.ID] = other
	_, err = s.Comment(context.Background(), other, ticketID, CommentInput{Version: 1, Body: "private attempt"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("other employee sees ticket: %v", err)
	}
	_, err = s.Comment(context.Background(), employee(), ticketID, CommentInput{Version: 1, Body: "note", IsInternal: true})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("employee internal note accepted: %v", err)
	}
}
func TestVersionConflictAndClosedMutations(t *testing.T) {
	s, store := fixture()
	_, err := s.Update(context.Background(), engineer(), ticketID, UpdateInput{Version: 2, Priority: str("HIGH")})
	if !errors.Is(err, ErrVersionConflict) || store.tx.audits != 0 {
		t.Fatalf("stale write accepted: %v", err)
	}
	store.tx.ticket.Status = Closed
	_, err = s.Comment(context.Background(), employee(), ticketID, CommentInput{Version: 1, Body: "note"})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("closed comment accepted: %v", err)
	}
	_, err = s.Attach(context.Background(), employee(), ticketID, AttachmentInput{Version: 1, AttachmentIDs: []string{mediaID}})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("closed attachment accepted: %v", err)
	}
}
func TestAssignmentAndResolutionTimestamp(t *testing.T) {
	s, store := fixture()
	store.tx.ticket.AssigneeID = str(engineerID)
	out, err := s.Update(context.Background(), engineer(), ticketID, UpdateInput{Version: 1, Status: str(InProgress)})
	if err != nil || out.AssigneeID == nil || *out.AssigneeID != engineerID {
		t.Fatalf("omitted assignment cleared: %+v %v", out, err)
	}
	out, err = s.Update(context.Background(), engineer(), ticketID, UpdateInput{Version: 2, Status: str(Resolved), AssigneeID: OptionalID{Present: true}})
	if err != nil || out.AssigneeID != nil || out.ResolvedAt == nil {
		t.Fatalf("clear/resolve failed: %+v %v", out, err)
	}
	resolved := *out.ResolvedAt
	out, err = s.Confirm(context.Background(), employee(), ticketID, VersionInput{Version: 3})
	if err != nil || out.Status != Closed || out.ResolvedAt == nil || !out.ResolvedAt.Equal(resolved) {
		t.Fatalf("owner close lost timestamp: %+v %v", out, err)
	}
	admin := engineer()
	admin.Role = auth.RoleAdmin
	store.tx.users[admin.ID] = admin
	out, err = s.Reopen(context.Background(), admin, ticketID, ReopenInput{Version: 4, Reason: "Issue persists"})
	if err != nil || out.ResolvedAt != nil || out.Status != InProgress {
		t.Fatalf("admin reopening failed: %+v %v", out, err)
	}
}
func TestAuditFailureRollsBackMutation(t *testing.T) {
	s, store := fixture()
	store.tx.auditErr = errors.New("audit unavailable")
	_, err := s.Update(context.Background(), engineer(), ticketID, UpdateInput{Version: 1, Status: str(InProgress)})
	if err == nil || store.tx.ticket.Version != 1 || store.tx.ticket.Status != New || store.tx.histories != 0 {
		t.Fatalf("partial mutation: %+v %v", store.tx.ticket, err)
	}
}
func TestCreationUsesFreshDepartmentAndValidatesAssociations(t *testing.T) {
	s, store := fixture()
	fresh := employee()
	fresh.DepartmentID = mediaID
	store.tx.users[fresh.ID] = fresh
	out, err := s.Create(context.Background(), employee(), CreateInput{Title: " Problem ", Description: "Needs support", AttachmentIDs: []string{mediaID}})
	if err != nil || out.DepartmentID != mediaID || out.Priority != "HIGH" || len(out.AttachmentIDs) != 1 {
		t.Fatalf("snapshot wrong: %+v %v", out, err)
	}
	_, err = s.Create(context.Background(), employee(), CreateInput{Title: "Problem", Description: "Support", AttachmentIDs: []string{mediaID, mediaID}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate association accepted: %v", err)
	}
	store.tx.categoryActive = false
	_, err = s.Create(context.Background(), employee(), CreateInput{Title: "Problem", Description: "Support", CategoryID: str(categoryID)})
	if !errors.Is(err, ErrInactiveCategory) {
		t.Fatalf("inactive category accepted: %v", err)
	}
}
func TestExplicitTicketPriorityOverridesClinicDefault(t *testing.T) {
	s, _ := fixture()
	out, err := s.Create(context.Background(), employee(), CreateInput{Title: "Problem", Description: "Needs support", Priority: "LOW"})
	if err != nil || out.Priority != "LOW" {
		t.Fatalf("explicit priority replaced by clinic default: %+v %v", out, err)
	}
}
func TestConversionMustBeResolvedAndUnique(t *testing.T) {
	s, store := fixture()
	in := ArticleInput{Version: 1, Slug: "support-guide", Title: "Guide", Description: "Summary", CategoryID: categoryID, Symptoms: "Printer fails", SolutionSteps: []string{"Restart printer"}}
	_, err := s.Convert(context.Background(), engineer(), ticketID, in)
	if !errors.Is(err, ErrTransition) {
		t.Fatalf("unresolved conversion accepted: %v", err)
	}
	store.tx.ticket.Status = Resolved
	a, err := s.Convert(context.Background(), engineer(), ticketID, in)
	if err != nil || a.ID == "" || store.tx.ticket.Version != 2 {
		t.Fatalf("conversion failed: %v", err)
	}
	in.Version = 2
	_, err = s.Convert(context.Background(), engineer(), ticketID, in)
	if !errors.Is(err, ErrAlreadyConverted) {
		t.Fatalf("duplicate conversion accepted: %v", err)
	}
}
