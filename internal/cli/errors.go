package cli

import (
	"errors"
	"fmt"
)

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
	if e.Message != "" && e.Err != nil {
		if e.Message == e.Err.Error() {
			return fmt.Sprintf("usage error: %s", e.Message)
		}
		return fmt.Sprintf("usage error: %s: %v", e.Message, e.Err)
	}
	if e.Err != nil {
		return fmt.Sprintf("usage error: %v", e.Err)
	}
	return fmt.Sprintf("usage error: %s", e.Message)
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

type providerConfigValidationError struct {
	message string
	err     error
}

func (e *providerConfigValidationError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.message, e.err)
	}
	return e.message
}

func (e *providerConfigValidationError) Unwrap() error {
	return e.err
}

func newProviderConfigValidationError(message string, err error) error {
	return &providerConfigValidationError{message: message, err: err}
}

func usageErrorForProviderConfig(err error) *UsageError {
	var validationErr *providerConfigValidationError
	if errors.As(err, &validationErr) {
		return &UsageError{Message: validationErr.Error(), Err: validationErr}
	}
	return &UsageError{Message: "invalid provider config", Err: err}
}

// RuntimeError represents a runtime error (network, IO, provider error, etc.)
type RuntimeError struct {
	Message string
	Err     error
}

func (e *RuntimeError) Error() string {
	if e.Message != "" && e.Err != nil {
		return fmt.Sprintf("error: %s: %v", e.Message, e.Err)
	}
	if e.Err != nil {
		return fmt.Sprintf("error: %v", e.Err)
	}
	return fmt.Sprintf("error: %s", e.Message)
}

func (e *RuntimeError) Unwrap() error {
	return e.Err
}
