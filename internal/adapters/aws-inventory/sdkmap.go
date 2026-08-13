package awsinventory

import (
	"errors"
	"fmt"

	smithy "github.com/aws/smithy-go"
)

func mapAWSError(service, op, region string, err error) error {
	if err == nil {
		return nil
	}
	code := "Unknown"
	msg := redactMessage(err.Error())
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code = apiErr.ErrorCode()
		msg = redactMessage(apiErr.ErrorMessage())
	}
	retryable := isRetryableCode(code)
	ae := &APIError{
		Service:   service,
		Operation: op,
		Region:    region,
		Code:      code,
		Message:   msg,
		Retryable: retryable,
	}
	if code == "AccessDenied" || code == "UnauthorizedOperation" {
		return fmt.Errorf("%w: %w", ErrAccessDenied, ae)
	}
	return ae
}
