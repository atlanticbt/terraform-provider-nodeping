package client

import (
	"fmt"
	"net/http"
	"strings"
)

type APIError struct {
	StatusCode int
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("NodePing API error (status %d, request %s): %s", e.StatusCode, e.RequestID, e.Message)
	}
	return fmt.Sprintf("NodePing API error (status %d): %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether the API said the resource does not exist.
//
// NodePing does not reliably answer a missing resource with a 404 — it also
// returns `{"error": "... not found"}` under HTTP 200 (see docs/assumptions.md
// A-002), so the message is inspected as well as the status code.
//
// Retryable statuses (429 and 5xx) are excluded from message matching on
// purpose. A transient server failure whose text happens to mention "not found"
// must stay a real error: classifying it as absent would make Terraform drop
// the resource from state and recreate live infrastructure on the next apply.
func (e *APIError) IsNotFound() bool {
	if e.StatusCode == http.StatusNotFound {
		return true
	}
	if e.IsRetryable() {
		return false
	}
	return isNotFoundMessage(e.Message)
}

// notFoundMessageFragments are the phrases NodePing has been observed using to
// say "no such object". The list is deliberately narrow: an unrecognised error
// message must surface as a real error rather than a silent not-found.
var notFoundMessageFragments = []string{
	"not found",
	"does not exist",
	"doesn't exist",
	"no such",
	"unknown id",
	"nonexistent",
	"non-existent",
}

// isNotFoundMessage is the single classifier for NodePing's not-found wording.
// Every call site in this package routes through APIError.IsNotFound.
func isNotFoundMessage(message string) bool {
	lowered := strings.ToLower(message)
	for _, fragment := range notFoundMessageFragments {
		if strings.Contains(lowered, fragment) {
			return true
		}
	}
	return false
}

func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

func (e *APIError) IsRetryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}

type NotFoundError struct {
	ResourceType string
	ResourceID   string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with ID %q not found", e.ResourceType, e.ResourceID)
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error for field %q: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// RateLimitError is returned for HTTP 429 responses. RetryAfter holds the
// server-supplied delay in seconds (0 when the server sent no usable
// Retry-After header).
//
// It unwraps to an *APIError so existing errors.As/errors.Is checks against
// *APIError continue to match a 429.
type RateLimitError struct {
	RetryAfter int
	StatusCode int
	Message    string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limit exceeded, retry after %d seconds", e.RetryAfter)
	}
	return "rate limit exceeded"
}

func (e *RateLimitError) Unwrap() error {
	status := e.StatusCode
	if status == 0 {
		status = 429
	}
	return &APIError{StatusCode: status, Message: e.Message}
}
