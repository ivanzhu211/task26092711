package booth

import (
	"database/sql"
	"time"
)

// loadGrid builds the unavailable-cell bitmap from blocked cells plus the
// footprints of all offered and sold booths.
func loadGrid(tx *sql.Tx, rows, cols int) (*grid, error) {
	g := newGrid(rows, cols)
	brows, err := tx.Query(`SELECT row, col FROM blocked`)
	if err != nil {
		return nil, err
	}
	defer brows.Close()
	for brows.Next() {
		var r, c int
		if err := brows.Scan(&r, &c); err != nil {
			return nil, err
		}
		g.occupy(Cell{Row: r, Col: c})
	}
	if err := brows.Err(); err != nil {
		return nil, err
	}
	qrows, err := tx.Query(
		`SELECT size, anchor_row, anchor_col FROM requests WHERE state IN (?, ?)`,
		stateOffr, stateSold)
	if err != nil {
		return nil, err
	}
	defer qrows.Close()
	for qrows.Next() {
		var size, ar, ac int
		if err := qrows.Scan(&size, &ar, &ac); err != nil {
			return nil, err
		}
		g.place(Size(size), ar, ac)
	}
	return g, qrows.Err()
}

type waitingReq struct {
	exhibitor string
	size      Size
}

func fetchQueue(tx *sql.Tx) ([]waitingReq, error) {
	rows, err := tx.Query(
		`SELECT exhibitor, size FROM requests WHERE state=? ORDER BY seq`, stateWait)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var queue []waitingReq
	for rows.Next() {
		var w waitingReq
		var size int
		if err := rows.Scan(&w.exhibitor, &size); err != nil {
			return nil, err
		}
		w.size = Size(size)
		queue = append(queue, w)
	}
	return queue, rows.Err()
}

// promote scans the waiting queue in arrival order and issues an offer for
// every request that fits in g (which it updates); requests that do not fit
// stay queued and do not block later ones. New offers expire 48h after now.
// It returns the requests still waiting.
func promote(tx *sql.Tx, g *grid, queue []waitingReq, now time.Time) ([]waitingReq, error) {
	expires := now.UTC().Add(offerTTL).UnixNano()
	remaining := queue[:0]
	for _, w := range queue {
		r, c, ok := g.findFirst(w.size)
		if !ok {
			remaining = append(remaining, w)
			continue
		}
		g.place(w.size, r, c)
		if _, err := tx.Exec(
			`UPDATE requests SET state=?, anchor_row=?, anchor_col=?, expires_at=? WHERE exhibitor=?`,
			stateOffr, r, c, expires, w.exhibitor); err != nil {
			return nil, err
		}
	}
	return remaining, nil
}

// scanQueue reloads the grid and waiting queue, then promotes whatever fits.
func scanQueue(tx *sql.Tx, hall Hall, now time.Time) error {
	g, err := loadGrid(tx, hall.Rows, hall.Cols)
	if err != nil {
		return err
	}
	queue, err := fetchQueue(tx)
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		return nil
	}
	_, err = promote(tx, g, queue, now)
	return err
}

// settle advances the stored state through every offer expiration due at or
// before t, in true time order: for each distinct expiry moment, all offers
// expiring then are released together, then the waiting queue is scanned once
// (arrival order, non-blocking) and any newly placed offers expire 48h after
// that moment. The loop continues because replacement offers may themselves
// expire before t. The grid and queue are maintained incrementally so a call
// settling thousands of backlog batches stays fast. Runs inside the caller's
// transaction.
func settle(tx *sql.Tx, hall Hall, t time.Time) error {
	tns := t.UTC().UnixNano()
	var minExp int64
	if err := tx.QueryRow(
		`SELECT COALESCE(MIN(expires_at), 0) FROM requests WHERE state=?`,
		stateOffr).Scan(&minExp); err != nil {
		return err
	}
	if minExp == 0 || minExp > tns {
		return nil // nothing to settle; avoid loading anything
	}
	g, err := loadGrid(tx, hall.Rows, hall.Cols)
	if err != nil {
		return err
	}
	queue, err := fetchQueue(tx)
	if err != nil {
		return err
	}
	for minExp != 0 && minExp <= tns {
		rows, err := tx.Query(
			`SELECT size, anchor_row, anchor_col FROM requests WHERE state=? AND expires_at=?`,
			stateOffr, minExp)
		if err != nil {
			return err
		}
		type freed struct {
			size, ar, ac int
		}
		var batch []freed
		for rows.Next() {
			var f freed
			if err := rows.Scan(&f.size, &f.ar, &f.ac); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, f)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`DELETE FROM requests WHERE state=? AND expires_at=?`, stateOffr, minExp); err != nil {
			return err
		}
		for _, f := range batch {
			g.unplace(Size(f.size), f.ar, f.ac)
		}
		if len(queue) > 0 {
			at := time.Unix(0, minExp).UTC()
			if queue, err = promote(tx, g, queue, at); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(
			`SELECT COALESCE(MIN(expires_at), 0) FROM requests WHERE state=?`,
			stateOffr).Scan(&minExp); err != nil {
			return err
		}
	}
	return nil
}
