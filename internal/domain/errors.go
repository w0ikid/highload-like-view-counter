package domain

import "errors"

// Error carries a stable Code for the frontend. Specific errors point to one
// of the general kinds below; the REST transport maps only kinds to statuses.
type Error struct {
	kind    *Error
	Code    string
	Message string
	Field   string
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Is(target error) bool { return e.kind != nil && target == e.kind }

func FieldError(field, code, message string) *Error {
	e := newError(ErrValidation, code, message)
	e.Field = field
	return e
}

func newKind(code, message string) *Error { return &Error{Code: code, Message: message} }

func newError(kind *Error, code, message string) *Error {
	return &Error{kind: kind, Code: code, Message: message}
}

var (
	ErrNotFound      = newKind("not_found", "not found")
	ErrAlreadyExists = newKind("already_exists", "already exists")
	// ErrConflict: the action does not fit the current state (e.g. no trucks left).
	ErrConflict     = newKind("conflict", "conflict")
	ErrForbidden    = newKind("forbidden", "forbidden")
	ErrUnauthorized = newKind("unauthorized", "unauthorized")
	ErrValidation   = newKind("validation_failed", "validation failed")
	ErrRateLimited  = newKind("rate_limited", "too many requests")
	// ErrUnavailable: an external service the action needs is down or not configured.
	ErrUnavailable = newKind("unavailable", "service unavailable")
)

// AsError returns the outermost *Error in err's chain.
func AsError(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
