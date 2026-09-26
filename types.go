package boothalloc

import "time"

// Size is the kind of booth a exhibitor requests.
type Size string

const (
	// Standard occupies a single cell.
	Standard Size = "Standard"
	// Premium occupies two horizontally adjacent cells in one row.
	Premium Size = "Premium"
	// Island occupies a 2x2 block of cells.
	Island Size = "Island"
)

// Cell is one 10x10 foot spot on the hall grid.
type Cell struct {
	Row    rune // A-Z
	Column int  // 1-based
}

// Hall describes the venue grid and the cells that cannot be sold.
type Hall struct {
	Rows    int    // number of rows, addressed A..(A+Rows-1)
	Columns int    // number of columns, 1..Columns
	Blocked []Cell // aisles, pillars, ...
}

// Booth is a concrete assignment of cells.
type Booth struct {
	ID    string
	Size  Size
	Cells []Cell
}

// RequestResult reports whether a Request was served immediately.
type RequestResult struct {
	Offered bool  // true: an offer was made; false: queued
	Booth   Booth // valid when Offered is true
}

// OfferState is one live offer in a Snapshot.
type OfferState struct {
	Exhibitor string
	Booth     Booth
	Expires   time.Time
}

// SoldInfo is one sold booth in a Snapshot.
type SoldInfo struct {
	Exhibitor string
	Booth     Booth
}

// Waiter is one entry of the waiting queue in a Snapshot.
type Waiter struct {
	Exhibitor string
	Size      Size
	Since     time.Time
}

// Snapshot is the observable state at one point in time.
type Snapshot struct {
	Time     time.Time
	Sold     []SoldInfo
	Offers   []OfferState
	Waitlist []Waiter
}
