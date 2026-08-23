package ports

import (
	"errors"
	"strings"
)

// MissingPermissionsError indicates required IAM actions are not granted.
type MissingPermissionsError struct {
	Actions []string
}

func (e *MissingPermissionsError) Error() string {
	return "missing IAM permissions: " + strings.Join(e.Actions, ", ")
}

// ErrMissingPermissions returns an error for the given missing IAM actions.
func ErrMissingPermissions(actions []string) error {
	if len(actions) == 0 {
		return nil
	}
	cp := append([]string(nil), actions...)
	return &MissingPermissionsError{Actions: cp}
}

// IsMissingPermissions reports whether err is a MissingPermissionsError.
func IsMissingPermissions(err error) bool {
	var mp *MissingPermissionsError
	return errors.As(err, &mp)
}
