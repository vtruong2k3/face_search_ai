package store

import "errors"

var (
	ErrUnavailable  = errors.New("persistence unavailable")
	ErrConflict     = errors.New("persistence conflict")
	ErrNotFound     = errors.New("record not found")
	ErrInvalidState = errors.New("invalid persistence state")
)
