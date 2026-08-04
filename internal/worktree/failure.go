package worktree

import (
	"context"
	"errors"
)

const (
	failureDiscoveryBound      = "discovery_bound_exceeded"
	failureDiscoveryIncomplete = "discovery_incomplete"
	failureGitChanged          = "git_executable_changed"
	failureGitInspection       = "git_inspection_incomplete"
	failureGitUnavailable      = "git_unavailable"
)

type evidenceError struct {
	category string
	message  string
	cause    error
}

func (e *evidenceError) Error() string { return e.message }
func (e *evidenceError) Unwrap() error { return e.cause }

func newEvidenceError(category, message string, cause error) error {
	return &evidenceError{category: category, message: message, cause: cause}
}

// EvidenceCategory returns one bounded, path-free category for durable audit.
func EvidenceCategory(err error) string {
	var evidence *evidenceError
	if errors.As(err, &evidence) {
		return evidence.category
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "operation_timed_out"
	}
	if errors.Is(err, context.Canceled) {
		return "operation_canceled"
	}
	return "inventory_incomplete"
}
