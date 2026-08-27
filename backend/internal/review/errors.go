package review

import "errors"

var (
	ErrInvalidInput    = errors.New("invalid review input")
	ErrNotReady        = errors.New("generated test is not ready for review")
	ErrAlreadyReviewed = errors.New("generated test already has a review decision")
	ErrStaleVersion    = errors.New("generated test is not the latest version")
)
