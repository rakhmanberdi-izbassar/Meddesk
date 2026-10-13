package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type sessionContextKey struct{}
type Guard struct {
	service *Service
	origins map[string]struct{}
}

func NewGuard(service *Service, opts Options) *Guard {
	origins := make(map[string]struct{}, len(opts.AllowedOrigins))
	for _, origin := range opts.AllowedOrigins {
		origins[origin] = struct{}{}
	}
	return &Guard{service: service, origins: origins}
}

func (g *Guard) validOrigin(r *http.Request) bool {
	values := r.Header.Values("Origin")
	if len(values) != 1 {
		return false
	}
	_, allowed := g.origins[values[0]]
	return allowed
}

func csrfMatches(token string, session Session) bool {
	hash, ok := hashToken(token)
	return ok && len(session.CSRFHash) == 32 && subtle.ConstantTimeCompare(hash, session.CSRFHash) == 1
}

func (g *Guard) Protect(next http.Handler, roles ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil {
			response.Error(w, r, http.StatusUnauthorized, "unauthenticated", "Authentication required")
			return
		}
		session, err := g.service.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			if errors.Is(err, ErrUnauthenticated) {
				response.Error(w, r, http.StatusUnauthorized, "unauthenticated", "Authentication required")
			} else {
				response.Error(w, r, http.StatusInternalServerError, "internal_error", "Unable to authenticate request")
			}
			return
		}
		if len(roles) > 0 {
			allowed := false
			for _, role := range roles {
				if session.User.Role == role {
					allowed = true
					break
				}
			}
			if !allowed {
				response.Error(w, r, http.StatusForbidden, "forbidden", "Insufficient permissions")
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			csrfCookie, err := r.Cookie(CSRFCookie)
			if !g.validOrigin(r) || err != nil || !csrfMatches(csrfCookie.Value, session) || len(r.Header.Values(CSRFHeader)) != 1 || !csrfMatches(r.Header.Get(CSRFHeader), session) {
				response.Error(w, r, http.StatusForbidden, "csrf_failed", "A valid origin and CSRF token are required")
				return
			}
		}
		ctx := context.WithValue(r.Context(), userContextKey{}, session.User)
		ctx = context.WithValue(ctx, sessionContextKey{}, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
