package knowledge

import (
	"context"
	"errors"
	"github.com/meddesk/meddesk/backend/internal/auth"
	"strings"
	"testing"
)

const actorID = "00000000-0000-4000-8000-000000000001"
const categoryID = "00000000-0000-4000-8000-000000000002"
const articleID = "00000000-0000-4000-8000-000000000003"

func truth(v bool) *bool  { return &v }
func engineer() auth.User { return auth.User{ID: actorID, Role: auth.RoleEngineer, IsActive: true} }
func validInput() ArticleInput {
	return ArticleInput{Title: " NCALayer іске қосу ", Slug: "ncalayer-start", Description: " Қауіпсіз нұсқаулық ", CategoryID: categoryID, Symptoms: " ЭЦҚ қосылмайды ", SolutionSteps: []string{" NCALayer ашыңыз "}, Tags: []string{"ЭЦҚ", "эцқ", "NCALayer"}, ImageIDs: []string{}, IsPublished: truth(true)}
}

type serviceStore struct {
	Store
	mutation *serviceMutation
	filter   Filter
	lists    int
}

func (r *serviceStore) Transaction(ctx context.Context, fn func(MutationStore) error) error {
	return fn(r.mutation)
}
func (r *serviceStore) List(_ context.Context, f Filter) (ArticlePage, error) {
	r.filter = f
	r.lists++
	return ArticlePage{Items: []Article{}, Limit: f.Limit}, nil
}

type serviceMutation struct {
	MutationStore
	current            auth.User
	activeCategory     bool
	version            int32
	saved              ArticleInput
	writes, audits     int
	mediaErr, auditErr error
}

func (m *serviceMutation) Lock(context.Context) error                       { return nil }
func (m *serviceMutation) Actor(context.Context, string) (auth.User, error) { return m.current, nil }
func (m *serviceMutation) Category(context.Context, string) (Category, error) {
	return Category{ID: categoryID, IsActive: m.activeCategory}, nil
}
func (m *serviceMutation) Article(context.Context, string, bool) (Article, error) {
	return Article{ID: articleID, Version: m.version}, nil
}
func (m *serviceMutation) CheckMedia(context.Context, string, string, string, string, bool) error {
	return m.mediaErr
}
func (m *serviceMutation) CreateArticle(_ context.Context, _ string, input ArticleInput) (Article, error) {
	m.writes++
	m.saved = input
	return Article{ID: articleID, Version: 1, CategoryID: categoryID, IsPublished: *input.IsPublished}, nil
}
func (m *serviceMutation) UpdateArticle(ctx context.Context, _ string, input ArticleInput) (Article, error) {
	return m.CreateArticle(ctx, actorID, input)
}
func (m *serviceMutation) Audit(context.Context, string, string, string, string, map[string]any) error {
	m.audits++
	return m.auditErr
}
func fixtureService() (*Service, *serviceStore) {
	repo := &serviceStore{mutation: &serviceMutation{current: engineer(), activeCategory: true, version: 2}}
	return NewService(repo), repo
}
func TestArticleValidationAndNormalization(t *testing.T) {
	s, repo := fixtureService()
	article, err := s.Save(context.Background(), engineer(), "", validInput())
	if err != nil {
		t.Fatal(err)
	}
	if article.Version != 1 || repo.mutation.audits != 1 || repo.mutation.saved.Title != "NCALayer іске қосу" || len(repo.mutation.saved.Tags) != 2 || repo.mutation.saved.SolutionSteps[0] != "NCALayer ашыңыз" {
		t.Fatalf("normalization/audit failed: %+v", repo.mutation.saved)
	}
	cases := []struct {
		name   string
		mutate func(*ArticleInput)
	}{{"missing publication", func(i *ArticleInput) { i.IsPublished = nil }}, {"empty steps", func(i *ArticleInput) { i.SolutionSteps = nil }}, {"empty step", func(i *ArticleInput) { i.SolutionSteps = []string{" "} }}, {"invalid slug", func(i *ArticleInput) { i.Slug = "../file" }}, {"duplicate images", func(i *ArticleInput) { i.ImageIDs = []string{actorID, strings.ToUpper(actorID)} }}, {"invalid UTF8", func(i *ArticleInput) { i.Description = string([]byte{0xff}) }}, {"control title", func(i *ArticleInput) { i.Title = "Hello\nworld" }}, {"long symptom", func(i *ArticleInput) { i.Symptoms = strings.Repeat("қ", 10001) }}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validInput()
			tc.mutate(&input)
			before := repo.mutation.writes
			if _, err := s.Save(context.Background(), engineer(), "", input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("invalid accepted: %v", err)
			}
			if repo.mutation.writes != before {
				t.Fatal("invalid input wrote data")
			}
		})
	}
}
func TestArticleSaveRechecksRoleVersionCategoryAndMedia(t *testing.T) {
	s, repo := fixtureService()
	input := validInput()
	input.Version = 1
	if _, err := s.Save(context.Background(), engineer(), articleID, input); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale version: %v", err)
	}
	input.Version = 2
	repo.mutation.current.Role = auth.RoleEmployee
	if _, err := s.Save(context.Background(), engineer(), articleID, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stale role authorized: %v", err)
	}
	repo.mutation.current = engineer()
	repo.mutation.activeCategory = false
	if _, err := s.Save(context.Background(), engineer(), articleID, input); !errors.Is(err, ErrInactiveCategory) {
		t.Fatalf("inactive category: %v", err)
	}
	repo.mutation.activeCategory = true
	repo.mutation.mediaErr = ErrForbidden
	input.ImageIDs = []string{actorID}
	if _, err := s.Save(context.Background(), engineer(), articleID, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign media: %v", err)
	}
	if repo.mutation.writes != 0 || repo.mutation.audits != 0 {
		t.Fatal("failed policies wrote data")
	}
}
func TestInactiveCategoryCanRetainDraft(t *testing.T) {
	s, repo := fixtureService()
	repo.mutation.activeCategory = false
	input := validInput()
	input.IsPublished = truth(false)
	if _, err := s.Save(context.Background(), engineer(), "", input); err != nil {
		t.Fatalf("draft in inactive category: %v", err)
	}
}
func TestEmployeeSearchCannotLeakDrafts(t *testing.T) {
	s, repo := fixtureService()
	employee := engineer()
	employee.Role = auth.RoleEmployee
	if _, err := s.List(context.Background(), employee, Filter{Published: truth(false), Limit: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("draft filter: %v", err)
	}
	if _, err := s.List(context.Background(), employee, Filter{Search: " ЭЦҚ ", Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if !repo.filter.PublicOnly || repo.filter.Published == nil || !*repo.filter.Published || repo.filter.Search != "ЭЦҚ" {
		t.Fatalf("unconstrained search: %+v", repo.filter)
	}
	for _, f := range []Filter{{Limit: 101}, {Limit: 20, Offset: -1}, {Limit: 20, CategoryID: "../bad"}, {Limit: 20, Search: strings.Repeat("a", 101)}} {
		if _, err := s.List(context.Background(), employee, f); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid filter accepted: %+v %v", f, err)
		}
	}
}
func TestCategoryMutationRequiresAdministrator(t *testing.T) {
	s, _ := fixtureService()
	if _, err := s.SaveCategory(context.Background(), engineer(), "", CategoryInput{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("engineer category mutation: %v", err)
	}
}
