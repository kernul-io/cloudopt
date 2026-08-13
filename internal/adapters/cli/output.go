package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Result is the stable machine-readable command outcome written to stdout.
type Result struct {
	Status  string `json:"status"`
	Command string `json:"command,omitempty"`
	Message string `json:"message,omitempty"`
	Version string `json:"version,omitempty"`
}

const (
	StatusOK             = "ok"
	StatusError          = "error"
	StatusNotImplemented = "not_implemented"
)

func writeResult(w io.Writer, r Result) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// EmitOK writes a successful command result to stdout.
func EmitOK(command, message string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: command,
		Message: message,
	})
}

// EmitError writes a failed command result to stdout.
func EmitError(command, message string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusError,
		Command: command,
		Message: message,
	})
}

// EmitNotImplemented writes a deferred-feature result to stdout.
func EmitNotImplemented(command, message string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusNotImplemented,
		Command: command,
		Message: message,
	})
}

// EmitVersion writes version metadata to stdout.
func EmitVersion(version string) error {
	return writeResult(os.Stdout, Result{
		Status:  StatusOK,
		Command: "version",
		Version: version,
	})
}

// FormatResult returns JSON for tests.
func FormatResult(r Result) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b) + "\n", nil
}
