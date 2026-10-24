package response

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"highload-counters/internal/domain"
)

func TestDomainErrorResponse(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		err                  error
		status               int
		code, message, field string
	}{
		{"field", fmt.Errorf("service internals: %w", domain.ErrPasswordInvalid), 400, "password_invalid", "password must be 8-128 characters", "password"},
		{"credentials", domain.ErrInvalidCredentials, 401, "invalid_credentials", "invalid email or password", ""},
		{"email conflict", domain.ErrEmailTaken, 409, "email_taken", "email is used by another account", ""},
		{"expired", domain.ErrTokenExpired, 401, "token_expired", "token expired", ""},
		{"dependency", fmt.Errorf("%w: password=secret host=private", domain.ErrUnavailable), 503, "unavailable", "service unavailable", ""},
		{"unexpected", errors.New("SQL password=secret"), 500, "internal_error", "internal error", ""},
		{"deadline precedence", fmt.Errorf("%w: %w", domain.ErrUnavailable, context.DeadlineExceeded), 504, "deadline_exceeded", "request deadline exceeded", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			HandleError(recorder, httptest.NewRequest("GET", "/", nil), "request-123", tc.err)
			var body ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != tc.status || body.Code != tc.code || body.Message != tc.message || body.Field != tc.field || body.RequestID != "request-123" {
				t.Fatalf("status=%d body=%+v", recorder.Code, body)
			}
		})
	}
}

func TestWrappedSpecificErrorKeepsKind(t *testing.T) {
	err := fmt.Errorf("create user: %w", domain.ErrEmailTaken)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatal("specific error lost kind")
	}
	if errors.Is(err, domain.ErrValidation) {
		t.Fatal("incorrect kind match")
	}
	de, ok := domain.AsError(err)
	if !ok || de != domain.ErrEmailTaken {
		t.Fatal("specific error lost code")
	}
}
