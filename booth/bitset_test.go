package booth

import "testing"

func TestFindFirstBasic(t *testing.T) {
	g := newGrid(3, 4)
	if r, c, ok := g.findFirst(Standard); !ok || r != 0 || c != 1 {
		t.Fatalf("empty grid standard: got %d,%d,%v", r, c, ok)
	}
	g.place(Standard, 0, 1)
	if r, c, ok := g.findFirst(Standard); !ok || r != 0 || c != 2 {
		t.Fatalf("after occupy: got %d,%d,%v", r, c, ok)
	}
	g.place(Island, 0, 1) // occupies (0,1)(0,2)(1,1)(1,2)
	if r, c, ok := g.findFirst(Premium); !ok || r != 0 || c != 3 {
		t.Fatalf("premium: got %d,%d,%v", r, c, ok)
	}
	if r, c, ok := g.findFirst(Island); !ok || r != 0 || c != 3 {
		t.Fatalf("island: got %d,%d,%v", r, c, ok)
	}
}

func TestFindFirstWordBoundary(t *testing.T) {
	g := newGrid(2, 80)
	// Fill everything except cols 64-65 in row 0.
	for c := 1; c <= 80; c++ {
		if c == 64 || c == 65 {
			continue
		}
		g.occupy(Cell{Row: 0, Col: c})
	}
	for c := 1; c <= 80; c++ {
		g.occupy(Cell{Row: 1, Col: c})
	}
	if r, c, ok := g.findFirst(Premium); !ok || r != 0 || c != 64 {
		t.Fatalf("premium across word boundary: got %d,%d,%v", r, c, ok)
	}
	if _, _, ok := g.findFirst(Island); ok {
		t.Fatalf("island should not fit with row 1 full")
	}
	if r, c, ok := g.findFirst(Standard); !ok || r != 0 || c != 64 {
		t.Fatalf("standard: got %d,%d,%v", r, c, ok)
	}
	g.place(Premium, 0, 64)
	if _, _, ok := g.findFirst(Standard); ok {
		t.Fatalf("grid should be full")
	}
}

func TestFullRows(t *testing.T) {
	g := newGrid(26, 80)
	for r := 0; r < 26; r++ {
		for c := 1; c <= 80; c++ {
			g.occupy(Cell{Row: r, Col: c})
		}
	}
	for _, s := range []Size{Standard, Premium, Island} {
		if _, _, ok := g.findFirst(s); ok {
			t.Fatalf("size %d should not fit in full grid", s)
		}
	}
}
