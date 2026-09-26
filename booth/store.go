package booth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var (
	// ErrTimeRegression is returned when t is smaller than any previously seen time.
	ErrTimeRegression = errors.New("booth: time went backwards")
	// ErrHallMismatch is returned when Open is given a hall that differs from the stored one.
	ErrHallMismatch = errors.New("booth: hall does not match stored hall")
	// ErrQueueFull is returned when the waiting queue has reached its cap.
	ErrQueueFull = errors.New("booth: waiting queue is full")
	// ErrDuplicateRequest is returned when the exhibitor already has an in-flight request.
	ErrDuplicateRequest = errors.New("booth: exhibitor already has an in-flight request")
	// ErrNoOffer is returned by Accept when there is no live offer for the exhibitor.
	ErrNoOffer = errors.New("booth: no live offer for exhibitor")
)

const (
	maxRows   = 26
	maxCols   = 80
	maxQueue  = 2000
	offerTTL  = 48 * time.Hour
	stateWait = "waiting"
	stateOffr = "offered"
	stateSold = "sold"
)

const schema = `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS blocked (row INTEGER NOT NULL, col INTEGER NOT NULL, PRIMARY KEY (row, col));
CREATE TABLE IF NOT EXISTS requests (
	exhibitor  TEXT PRIMARY KEY,
	size       INTEGER NOT NULL,
	state      TEXT NOT NULL,
	seq        INTEGER NOT NULL,
	anchor_row INTEGER NOT NULL DEFAULT -1,
	anchor_col INTEGER NOT NULL DEFAULT -1,
	expires_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_requests_state_seq ON requests (state, seq);
CREATE INDEX IF NOT EXISTS idx_requests_expiry ON requests (state, expires_at);
`

type hallConfig struct {
	Rows    int    `json:"rows"`
	Cols    int    `json:"cols"`
	Blocked []Cell `json:"blocked"`
}

func validateHall(h Hall) error {
	if h.Rows < 1 || h.Rows > maxRows || h.Cols < 1 || h.Cols > maxCols {
		return fmt.Errorf("booth: hall dimensions %dx%d out of range", h.Rows, h.Cols)
	}
	seen := make(map[Cell]bool, len(h.Blocked))
	for _, c := range h.Blocked {
		if c.Row < 0 || c.Row >= h.Rows || c.Col < 1 || c.Col > h.Cols {
			return fmt.Errorf("booth: blocked cell %v out of range", c)
		}
		if seen[c] {
			return fmt.Errorf("booth: duplicate blocked cell %v", c)
		}
		seen[c] = true
	}
	return nil
}

func openStore(path string, hall Hall) (*sql.DB, error) {
	if err := validateHall(hall); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := initOrCheckHall(db, hall); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initOrCheckHall(db *sql.DB, hall Hall) error {
	blocked := append([]Cell(nil), hall.Blocked...)
	cfg, err := json.Marshal(hallConfig{Rows: hall.Rows, Cols: hall.Cols, Blocked: blocked})
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing string
	err = tx.QueryRow(`SELECT value FROM meta WHERE key='hall'`).Scan(&existing)
	switch {
	case err == sql.ErrNoRows:
		if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES ('hall', ?)`, string(cfg)); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES ('max_time', '0')`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO meta (key, value) VALUES ('seq', '0')`); err != nil {
			return err
		}
		for _, c := range hall.Blocked {
			if _, err := tx.Exec(`INSERT INTO blocked (row, col) VALUES (?, ?)`, c.Row, c.Col); err != nil {
				return err
			}
		}
	case err != nil:
		return err
	default:
		if existing != string(cfg) {
			return ErrHallMismatch
		}
	}
	return tx.Commit()
}

// checkAndAdvanceTime enforces global monotonicity and returns the stored max as
// well as the (possibly updated) current max. Must be called inside a tx.
func checkAndAdvanceTime(tx *sql.Tx, t time.Time) error {
	var ns int64
	if err := tx.QueryRow(`SELECT value FROM meta WHERE key='max_time'`).Scan(&ns); err != nil {
		return err
	}
	tns := t.UTC().UnixNano()
	if tns < ns {
		return ErrTimeRegression
	}
	if tns > ns {
		if _, err := tx.Exec(`UPDATE meta SET value=? WHERE key='max_time'`, fmt.Sprint(tns)); err != nil {
			return err
		}
	}
	return nil
}

func nextSeq(tx *sql.Tx) (int64, error) {
	var s string
	if err := tx.QueryRow(`SELECT value FROM meta WHERE key='seq'`).Scan(&s); err != nil {
		return 0, err
	}
	var n int64
	if _, err := fmt.Sscan(s, &n); err != nil {
		return 0, err
	}
	n++
	if _, err := tx.Exec(`UPDATE meta SET value=? WHERE key='seq'`, fmt.Sprint(n)); err != nil {
		return 0, err
	}
	return n, nil
}

var errTODO = errors.New("not implemented")
