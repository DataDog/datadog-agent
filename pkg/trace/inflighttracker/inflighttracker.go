package inflighttracker

import (
	"sync/atomic"
)

type DoneFunc func()

func noop() {}

type Tracker struct {
	size  int64
	alloc atomic.Int64
}

// New creates a new Tracker
func New(size int64) *Tracker {
	tracker := Tracker{
		size: size,
	}
	return &tracker
}

func (t *Tracker) Enabled() bool {
	return t.size > 0
}

func (t *Tracker) Size() int64 {
	return t.size
}

func (t *Tracker) Alloc() int64 {
	return t.alloc.Load()
}

// Free returns the amount of free (available) bytes
func (t *Tracker) Free() int64 {
	return t.size - t.alloc.Load()
}

// Track allocates the amount of bytes in the tracker, and returns the function that frees them
func (t *Tracker) Track(size int64) DoneFunc {
	if !t.Enabled() {
		return noop
	}
	t.alloc.Add(size)
	return func() { t.alloc.Add(-size) }
}
