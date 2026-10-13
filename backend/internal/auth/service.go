package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const PasswordCost = 12

// Password lengths are measured in UTF-8 bytes because bcrypt's hard limit is 72.
func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 || !utf8.ValidString(password) {
		return "", ErrValidation
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	return string(hash), err
}

func NormalizeEmail(email string) (string, error) {
	if !utf8.ValidString(email) {
		return "", ErrValidation
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) < 3 || len(email) > 254 || !utf8.ValidString(email) || strings.IndexFunc(email, unicode.IsSpace) >= 0 {
		return "", ErrValidation
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || strings.Count(email, "@") != 1 {
		return "", ErrValidation
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts[0]) > 64 || !strings.Contains(parts[1], ".") || strings.HasPrefix(parts[1], ".") || strings.HasSuffix(parts[1], ".") {
		return "", ErrValidation
	}
	return email, nil
}

type Service struct {
	repo      Repository
	ttl       time.Duration
	now       func() time.Time
	dummyHash []byte
}

func NewService(repo Repository, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	// Comparing a missing account still performs bcrypt work to avoid disclosing
	// account existence through a fast failure path. This is not an account hash.
	hash, _ := bcrypt.GenerateFromPassword([]byte(rand.Text()), PasswordCost)
	return &Service{repo: repo, ttl: ttl, now: time.Now, dummyHash: hash}
}

func (s *Service) Login(ctx context.Context, email, password, oldToken string) (LoginResult, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil || len(password) < 12 || len(password) > 72 || !utf8.ValidString(password) {
		return LoginResult{}, ErrValidation
	}
	credential, err := s.repo.FindUserByEmail(ctx, normalized)
	if err != nil && !errors.Is(err, ErrUnauthenticated) {
		return LoginResult{}, fmt.Errorf("find login credential: %w", err)
	}
	hash := []byte(credential.PasswordHash)
	if err != nil {
		hash = s.dummyHash
	}
	matches := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
	if err != nil || !matches || !credential.User.IsActive {
		return LoginResult{}, ErrInvalidCredentials
	}
	token, tokenHash, err := newToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate session token: %w", err)
	}
	csrfToken, csrfHash, err := newToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate CSRF token: %w", err)
	}
	expires := s.now().Add(s.ttl)
	oldHash, _ := hashToken(oldToken)
	user, err := s.repo.CreateSession(ctx, credential, tokenHash, csrfHash, oldHash, expires)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: user, Token: token, CSRFToken: csrfToken, ExpiresAt: expires}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	hash, ok := hashToken(token)
	if !ok {
		return Session{}, ErrUnauthenticated
	}
	session, err := s.repo.FindSession(ctx, hash)
	if err != nil {
		return Session{}, err
	}
	if !session.User.IsActive || !s.now().Before(session.ExpiresAt) {
		return Session{}, ErrUnauthenticated
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	hash, ok := hashToken(token)
	if !ok {
		return ErrUnauthenticated
	}
	return s.repo.RevokeSession(ctx, hash)
}

func newToken() (string, []byte, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(data)
	hash, _ := hashToken(token)
	return token, hash, nil
}

func hashToken(token string) ([]byte, bool) {
	if len(token) != 43 {
		return nil, false
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(data) != 32 || base64.RawURLEncoding.EncodeToString(data) != token {
		return nil, false
	}
	hash := sha256.Sum256([]byte(token))
	return hash[:], true
}
