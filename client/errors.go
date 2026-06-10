package client

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/internal/cli"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

// Sentinels callers branch on.
var (
	ErrSandboxNotFound     = errors.New("sandbox not found")
	ErrSandboxExists       = errors.New("sandbox already exists")
	ErrSandboxNotRunning   = errors.New("sandbox not running")
	ErrExecNotFound        = errors.New("exec not found")
	ErrIncompatibleVersion = errors.New("incompatible sbx/daemon version")
	ErrDaemonNotRunning    = errors.New("sandboxd not running")
	ErrBinaryNotFound      = cli.ErrBinaryNotFound
)

// APIError is a structured non-2xx response from the daemon REST API.
type APIError struct {
	Op      string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sbx %s: HTTP %d: %s", e.Op, e.Status, e.Message)
}

// CLIError re-exports a shell-out failure for callers (errors.As friendly).
type CLIError = cli.Error

// ParseMessage extracts {"message":...} from a daemon error body, falling back to
// the raw body.
func ParseMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	return string(body)
}

// MapError converts a transport error to a typed APIError/sentinel. Exported for
// sibling packages (sandbox, exec).
func MapError(op string, err error) error { return mapHTTPError(op, err) }

// mapHTTPError converts a transport.HTTPStatusError into an *APIError, joined with
// a sentinel when the status maps to one. Non-transport errors pass through.
func mapHTTPError(op string, err error) error {
	var se *transport.HTTPStatusError
	if !errors.As(err, &se) {
		return err
	}
	ae := &APIError{Op: op, Status: se.Status, Message: ParseMessage(se.Body)}
	switch se.Status {
	case 404:
		return errors.Join(ErrSandboxNotFound, ae)
	case 409:
		return errors.Join(ErrSandboxExists, ae)
	}
	return ae
}
