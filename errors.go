package boothalloc

import "errors"

var (
	// ErrInvalidHall is returned when the hall definition is malformed.
	ErrInvalidHall = errors.New("boothalloc: invalid hall")
	// ErrInvalidSize is returned for an unknown booth size.
	ErrInvalidSize = errors.New("boothalloc: invalid booth size")
	// ErrTimeBeforeLast is returned when t is earlier than a previously seen time.
	ErrTimeBeforeLast = errors.New("boothalloc: command time before last observed time")
	// ErrRequestInFlight is returned when the exhibitor already has a waiting
	// request or a live offer.
	ErrRequestInFlight = errors.New("boothalloc: exhibitor already has a request in flight")
	// ErrNoActiveRequest is returned when the exhibitor has no request that the
	// command can act on.
	ErrNoActiveRequest = errors.New("boothalloc: no active request for exhibitor")
	// ErrQueueFull is returned when the waiting queue already holds 2000 entries.
	ErrQueueFull = errors.New("boothalloc: waiting queue is full")
)
