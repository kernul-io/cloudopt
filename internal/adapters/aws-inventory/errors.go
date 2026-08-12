package awsinventory

import (
	"errors"
	"fmt"
	"strings"
)

// APIError captures a redacted AWS API failure at the adapter boundary.
type APIError struct {
	Service   string
	Operation string
	Region    string
	Code      string
	Message   string
	Retryable bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("aws %s.%s in %s: %s (%s)", e.Service, e.Operation, e.Region, e.Code, e.Message)
}

// ErrAccessDenied indicates missing IAM permissions.
var ErrAccessDenied = errors.New("access denied")

// ErrRoleAssume indicates STS role assumption failed.
var ErrRoleAssume = errors.New("role assumption failed")

// ErrCancelled indicates the caller context was cancelled.
var ErrCancelled = errors.New("collection cancelled")

func redactMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 240 {
		msg = msg[:240] + "..."
	}
	lower := strings.ToLower(msg)
	for _, secret := range []string{"secret", "token", "password", "credential"} {
		if strings.Contains(lower, secret) {
			return "redacted provider error"
		}
	}
	return msg
}

func isRetryableCode(code string) bool {
	switch code {
	case "Throttling", "ThrottlingException", "RequestLimitExceeded", "TooManyRequestsException", "ServiceUnavailable", "InternalError":
		return true
	default:
		return false
	}
}
