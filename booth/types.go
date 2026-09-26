package booth

import "time"

// Size is a booth footprint kind.
type Size int

const (
	Standard Size = iota + 1 // 1 cell
	Premium                  // 2 adjacent cells, same row
	Island                   // 2x2 cells
)

// Cell is one 10x10 ft grid square. Row is 0-based ('A'=0), Col is 1-based.
type Cell struct {
	Row int
	Col int
}

// Hall describes the venue grid and its unsellable cells.
type Hall struct {
	Rows    int // 1..26
	Cols    int // 1..80
	Blocked []Cell
}

// Offer is a live placement offer occupying concrete cells.
type Offer struct {
	Exhibitor string
	BoothID   string
	Size      Size
	ExpiresAt time.Time
}

// SoldBooth is a confirmed sale.
type SoldBooth struct {
	Exhibitor string
	BoothID   string
	Size      Size
}

// QueuedRequest is a waiting request in arrival order.
type QueuedRequest struct {
	Exhibitor string
	Size      Size
}

// Snapshot is the state visible at time t.
type Snapshot struct {
	Sold   []SoldBooth
	Offers []Offer
	Queue  []QueuedRequest
}

// RequestResult is the outcome of Request: either an immediate offer or queued.
type RequestResult struct {
	Offer  *Offer // non-nil when an offer was made
	Queued bool   // true when placed on the waiting queue
}

// AcceptResult is the outcome of a successful Accept.
type AcceptResult struct {
	BoothID string
	Size    Size
}

// ReleaseKind reports what Release removed.
type ReleaseKind int

const (
	ReleaseNone    ReleaseKind = iota // no active request for the exhibitor
	ReleaseWaiting                    // pulled from the waiting queue
	ReleaseOffer                      // offer declined
	ReleaseSold                       // sold booth returned
)
