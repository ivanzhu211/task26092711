package boothalloc

import (
	"database/sql"
	"time"
)

type reqState int

const (
	stateWaiting reqState = iota
	stateOffered
	stateSold
)

type request struct {
	exhibitor string
	size      Size
	state     reqState
	cells     []int
	createdAt time.Time
	expiresAt time.Time
	seq       int64
	lastCheck int64
}

// Store is an exhibition booth allocation store backed by SQLite.
type Store struct {
	db       *sql.DB
	hall     Hall
	rows     int
	cols     int
	blocked  []uint64
	free     []uint64
	reqs     map[string]*request
	waitlist []*request
	offers   *offerHeap
	seq      int64
	lastTime time.Time
	spaceGen int64
}

// Open creates or opens a store at path.
func Open(path string, hall Hall) (*Store, error) {
	return nil, nil
}

// Request places an immediate offer or enqueues the exhibitor.
func (s *Store) Request(t time.Time, exhibitor string, size Size) (RequestResult, error) {
	return RequestResult{}, nil
}

// Accept turns a live offer into a sold booth.
func (s *Store) Accept(t time.Time, exhibitor string) (Booth, error) {
	return Booth{}, nil
}

// Release withdraws a waiting request, declines an offer, or returns a sold booth.
func (s *Store) Release(t time.Time, exhibitor string) (bool, error) {
	return false, nil
}

// Snapshot returns the observable state at time t.
func (s *Store) Snapshot(t time.Time) (Snapshot, error) {
	return Snapshot{}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return nil
}
