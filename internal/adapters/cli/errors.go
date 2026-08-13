package cli

import (
	"context"
	"errors"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
)

// ExitError carries a process exit code for cobra RunE handlers.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "command failed"
}

// ExitCode maps an error to a stable CLI exit code.
func ExitCode(err error) int {
	if err == nil {
		return exitcodes.Success
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	if errors.Is(err, context.Canceled) {
		return exitcodes.GeneralError
	}

	var val *config.ValidationError
	if errors.As(err, &val) {
		return exitcodes.InvalidInput
	}

	if api.IsNotImplemented(err) {
		return exitcodes.Success
	}

	return exitcodes.GeneralError
}

// CollectionExitCode maps collect failures (reserved for step 05+).
func CollectionExitCode(err error) int {
	if err == nil {
		return exitcodes.Success
	}
	if api.IsNotImplemented(err) {
		return exitcodes.Success
	}
	return exitcodes.CollectionFail
}

// AnalysisExitCode maps analyze failures (reserved for step 03+).
func AnalysisExitCode(err error) int {
	if err == nil {
		return exitcodes.Success
	}
	if api.IsNotImplemented(err) {
		return exitcodes.Success
	}
	return exitcodes.AnalysisFail
}

// ReportExitCode maps report generation failures.
func ReportExitCode(err error) int {
	if err == nil {
		return exitcodes.Success
	}
	if api.IsNotImplemented(err) {
		return exitcodes.Success
	}
	return exitcodes.GeneralError
}
