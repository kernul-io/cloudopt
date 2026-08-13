package awsmetrics

import (
	"errors"
	"strings"

	"github.com/aws/smithy-go"
)

func isAccessDenied(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "AccessDeniedException" || code == "UnauthorizedOperation" || strings.Contains(strings.ToLower(code), "accessdenied")
	}
	return strings.Contains(strings.ToLower(err.Error()), "accessdenied")
}
