package boothalloc

import "errors"

// tryPlace finds a fitting set of free cells.
func (s *Store) tryPlace(size Size) ([]int, bool) {
	switch size {
	case Standard:
		for r := 0; r < s.rows; r++ {
			mask := s.free[r]
			if mask == 0 {
				continue
			}
			b := bitIndex(mask)
			return []int{r*s.cols + b}, true
		}
		return nil, false
	case Premium:
		for r := 0; r < s.rows; r++ {
			mask := s.free[r]
			// Adjacent pair (b, b+1); bit b+1 must be within the grid.
			pairs := mask & (mask << 1) & (s.maxColMask() << 1)
			if pairs == 0 {
				continue
			}
		b := bitIndex(pairs)
		base := r*s.cols + b
		return []int{base, base + 1}, true
		}
		return nil, false
	case Island:
		for r := 0; r+1 < s.rows; r++ {
			mask := s.free[r] & s.free[r+1]
			pairs := mask & (mask << 1) & (s.maxColMask() << 1)
			if pairs == 0 {
				continue
		}
		b := bitIndex(pairs) - 1
			top := r*s.cols + b
			return []int{top, top + 1, top + s.cols, top + s.cols + 1}, true
		}
		return nil, false
	default:
		return nil, false
	}
}

// markUsed removes cells from the free set.
func (s *Store) markUsed(cells []int) {
	for _, idx := range cells {
		s.free[idx/s.cols] &^= uint64(1) << (idx % s.cols)
	}
}

// markFree returns cells to the free set.
func (s *Store) markFree(cells []int) {
	for _, idx := range cells {
		s.free[idx/s.cols] |= uint64(1) << (idx % s.cols)
	}
}

// maxColMask has bits 0..cols-2 set, i.e. all valid left columns of a width-2 shape.
func (s *Store) maxColMask() uint64 {
	if s.cols <= 1 {
		return 0
	}
	return uint64(1)<<(s.cols-1) - 1
}

func bitIndex(mask uint64) int {
	for b := 0; mask != 0; b++ {
		if mask&1 == 1 {
			return b
		}
		mask >>= 1
	}
	panic(errors.New("boothalloc: bitIndex called on zero mask"))
}
