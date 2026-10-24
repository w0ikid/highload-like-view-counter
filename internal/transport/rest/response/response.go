package response

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"highload-counters/internal/domain"
)

type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"requestId"`
}

// StatusOf maps general kinds only. Specific error codes belong to domain.
func StatusOf(err error) int {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrAlreadyExists), errors.Is(err, domain.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, domain.ErrRateLimited):
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func HandleError(w http.ResponseWriter, r *http.Request, id string, err error) {
	status := StatusOf(err)
	body := ErrorResponse{Code: "internal_error", Message: "internal error", RequestID: id}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		body.Code, body.Message = "deadline_exceeded", "request deadline exceeded"
	case errors.Is(err, context.Canceled):
		body.Code, body.Message = "canceled", "request canceled"
	case status != http.StatusInternalServerError:
		if de, ok := domain.AsError(err); ok {
			body.Code, body.Message, body.Field = de.Code, de.Message, de.Field
		}
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	if status >= 500 {
		slog.ErrorContext(r.Context(), "request failed", "request_id", id, "code", body.Code)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
