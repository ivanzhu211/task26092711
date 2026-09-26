package booth

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReleasePromotesQueue(t *testing.T) {
	hall := Hall{Rows: 1, Cols: 2}
	db := openTemp(t, hall)
	defer db.Close()

	if _, err := db.Request(t0, "a", Premium); err != nil { // fills the row
		t.Fatal(err)
	}
	if _, err := db.Accept(t0, "a"); err != nil {
		t.Fatal(err)
	}
	res, _ := db.Request(t0.Add(time.Hour), "b", Standard)
	if !res.Queued {
		t.Fatalf("b queued: %+v", res)
	}
	kind, err := db.Release(t0.Add(2*time.Hour), "a")
	if err != nil || kind != ReleaseSold {
		t.Fatalf("release: %v %v", kind, err)
	}
	snap, _ := db.Snapshot(t0.Add(2 * time.Hour))
	if len(snap.Offers) != 1 || snap.Offers[0].Exhibitor != "b" ||
		snap.Offers[0].ExpiresAt != t0.Add(50*time.Hour) {
		t.Fatalf("b promoted with expiry t+48h: %+v", snap.Offers)
	}
	kind, _ = db.Release(t0.Add(3*time.Hour), "nobody")
	if kind != ReleaseNone {
		t.Fatalf("release none: %v", kind)
	}
}

func TestReleaseWaitingAndDecline(t *testing.T) {
	hall := Hall{Rows: 1, Cols: 1}
	db := openTemp(t, hall)
	defer db.Close()

	db.Request(t0, "a", Standard)
	db.Request(t0.Add(time.Hour), "b", Standard) // queued
	kind, _ := db.Release(t0.Add(2*time.Hour), "b")
	if kind != ReleaseWaiting {
		t.Fatalf("kind: %v", kind)
	}
	kind, _ = db.Release(t0.Add(3*time.Hour), "a")
	if kind != ReleaseOffer {
		t.Fatalf("kind: %v", kind)
	}
	// Declined offer frees the cell; nobody queued, so the hall is empty.
	snap, _ := db.Snapshot(t0.Add(3 * time.Hour))
	if len(snap.Offers) != 0 || len(snap.Queue) != 0 {
		t.Fatalf("empty: %+v", snap)
	}
	// b is out for good and may request again.
	res, err := db.Request(t0.Add(4*time.Hour), "b", Standard)
	if err != nil || res.Offer == nil {
		t.Fatalf("b re-request: %+v %v", res, err)
	}
}

func TestReopenAndHallMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hall.db")
	hall := Hall{Rows: 2, Cols: 2, Blocked: []Cell{{Row: 1, Col: 1}}}
	db, err := Open(path, hall)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Request(t0, "a", Standard); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Accept(t0, "a"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(path, hall)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	snap, err := db2.Snapshot(t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sold) != 1 || snap.Sold[0].Exhibitor != "a" || snap.Sold[0].BoothID != "A-1" {
		t.Fatalf("state survived reopen: %+v", snap.Sold)
	}
	db2.Close()

	other := Hall{Rows: 2, Cols: 3}
	if _, err := Open(path, other); err != ErrHallMismatch {
		t.Fatalf("want ErrHallMismatch, got %v", err)
	}
	other = Hall{Rows: 2, Cols: 2, Blocked: []Cell{{Row: 0, Col: 1}}}
	if _, err := Open(path, other); err != ErrHallMismatch {
		t.Fatalf("want ErrHallMismatch for blocked diff, got %v", err)
	}
}

func TestInvalidInput(t *testing.T) {
	db := openTemp(t, Hall{Rows: 1, Cols: 1})
	defer db.Close()
	if _, err := db.Request(t0, "x", Size(9)); err == nil {
		t.Fatal("invalid size accepted")
	}
	if _, err := db.Request(t0, "", Standard); err == nil {
		t.Fatal("empty exhibitor accepted")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "bad.db"), Hall{Rows: 27, Cols: 1}); err == nil {
		t.Fatal("oversized hall accepted")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "bad2.db"), Hall{Rows: 1, Cols: 1, Blocked: []Cell{{Row: 5, Col: 1}}}); err == nil {
		t.Fatal("out-of-range blocked cell accepted")
	}
}
