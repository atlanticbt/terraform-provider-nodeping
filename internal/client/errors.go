package client

import (
	"fmt"
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

func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
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
