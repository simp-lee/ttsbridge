package cli

import "fmt"

// Exit codes
const (
	ExitSuccess      = 0 // Success
	ExitRuntimeError = 1 // Runtime failure (network, provider error, IO error, etc.)
	ExitUsageError   = 2 // Usage/argument error
)

// UsageError represents a command-line usage error
type UsageError struct {
	Message string
	Err     error
}

func (e *UsageError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("usage error: %v", e.Err)
	}
	return fmt.Sprintf("usage error: %s", e.Message)
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

// RuntimeError represents a runtime error (network, IO, provider error, etc.)
type RuntimeError struct {
	Message string
	Err     error
}

func (e *RuntimeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("error: %v", e.Err)
	}
	return fmt.Sprintf("error: %s", e.Message)
}

func (e *RuntimeError) Unwrap() error {
	return e.Err
}
