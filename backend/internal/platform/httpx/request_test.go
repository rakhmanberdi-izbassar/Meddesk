package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeBoundsAndShape(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		status                  int
	}{
		{"valid", `{"name":"Демо"}`, "application/json; charset=utf-8", 0},
		{"unknown field", `{"role":"ADMIN"}`, "application/json", 400},
		{"multiple objects", `{"name":"a"}{"name":"b"}`, "application/json", 400},
		{"null", `null`, "application/json", 400},
		{"non utf8", "{\"name\":\"\xff\"}", "application/json", 400},
		{"wrong content type", `{"name":"a"}`, "text/plain", 415},
		{"oversized", `{"name":"` + strings.Repeat("a", MaxJSONBytes) + `"}`, "application/json", 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			var body struct {
				Name string `json:"name"`
			}
			err := Decode(httptest.NewRecorder(), r, &body)
			if tc.status == 0 {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var input *InputError
			if !errors.As(err, &input) || input.Status != tc.status {
				t.Fatalf("expected %d, got %v", tc.status, err)
			}
		})
	}
}

func TestPaginationFilters(t *testing.T) {
	page, err := ParsePagination(httptest.NewRequest("GET", "/?q=NCALayer&limit=10&offset=20&is_active=false", nil))
	if err != nil || page.Limit != 10 || page.Offset != 20 || page.Query != "NCALayer" || page.Active == nil || *page.Active {
		t.Fatalf("incorrect pagination: %+v %v", page, err)
	}
	for _, query := range []string{"limit=0", "limit=101", "offset=-1", "offset=100001", "is_active=1", "limit=10&limit=20", "owner=x", "q=" + strings.Repeat("a", 101)} {
		if _, err := ParsePagination(httptest.NewRequest("GET", "/?"+query, nil)); err == nil {
			t.Fatalf("accepted invalid filter %s", query)
		}
	}
}
