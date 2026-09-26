package booth

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type scriptCmd struct {
	at   time.Time
	op   string // "request" | "accept" | "release"
	who  string
	size Size
}

func fmtResult(v interface{}, err error) string {
	if err != nil {
		return "ERR " + err.Error()
	}
	switch r := v.(type) {
	case RequestResult:
		if r.Offer == nil {
			return fmt.Sprintf("{Queued:%v}", r.Queued)
		}
		return fmt.Sprintf("{Offer:%s}", fmtOffer(*r.Offer))
	case Snapshot:
		var b strings.Builder
		b.WriteString("{Sold:[")
		for i, s := range r.Sold {
			if i > 0 {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%s@%s/%d", s.Exhibitor, s.BoothID, s.Size)
		}
		b.WriteString("] Offers:[")
		for i, o := range r.Offers {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(fmtOffer(o))
		}
		b.WriteString("] Queue:[")
		for i, q := range r.Queue {
			if i > 0 {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "%s/%d", q.Exhibitor, q.Size)
		}
		b.WriteString("]}")
		return b.String()
	}
	return fmt.Sprintf("%+v", v)
}

func fmtOffer(o Offer) string {
	return fmt.Sprintf("%s@%s/%d exp=%s", o.Exhibitor, o.BoothID, o.Size,
		o.ExpiresAt.Format(time.RFC3339))
}

func execCmd(db *DB, c scriptCmd) string {
	switch c.op {
	case "request":
		res, err := db.Request(c.at, c.who, c.size)
		return "REQ " + c.who + " " + fmtResult(res, err)
	case "accept":
		res, err := db.Accept(c.at, c.who)
		return "ACC " + c.who + " " + fmtResult(res, err)
	case "release":
		res, err := db.Release(c.at, c.who)
		return "REL " + c.who + " " + fmtResult(res, err)
	}
	panic("bad op " + c.op)
}

// runScript executes the script against a fresh store and returns the
// formatted result of every command plus a final snapshot. When interleave
// is set, extra Snapshot calls at assorted times are inserted between
// commands and the store is closed and reopened halfway through; their
// snapshot outputs are returned too (prefixed with SNAP).
func runScript(t *testing.T, hall Hall, script []scriptCmd, finalT time.Time, interleave bool) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hall.db")
	db, err := Open(path, hall)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var out []string
	prev := script[0].at
	reopened := false
	for i, c := range script {
		if interleave {
			for _, st := range []time.Time{prev, prev.Add(c.at.Sub(prev) / 2), c.at} {
				snap, err := db.Snapshot(st)
				out = append(out, "SNAP "+fmtResult(snap, err))
			}
			if !reopened && i == len(script)/2 {
				if err := db.Close(); err != nil {
					t.Fatalf("Close: %v", err)
				}
				db, err = Open(path, hall)
				if err != nil {
					t.Fatalf("reopen: %v", err)
				}
				reopened = true
			}
		}
		out = append(out, execCmd(db, c))
		prev = c.at
	}
	snap, err := db.Snapshot(finalT)
	out = append(out, "FINAL "+fmtResult(snap, err))
	if err := db.Close(); err != nil {
		t.Fatalf("final Close: %v", err)
	}
	return out
}

// checkDeterminism runs the script plainly and with interleaved snapshots
// plus a close/reopen, and requires every command result and the final
// snapshot to be identical.
func checkDeterminism(t *testing.T, hall Hall, script []scriptCmd, finalT time.Time) []string {
	t.Helper()
	plain := runScript(t, hall, script, finalT, false)
	mixed := runScript(t, hall, script, finalT, true)
	var plainCmds, mixedCmds []string
	for _, s := range plain {
		if !strings.HasPrefix(s, "SNAP ") {
			plainCmds = append(plainCmds, s)
		}
	}
	for _, s := range mixed {
		if !strings.HasPrefix(s, "SNAP ") {
			mixedCmds = append(mixedCmds, s)
		}
	}
	if !reflect.DeepEqual(plainCmds, mixedCmds) {
		for i := 0; i < len(plainCmds) || i < len(mixedCmds); i++ {
			var a, b string
			if i < len(plainCmds) {
				a = plainCmds[i]
			}
			if i < len(mixedCmds) {
				b = mixedCmds[i]
			}
			if a != b {
				t.Errorf("cmd %d differs:\n  plain: %s\n  mixed: %s", i, a, b)
			}
		}
		t.FailNow()
	}
	return mixedCmds
}

// Scenario 1: an Island offer expires while nobody calls; the freed cells are
// taken by queued Standards whose offers then also expire while nobody calls;
// a later Accept of one of them must be rejected.
func TestDeterminismExpiryChain(t *testing.T) {
	hall := Hall{Rows: 2, Cols: 2}
	h := time.Hour
	script := []scriptCmd{
		{t0, "request", "island", Island},
		{t0.Add(1 * h), "request", "s1", Standard},
		{t0.Add(2 * h), "request", "s2", Standard},
		{t0.Add(3 * h), "request", "s3", Standard},
		// Nobody calls between t0+3h and t0+100h. The island offer expires at
		// t0+48h, s1..s3 are promoted then, and their offers expire at t0+96h.
		{t0.Add(100 * h), "accept", "s1", 0},
		{t0.Add(101 * h), "request", "late", Standard},
	}
	out := checkDeterminism(t, hall, script, t0.Add(200*h))

	want := []string{
		`REQ island {Offer:island@A-1/3 exp=2026-03-03T12:00:00Z}`,
		`REQ s1 {Queued:true}`,
		`REQ s2 {Queued:true}`,
		`REQ s3 {Queued:true}`,
		`ACC s1 ERR booth: no live offer for exhibitor`,
		`REQ late {Offer:late@A-1/1 exp=2026-03-07T17:00:00Z}`,
	}
	if len(out) < len(want) {
		t.Fatalf("short output: %v", out)
	}
	for i, w := range want {
		if out[i] != w {
			t.Errorf("out[%d] = %q, want %q", i, out[i], w)
		}
	}
	final := out[len(out)-1]
	wantFinal := `FINAL {Sold:[] Offers:[] Queue:[]}` // late's offer expired at t0+149h
	if final != wantFinal {
		t.Errorf("final snapshot:\n got  %s\n want %s", final, wantFinal)
	}
}

// Scenario 2: several offers expire at the same moment; only their combined
// release fits a queued Island. If they were released one at a time, a
// Standard queued behind the Island would grab the cells first.
func TestDeterminismBatchExpiry(t *testing.T) {
	hall := Hall{Rows: 2, Cols: 3}
	h := time.Hour
	script := []scriptCmd{
		{t0, "request", "p1", Premium},  // offer A-1 (A-1,A-2)
		{t0, "request", "p2", Premium},  // offer B-1 (B-1,B-2)
		{t0, "request", "x1", Standard}, // offer A-3
		{t0, "request", "x2", Standard}, // offer B-3
		{t0.Add(1 * h), "accept", "x1", 0},
		{t0.Add(1 * h), "accept", "x2", 0},
		{t0.Add(2 * h), "request", "isl", Island},   // queued: no 2x2 free
		{t0.Add(2 * h), "request", "std", Standard}, // queued behind isl
		// p1 and p2 both expire at t0+48h. Released together they free the
		// 2x2 block A-1..B-2, so isl gets an offer; released one by one, std
		// would grab A-1 first and isl would never fit.
		{t0.Add(50 * h), "accept", "isl", 0},
	}
	out := checkDeterminism(t, hall, script, t0.Add(60*h))

	accIsl := out[len(out)-2] // before FINAL
	want := `ACC isl {BoothID:A-1 Size:3}`
	if accIsl != want {
		t.Errorf("accept isl = %q, want %q", accIsl, want)
	}
	final := out[len(out)-1]
	wantFinal := `FINAL {Sold:[isl@A-1/3 x1@A-3/1 x2@B-3/1] Offers:[] Queue:[std/1]}`
	if final != wantFinal {
		t.Errorf("final snapshot:\n got  %s\n want %s", final, wantFinal)
	}
}
