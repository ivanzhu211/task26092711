package booth

import (
	"fmt"
	"testing"
	"time"
)

// One call that must settle a backlog of thousands of chained expiry batches
// (offer expires -> queued request promoted -> new offer expires -> ...)
// has to return well within 2 seconds.
func TestSettleBacklogPerformance(t *testing.T) {
	hall := Hall{Rows: 1, Cols: 1}
	db := openTemp(t, hall)
	defer db.Close()

	if _, err := db.Request(t0, "seed", Standard); err != nil {
		t.Fatal(err)
	}
	const backlog = 2000
	for i := 0; i < backlog; i++ {
		res, err := db.Request(t0.Add(time.Duration(i+1)*time.Second), fmt.Sprintf("q%04d", i), Standard)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Queued {
			t.Fatalf("q%d should be queued", i)
		}
	}
	// Jump past every chained expiry: batch k expires at t0 + k*48h.
	far := t0.Add(time.Duration(backlog+2) * 48 * time.Hour)
	start := time.Now()
	snap, err := db.Snapshot(far)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Offers) != 0 || len(snap.Sold) != 0 || len(snap.Queue) != 0 {
		t.Fatalf("everything should have expired: %+v", snap)
	}
	t.Logf("settled %d chained batches in %v", backlog+1, elapsed)
	if elapsed > 2*time.Second {
		t.Fatalf("settlement took %v, over the 2s budget", elapsed)
	}
}
