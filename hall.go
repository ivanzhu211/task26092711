package boothalloc

import "fmt"

// validate checks the hall definition.
func (h Hall) validate() error {
	if h.Rows < 1 || h.Rows > 26 {
		return fmt.Errorf("%w: rows must be in 1..26, got %d", ErrInvalidHall, h.Rows)
	}
	if h.Columns < 1 || h.Columns > 80 {
		return fmt.Errorf("%w: columns must be in 1..80, got %d", ErrInvalidHall, h.Columns)
	}
	seen := make(map[Cell]bool, len(h.Blocked))
	for _, c := range h.Blocked {
		if c.Row < 'A' || int(c.Row-'A') >= h.Rows || c.Column < 1 || c.Column > h.Columns {
			return fmt.Errorf("%w: blocked cell %s out of bounds", ErrInvalidHall, displayCell(c))
		}
		if seen[c] {
			return fmt.Errorf("%w: blocked cell %s listed more than once", ErrInvalidHall, displayCell(c))
		}
		seen[c] = true
	}
	return nil
}

// index converts a cell to its flat index in the grid.
func (h Hall) index(c Cell) (int, bool) {
	if c.Row < 'A' || int(c.Row-'A') >= h.Rows || c.Column < 1 || c.Column > h.Columns {
		return 0, false
	}
	return int(c.Row-'A')*h.Columns + (c.Column - 1), true
}

// cellID renders the booth id carried by cell idx (top-left convention).
func cellID(idx, columns int) string {
	row := rune('A' + idx/columns)
	col := idx%columns + 1
	return fmt.Sprintf("%c-%d", row, col)
}

// cellAt converts a flat index back to a Cell.
func cellAt(idx, columns int) Cell {
	return Cell{Row: rune('A' + idx/columns), Column: idx%columns + 1}
}

func displayCell(c Cell) string {
	return fmt.Sprintf("%c%d", c.Row, c.Column)
}
