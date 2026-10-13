package auth

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/meddesk/meddesk/backend/internal/platform/httpx"
	"github.com/meddesk/meddesk/backend/internal/platform/response"
)

type Handler struct {
	service                   *Service
	guard                     *Guard
	options                   Options
	ipLimiter, accountLimiter *limiter
}

func NewHandler(service *Service, guard *Guard, opts Options) *Handler {
	return &Handler{service: service, guard: guard, options: opts, ipLimiter: newLimiter(100, 10000, 5*time.Minute), accountLimiter: newLimiter(10, 10000, 15*time.Minute)}
}
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", h.login)
	mux.Handle("GET /api/v1/auth/me", h.guard.Protect(http.HandlerFunc(h.me)))
	mux.Handle("POST /api/v1/auth/logout", h.guard.Protect(http.HandlerFunc(h.logout)))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type authResponse struct {
	User      User   `json:"user"`
	CSRFToken string `json:"csrf_token"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.guard.validOrigin(r) {
		response.Error(w, r, http.StatusForbidden, "origin_forbidden", "A trusted request origin is required")
		return
	}
	if ok, retry := h.ipLimiter.allow(clientIP(r, h.options.TrustedProxyCIDRs)); !ok {
		rateLimited(w, r, retry)
		return
	}
	var input loginRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := httpx.Decode(w, r, &input); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		response.Error(w, r, http.StatusBadRequest, "validation_failed", "A valid email and password are required")
		return
	}
	if ok, retry := h.accountLimiter.allow(email); !ok {
		rateLimited(w, r, retry)
		return
	}
	oldToken := ""
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		oldToken = cookie.Value
	}
	result, err := h.service.Login(r.Context(), email, input.Password, oldToken)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			response.Error(w, r, http.StatusBadRequest, "validation_failed", "A valid email and a password of 12 to 72 UTF-8 bytes are required")
		case errors.Is(err, ErrInvalidCredentials):
			response.Error(w, r, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect")
		default:
			response.Error(w, r, http.StatusInternalServerError, "internal_error", "Unable to log in")
		}
		return
	}
	h.setCookie(w, SessionCookie, result.Token, result.ExpiresAt)
	h.setCookie(w, CSRFCookie, result.CSRFToken, result.ExpiresAt)
	response.JSON(w, http.StatusOK, authResponse{result.User, result.CSRFToken})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	session, _ := r.Context().Value(sessionContextKey{}).(Session)
	csrf, err := r.Cookie(CSRFCookie)
	if err != nil || !csrfMatches(csrf.Value, session) {
		h.clearCookies(w)
		response.Error(w, r, http.StatusUnauthorized, "unauthenticated", "Please log in again")
		return
	}
	response.JSON(w, http.StatusOK, authResponse{session.User, csrf.Value})
}
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(SessionCookie)
	if err := h.service.Logout(r.Context(), cookie.Value); err != nil && !errors.Is(err, ErrUnauthenticated) {
		response.Error(w, r, http.StatusInternalServerError, "internal_error", "Unable to log out")
		return
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) setCookie(w http.ResponseWriter, name, value string, expires time.Time) {
	seconds := int(math.Ceil(time.Until(expires).Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: CookiePath, HttpOnly: true, Secure: h.options.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: seconds})
}
func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{SessionCookie, CSRFCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: CookiePath, HttpOnly: true, Secure: h.options.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0), MaxAge: -1})
	}
}
func rateLimited(w http.ResponseWriter, r *http.Request, retry time.Duration) {
	seconds := int(math.Ceil(retry.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	response.Error(w, r, http.StatusTooManyRequests, "rate_limited", "Too many login attempts; please try later")
}
