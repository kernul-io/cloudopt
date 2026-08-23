package gcpinventory

import (
	"errors"
	"fmt"
	"strings"
)

const collectorSource = "gcp-inventory/collector"

// APIError captures a redacted Google API failure at the adapter boundary.
type APIError struct {
	Service   string
	Operation string
	Scope     string
	Code      string
	Message   string
	Retryable bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("gcp %s.%s in %s: %s (%s)", e.Service, e.Operation, e.Scope, e.Code, e.Message)
}

var (
	ErrAccessDenied = errors.New("access denied")
	ErrAPIDisabled  = errors.New("api not enabled")
	ErrQuota        = errors.New("quota exceeded")
	ErrCancelled    = errors.New("collection cancelled")
)

func redactMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 240 {
		msg = msg[:240] + "..."
	}
	lower := strings.ToLower(msg)
	for _, secret := range []string{"secret", "token", "password", "credential", "private_key"} {
		if strings.Contains(lower, secret) {
			return "redacted provider error"
		}
	}
	return msg
}

func isRetryableCode(code string) bool {
	switch code {
	case "RESOURCE_EXHAUSTED", "UNAVAILABLE", "DEADLINE_EXCEEDED", "INTERNAL":
		return true
	default:
		return false
	}
}

func mapGCPError(service, op, scope, code, msg string) error {
	retryable := isRetryableCode(code)
	ae := &APIError{
		Service:   service,
		Operation: op,
		Scope:     scope,
		Code:      code,
		Message:   redactMessage(msg),
		Retryable: retryable,
	}
	switch code {
	case "PERMISSION_DENIED", "UNAUTHENTICATED":
		return fmt.Errorf("%w: %w", ErrAccessDenied, ae)
	case "SERVICE_DISABLED", "FAILED_PRECONDITION":
		if strings.Contains(strings.ToLower(msg), "api") || strings.Contains(strings.ToLower(msg), "not enabled") {
			return fmt.Errorf("%w: %w", ErrAPIDisabled, ae)
		}
		return ae
	case "RESOURCE_EXHAUSTED":
		return fmt.Errorf("%w: %w", ErrQuota, ae)
	default:
		return ae
	}
}

func errorsIsAccessDenied(err error) bool {
	return errors.Is(err, ErrAccessDenied)
}

func errorsIsAPIDisabled(err error) bool {
	return errors.Is(err, ErrAPIDisabled)
}
