package booth

import (
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func openTemp(t *testing.T, hall Hall) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "hall.db"), hall)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}

func TestSmoke(t *testing.T) {
	hall := Hall{Rows: 3, Cols: 4, Blocked: []Cell{{Row: 0, Col: 1}}}
	db := openTemp(t, hall)
	defer db.Close()

	res, err := db.Request(t0, "alice", Standard)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if res.Offer == nil || res.Offer.BoothID != "A-2" { // A-1 is blocked
		t.Fatalf("want offer A-2, got %+v", res.Offer)
	}
	if res.Offer.ExpiresAt != t0.Add(48*time.Hour) {
		t.Fatalf("expiry: %v", res.Offer.ExpiresAt)
	}
	acc, err := db.Accept(t0.Add(time.Hour), "alice")
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if acc.BoothID != "A-2" {
		t.Fatalf("accept: %v", acc)
	}
	if _, err := db.Request(t0.Add(2*time.Hour), "alice", Standard); err != ErrDuplicateRequest {
		t.Fatalf("want ErrDuplicateRequest, got %v", err)
	}
	if _, err := db.Request(t0.Add(time.Hour), "bob", Standard); err != ErrTimeRegression {
		t.Fatalf("want ErrTimeRegression, got %v", err)
	}
	snap, err := db.Snapshot(t0.Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Sold) != 1 || snap.Sold[0].BoothID != "A-2" || snap.Sold[0].Exhibitor != "alice" {
		t.Fatalf("sold: %+v", snap.Sold)
	}
	kind, err := db.Release(t0.Add(3*time.Hour), "alice")
	if err != nil || kind != ReleaseSold {
		t.Fatalf("release: %v %v", kind, err)
	}
	snap, _ = db.Snapshot(t0.Add(3 * time.Hour))
	if len(snap.Sold) != 0 || len(snap.Offers) != 0 || len(snap.Queue) != 0 {
		t.Fatalf("after release: %+v", snap)
	}
}

func TestOfferExpiryAndQueue(t *testing.T) {
	hall := Hall{Rows: 1, Cols: 1}
	db := openTemp(t, hall)
	defer db.Close()

	if _, err := db.Request(t0, "a", Standard); err != nil {
		t.Fatal(err)
	}
	res, _ := db.Request(t0, "b", Standard)
	if !res.Queued {
		t.Fatalf("b should be queued: %+v", res)
	}
	// a's offer expires at t0+48h; at exactly that moment b must be promoted.
	res2, err := db.Request(t0.Add(48*time.Hour), "c", Standard)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Queued {
		t.Fatalf("c should be queued behind b: %+v", res2)
	}
	snap, _ := db.Snapshot(t0.Add(48 * time.Hour))
	if len(snap.Offers) != 1 || snap.Offers[0].Exhibitor != "b" {
		t.Fatalf("b should hold the offer: %+v", snap.Offers)
	}
	if snap.Offers[0].ExpiresAt != t0.Add(96*time.Hour) {
		t.Fatalf("b expiry: %v", snap.Offers[0].ExpiresAt)
	}
	if len(snap.Queue) != 1 || snap.Queue[0].Exhibitor != "c" {
		t.Fatalf("queue: %+v", snap.Queue)
	}
	// Accept of expired offer "a" fails.
	if _, err := db.Accept(t0.Add(48*time.Hour), "a"); err != ErrNoOffer {
		t.Fatalf("want ErrNoOffer, got %v", err)
	}
}
