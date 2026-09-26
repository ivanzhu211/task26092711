package boothalloc

import (
	"container/heap"
	"database/sql"
	"strconv"
	"strings"
	"time"
)

const offerTTL = 48 * time.Hour

// offerHeap orders live offers by (expiresAt, seq).
type offerHeap []*request

func (h offerHeap) Len() int { return len(h) }
func (h offerHeap) Less(i, j int) bool {
	if !h[i].expiresAt.Equal(h[j].expiresAt) {
		return h[i].expiresAt.Before(h[j].expiresAt)
	}
	return h[i].seq < h[j].seq
}
func (h offerHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *offerHeap) Push(x any)    { *h = append(*h, x.(*request)) }
func (h *offerHeap) Pop() any {
	old := *h
	n := len(old)
	r := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return r
}

// settle advances all time-driven effects up to and including t.
//
// Offers expiring at exactly t are treated as expired. Expirations sharing one
// instant are released together before a single queue sweep.
func (s *Store) settle(tx *sql.Tx, t time.Time) error {
	for s.offers.Len() > 0 {
		nearest := (*s.offers)[0]
		if nearest.expiresAt.After(t) {
			break
		}
		expiry := nearest.expiresAt
		batch := make([]*request, 0, 1)
		for s.offers.Len() > 0 && (*s.offers)[0].expiresAt.Equal(expiry) {
			batch = append(batch, heap.Pop(s.offers).(*request))
		}
		for _, r := range batch {
			s.markFree(r.cells)
			delete(s.reqs, r.exhibitor)
			if _, err := tx.Exec(`DELETE FROM requests WHERE exhibitor = ?`, r.exhibitor); err != nil {
				return err
			}
		}
		s.spaceGen++
		if err := s.scanQueue(tx, expiry); err != nil {
			return err
		}
	}
	return nil
}

// scanQueue serves waiters whose size now fits, in arrival order.
//
// A waiter that does not fit does not block later waiters. Each waiter is
// attempted at most once per space generation.
func (s *Store) scanQueue(tx *sql.Tx, at time.Time) error {
	gen := s.spaceGen
	keep := s.waitlist[:0]
	for _, w := range s.waitlist {
		if w.lastCheck == gen {
			keep = append(keep, w)
			continue
		}
		w.lastCheck = gen
		cells, ok := s.tryPlace(w.size)
		if !ok {
			keep = append(keep, w)
			continue
		}
		w.state = stateOffered
		w.cells = cells
		w.expiresAt = at.Add(offerTTL)
		s.markUsed(cells)
		heap.Push(s.offers, w)
		if _, err := tx.Exec(
			`UPDATE requests SET state = 'offered', cells = ?, expires_at = ? WHERE exhibitor = ?`,
			encodeCells(cells), w.expiresAt, w.exhibitor,
		); err != nil {
			return err
		}
	}
	s.waitlist = keep
	return nil
}

func encodeCells(cells []int) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}

func decodeCells(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	cells := make([]int, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		cells = append(cells, v)
	}
	return cells
}
