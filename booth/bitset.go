package booth

import "math/bits"

// rowMask is a bitmask of up to 80 columns: lo bit i = col i+1 (1..64),
// hi bit i = col 65+i (65..80). A set bit means the cell is unavailable
// (blocked or occupied).
type rowMask struct {
	lo uint64
	hi uint64
}

func (m rowMask) test(col int) bool {
	if col <= 64 {
		return m.lo&(uint64(1)<<uint(col-1)) != 0
	}
	return m.hi&(uint64(1)<<uint(col-65)) != 0
}

func (m *rowMask) set(col int) {
	if col <= 64 {
		m.lo |= uint64(1) << uint(col-1)
	} else {
		m.hi |= uint64(1) << uint(col-65)
	}
}

// orShiftRight1 returns m | (m >> 1) across the 128-bit value; a zero bit at
// position col means cols col and col+1 are both free.
func orShiftRight1(m rowMask) rowMask {
	return rowMask{
		lo: m.lo | (m.lo >> 1) | (m.hi << 63),
		hi: m.hi | (m.hi >> 1),
	}
}

// firstZero returns the 1-based column of the lowest clear bit at or after
// col 1, or 0 if none within cols.
func firstZero(m rowMask, cols int) int {
	lo := m.lo
	if cols < 64 {
		lo |= maskAbove(cols)
	}
	if lo != ^uint64(0) {
		return bits.TrailingZeros64(^lo) + 1
	}
	if cols <= 64 {
		return 0
	}
	hi := m.hi
	if cols < 128 {
		hi |= maskAbove(cols - 64)
	}
	if hi != ^uint64(0) {
		return 64 + bits.TrailingZeros64(^hi) + 1
	}
	return 0
}

func maskAbove(n int) uint64 {
	if n >= 64 {
		return 0
	}
	return ^uint64(0) << uint(n)
}

// grid is an in-memory unavailable-cell bitmap for placement scans.
type grid struct {
	rows int
	cols int
	m    []rowMask
}

func newGrid(rows, cols int) *grid {
	return &grid{rows: rows, cols: cols, m: make([]rowMask, rows)}
}

func (g *grid) occupy(c Cell) { g.m[c.Row].set(c.Col) }

// findFirst returns the top-left anchor of the first position (scanning rows
// A..Z, then columns ascending) where a booth of size s fits, or ok=false.
func (g *grid) findFirst(s Size) (row, col int, ok bool) {
	switch s {
	case Standard:
		for r := 0; r < g.rows; r++ {
			if c := firstZero(g.m[r], g.cols); c != 0 {
				return r, c, true
			}
		}
	case Premium:
		if g.cols < 2 {
			return 0, 0, false
		}
		for r := 0; r < g.rows; r++ {
			if c := firstZero(orShiftRight1(g.m[r]), g.cols-1); c != 0 {
				return r, c, true
			}
		}
	case Island:
		if g.cols < 2 || g.rows < 2 {
			return 0, 0, false
		}
		for r := 0; r+1 < g.rows; r++ {
			pair := rowMask{lo: g.m[r].lo | g.m[r+1].lo, hi: g.m[r].hi | g.m[r+1].hi}
			if c := firstZero(orShiftRight1(pair), g.cols-1); c != 0 {
				return r, c, true
			}
		}
	}
	return 0, 0, false
}

// place marks the footprint of s anchored at (row, col) as occupied.
func (g *grid) place(s Size, row, col int) {
	for _, c := range footprintCells(s, row, col) {
		g.occupy(c)
	}
}

func (m *rowMask) clear(col int) {
	if col <= 64 {
		m.lo &^= uint64(1) << uint(col-1)
	} else {
		m.hi &^= uint64(1) << uint(col-65)
	}
}

// unplace frees the footprint of s anchored at (row, col). The footprint must
// not overlap blocked cells or any other booth.
func (g *grid) unplace(s Size, row, col int) {
	for _, c := range footprintCells(s, row, col) {
		g.m[c.Row].clear(c.Col)
	}
}

// footprintCells lists the cells covered by a booth of size s anchored at
// (row, col). The anchor must be a valid placement.
func footprintCells(s Size, row, col int) []Cell {
	switch s {
	case Standard:
		return []Cell{{Row: row, Col: col}}
	case Premium:
		return []Cell{{Row: row, Col: col}, {Row: row, Col: col + 1}}
	case Island:
		return []Cell{
			{Row: row, Col: col}, {Row: row, Col: col + 1},
			{Row: row + 1, Col: col}, {Row: row + 1, Col: col + 1},
		}
	}
	return nil
}
