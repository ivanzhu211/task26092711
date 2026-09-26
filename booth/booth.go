package booth

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DB is an open booth-allocation store. It is not safe for concurrent use.
type DB struct {
	sql  *sql.DB
	hall Hall
}

// Open creates or opens the store at path. When the file already contains a
// hall, hall must match it exactly or ErrHallMismatch is returned.
func Open(path string, hall Hall) (*DB, error) {
	sqldb, err := openStore(path, hall)
	if err != nil {
		return nil, err
	}
	blocked := append([]Cell(nil), hall.Blocked...)
	return &DB{sql: sqldb, hall: Hall{Rows: hall.Rows, Cols: hall.Cols, Blocked: blocked}}, nil
}

// Close closes the store.
func (db *DB) Close() error {
	return db.sql.Close()
}

// begin enforces time monotonicity (committing the new max time even if the
// call itself later fails, since t has been "seen"), then starts a
// transaction with all offer expirations due at or before t settled.
func (db *DB) begin(t time.Time) (*sql.Tx, error) {
	tx0, err := db.sql.Begin()
	if err != nil {
		return nil, err
	}
	if err := checkAndAdvanceTime(tx0, t); err != nil {
		tx0.Rollback()
		return nil, err
	}
	if err := tx0.Commit(); err != nil {
		return nil, err
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return nil, err
	}
	if err := settle(tx, db.hall, t); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func validSize(s Size) bool { return s >= Standard && s <= Island }

func boothID(row, col int) string { return fmt.Sprintf("%c-%d", 'A'+row, col) }

// Request registers exhibitor's desire for a booth of the given size. If one
// fits, an offer occupying concrete cells is issued immediately; otherwise
// the request joins the waiting queue. Each exhibitor may have at most one
// in-flight request.
func (db *DB) Request(t time.Time, exhibitor string, size Size) (RequestResult, error) {
	if exhibitor == "" {
		return RequestResult{}, errors.New("booth: empty exhibitor")
	}
	if !validSize(size) {
		return RequestResult{}, fmt.Errorf("booth: invalid size %d", size)
	}
	tx, err := db.begin(t)
	if err != nil {
		return RequestResult{}, err
	}
	defer tx.Rollback()

	var cnt int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM requests WHERE exhibitor=?`, exhibitor).Scan(&cnt); err != nil {
		return RequestResult{}, err
	}
	if cnt > 0 {
		return RequestResult{}, ErrDuplicateRequest
	}

	g, err := loadGrid(tx, db.hall.Rows, db.hall.Cols)
	if err != nil {
		return RequestResult{}, err
	}
	r, c, fits := g.findFirst(size)
	expires := t.UTC().Add(offerTTL).UnixNano()
	if !fits {
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM requests WHERE state=?`, stateWait).Scan(&cnt); err != nil {
			return RequestResult{}, err
		}
		if cnt >= maxQueue {
			return RequestResult{}, ErrQueueFull
		}
		r, c = -1, -1
		expires = 0
	}
	seq, err := nextSeq(tx)
	if err != nil {
		return RequestResult{}, err
	}
	state := stateOffr
	if !fits {
		state = stateWait
	}
	if _, err := tx.Exec(
		`INSERT INTO requests (exhibitor, size, state, seq, anchor_row, anchor_col, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		exhibitor, int(size), state, seq, r, c, expires); err != nil {
		return RequestResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RequestResult{}, err
	}
	if !fits {
		return RequestResult{Queued: true}, nil
	}
	return RequestResult{Offer: &Offer{
		Exhibitor: exhibitor,
		BoothID:   boothID(r, c),
		Size:      size,
		ExpiresAt: time.Unix(0, expires).UTC(),
	}}, nil
}

// Accept converts the exhibitor's live offer into a sale. Offers already
// expired at t were released during settlement, so Accept then fails with
// ErrNoOffer.
func (db *DB) Accept(t time.Time, exhibitor string) (AcceptResult, error) {
	tx, err := db.begin(t)
	if err != nil {
		return AcceptResult{}, err
	}
	defer tx.Rollback()
	var size, ar, ac int
	err = tx.QueryRow(
		`SELECT size, anchor_row, anchor_col FROM requests WHERE exhibitor=? AND state=?`,
		exhibitor, stateOffr).Scan(&size, &ar, &ac)
	if err == sql.ErrNoRows {
		return AcceptResult{}, ErrNoOffer
	}
	if err != nil {
		return AcceptResult{}, err
	}
	if _, err := tx.Exec(
		`UPDATE requests SET state=?, expires_at=0 WHERE exhibitor=?`, stateSold, exhibitor); err != nil {
		return AcceptResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{BoothID: boothID(ar, ac), Size: Size(size)}, nil
}

// Release withdraws a waiting request, declines an offer, or returns a sold
// booth. Freed cells trigger one waiting-queue scan at time t.
func (db *DB) Release(t time.Time, exhibitor string) (ReleaseKind, error) {
	tx, err := db.begin(t)
	if err != nil {
		return ReleaseNone, err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRow(
		`SELECT state FROM requests WHERE exhibitor=?`, exhibitor).Scan(&state)
	if err == sql.ErrNoRows {
		return ReleaseNone, tx.Commit()
	}
	if err != nil {
		return ReleaseNone, err
	}
	if _, err := tx.Exec(`DELETE FROM requests WHERE exhibitor=?`, exhibitor); err != nil {
		return ReleaseNone, err
	}
	kind := ReleaseWaiting
	if state == stateOffr {
		kind = ReleaseOffer
	} else if state == stateSold {
		kind = ReleaseSold
	}
	if state == stateOffr || state == stateSold {
		if err := scanQueue(tx, db.hall, t); err != nil {
			return ReleaseNone, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ReleaseNone, err
	}
	return kind, nil
}

// Snapshot returns the sold booths, live offers (with their expiry moments)
// and the waiting queue as of time t.
func (db *DB) Snapshot(t time.Time) (Snapshot, error) {
	tx, err := db.begin(t)
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()

	snap := Snapshot{
		Sold:   []SoldBooth{},
		Offers: []Offer{},
		Queue:  []QueuedRequest{},
	}
	rows, err := tx.Query(
		`SELECT exhibitor, size, state, anchor_row, anchor_col, expires_at FROM requests
		 ORDER BY CASE state WHEN 'sold' THEN 0 WHEN 'offered' THEN 1 ELSE 2 END,
		          anchor_row, anchor_col, seq`)
	if err != nil {
		return Snapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var exhibitor, state string
		var size, ar, ac int
		var expNS int64
		if err := rows.Scan(&exhibitor, &size, &state, &ar, &ac, &expNS); err != nil {
			return Snapshot{}, err
		}
		switch state {
		case stateSold:
			snap.Sold = append(snap.Sold, SoldBooth{
				Exhibitor: exhibitor, BoothID: boothID(ar, ac), Size: Size(size)})
		case stateOffr:
			snap.Offers = append(snap.Offers, Offer{
				Exhibitor: exhibitor, BoothID: boothID(ar, ac), Size: Size(size),
				ExpiresAt: time.Unix(0, expNS).UTC()})
		case stateWait:
			snap.Queue = append(snap.Queue, QueuedRequest{
				Exhibitor: exhibitor, Size: Size(size)})
		}
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, err
	}
	return snap, tx.Commit()
}
