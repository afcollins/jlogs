package parser

// ring is a fixed-capacity circular buffer of Occurrence values, equivalent to
// Python's collections.deque(maxlen=N). Once full, push overwrites the oldest
// entry. snapshot returns the contents in insertion order (oldest first), which
// matches the original Python behavior when the deque is converted to a list.
//
// We use a custom ring rather than a slice with re-slicing because:
//   - it avoids reallocating on every push past capacity, and
//   - it makes the bounded-memory guarantee explicit in the type.
type ring struct {
	buf   []Occurrence
	start int  // index of the oldest element, when full
	size  int  // number of elements currently held
	cap   int  // capacity
	full  bool // once true, start tracks the oldest slot
}

func newRing(capacity int) *ring {
	return &ring{
		buf: make([]Occurrence, capacity),
		cap: capacity,
	}
}

func (r *ring) push(o Occurrence) {
	if !r.full {
		r.buf[r.size] = o
		r.size++
		if r.size == r.cap {
			r.full = true
			r.start = 0
		}
		return
	}
	// Overwrite oldest, advance start.
	r.buf[r.start] = o
	r.start = (r.start + 1) % r.cap
}

// snapshot returns a fresh slice with elements in oldest-to-newest order.
// The returned slice is owned by the caller.
func (r *ring) snapshot() []Occurrence {
	out := make([]Occurrence, r.size)
	if !r.full {
		copy(out, r.buf[:r.size])
		return out
	}
	// Full ring: start..end-of-buf, then 0..start.
	n := copy(out, r.buf[r.start:])
	copy(out[n:], r.buf[:r.start])
	return out
}
