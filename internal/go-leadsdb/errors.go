package leadsdb

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common API error cases.
var (
	ErrNotFound     = errors.New("leadsdb: not found")
	ErrUnauthorized = errors.New("leadsdb: unauthorized")
	ErrRateLimited  = errors.New("leadsdb: rate limited")
	ErrForbidden    = errors.New("leadsdb: forbidden")
	ErrInternal     = errors.New("leadsdb: internal server error")
)

// APIError represents an error response from the LeadsDB API.
type APIError struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	RetryAfter int    `json:"-"` // seconds, from Retry-After header (for 429)
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("leadsdb: %s: %s (status %d)", e.Code, e.Message, e.StatusCode)
	}

	return fmt.Sprintf("leadsdb: %s (status %d)", e.Message, e.StatusCode)
}

// Is implements errors.Is support for sentinel errors.
func (e *APIError) Is(target error) bool {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return target == ErrUnauthorized
	case http.StatusForbidden:
		return target == ErrForbidden
	case http.StatusNotFound:
		return target == ErrNotFound
	case http.StatusTooManyRequests:
		return target == ErrRateLimited
	case http.StatusInternalServerError:
		return target == ErrInternal
	default:
		return false
	}
}
