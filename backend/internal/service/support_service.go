package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/config"
	"cadastral-boundary-topology-resolution/backend/internal/dto"
	"cadastral-boundary-topology-resolution/backend/internal/model"
	"cadastral-boundary-topology-resolution/backend/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	CodeInvalidInput   = "INVALID_INPUT"
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "STATE_CONFLICT"
	CodeUnauthorized   = "AUTH_REQUIRED"
	CodeForbidden      = "FORBIDDEN"
	CodeAlgorithmInput = "ALGORITHM_INPUT_INSUFFICIENT"
	CodeInternal       = "INTERNAL_ERROR"
)

type AppError struct {
	Code    string
	Status  int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func invalid(message string, err error) error {
	return &AppError{CodeInvalidInput, http.StatusBadRequest, message, err}
}

func notFound(resource string) error {
	return &AppError{CodeNotFound, http.StatusNotFound, resource + " does not exist", repository.ErrNotFound}
}

func conflict(message string, err error) error {
	return &AppError{CodeConflict, http.StatusConflict, message, err}
}

func internal(message string, err error) error {
	return &AppError{CodeInternal, http.StatusInternalServerError, message, err}
}

type Actor struct {
	ID                        uint
	Username, Role, RequestID string
}

type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type AuthService struct {
	store  *repository.Store
	secret []byte
	ttl    time.Duration
}

func NewAuthService(store *repository.Store, cfg config.Config) *AuthService {
	return &AuthService{store, []byte(cfg.JWTSecret), cfg.AccessTokenTTL}
}

func (s *AuthService) Login(request dto.LoginRequest) (dto.LoginResponse, error) {
	user, err := s.store.Users.FindByUsername(request.Username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return dto.LoginResponse{}, &AppError{CodeUnauthorized, http.StatusUnauthorized, "username or password is incorrect", err}
		}
		return dto.LoginResponse{}, internal("authentication lookup failed", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)); err != nil {
		return dto.LoginResponse{}, &AppError{CodeUnauthorized, http.StatusUnauthorized, "username or password is incorrect", err}
	}
	now, expires := time.Now(), time.Now().Add(s.ttl)
	claims := Claims{UserID: user.ID, Username: user.Username, Role: user.Role, RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprint(user.ID), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), NotBefore: jwt.NewNumericDate(now)}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return dto.LoginResponse{}, internal("token signing failed", err)
	}
	return dto.LoginResponse{Token: token, ExpiresAt: expires, User: dto.UserView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role}}, nil
}

func (s *AuthService) Parse(tokenString string) (Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return Claims{}, &AppError{CodeUnauthorized, http.StatusUnauthorized, "access token is invalid or expired", err}
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return Claims{}, &AppError{CodeUnauthorized, http.StatusUnauthorized, "access token claims are invalid", nil}
	}
	user, err := s.store.Users.FindByID(claims.UserID)
	if err != nil {
		return Claims{}, &AppError{CodeUnauthorized, http.StatusUnauthorized, "account is inactive", err}
	}
	// Authorization follows the current account record, not role/name claims
	// captured when an older token was issued.
	claims.Username = user.Username
	claims.Role = user.Role
	return *claims, nil
}

type AuditService struct{ store *repository.Store }

func NewAuditService(store *repository.Store) *AuditService { return &AuditService{store} }

func (s *AuditService) List(query dto.AuditQuery) ([]model.AuditLog, dto.Pagination, error) {
	normalizePage(&query.Page, &query.PageSize)
	entries, total, err := s.store.Audits.List(query)
	if err != nil {
		return nil, dto.Pagination{}, internal("list audit entries failed", err)
	}
	return entries, dto.Pagination{Page: query.Page, PageSize: query.PageSize, Total: total}, nil
}

func (s *AuditService) Ready(ctx context.Context) error {
	return s.store.Ping(ctx)
}

func normalizePage(page, size *int) {
	if *page < 1 {
		*page = 1
	}
	if *size < 1 {
		*size = 20
	}
	if *size > 100 {
		*size = 100
	}
}

func snapshot(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("{\"summary_error\":%q}", err.Error())
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return string(encoded)
	}
	scrubSnapshot(decoded)
	clean, err := json.Marshal(decoded)
	if err != nil {
		return string(encoded)
	}
	return string(clean)
}

func scrubSnapshot(value any) {
	const redacted = "[REDACTED]"
	sensitive := func(key string) bool {
		key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
		for _, token := range []string{"password", "passwd", "token", "authorization", "secret", "credential", "private_key", "apikey", "api_key"} {
			if strings.Contains(key, token) {
				return true
			}
		}
		return false
	}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			for key, child := range current {
				if sensitive(key) {
					current[key] = redacted
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
}

func audit(actor Actor, action, resource string, resourceID uint, relatedID *uint, before, after string) *model.AuditLog {
	return &model.AuditLog{
		ActorID:      actor.ID,
		ActorName:    actor.Username,
		Action:       action,
		ResourceType: resource,
		ResourceID:   resourceID,
		RelatedID:    relatedID,
		RequestID:    actor.RequestID,
		Before:       before,
		After:        after,
	}
}
