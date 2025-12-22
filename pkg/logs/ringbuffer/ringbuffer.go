package ringbuffer

import "sync"

type RingBufferInterface[T any] interface {
	Push(T)
	Size() uint64
	Capacity() uint64
	ReadAll() []T
	Clear()
}

// RingBuffer is a thread-safe implementation of RingBufferInterface.
type RingBuffer[T any] struct {
	buffer   []T
	capacity uint64
	head     uint64
	tail     uint64
	size     uint64
	mu       sync.RWMutex
}

// compile time check
var _ RingBufferInterface[struct{}] = (*RingBuffer[struct{}])(nil)

func NewRingBuffer[T any](capacity uint64) *RingBuffer[T] {
	return &RingBuffer[T]{
		capacity: capacity,
		buffer: make(
			[]T,
			capacity,
		),
		head: 0,
		tail: 0,
		size: 0,
	}
}

// Push adds an element to the ring buffer.
func (r *RingBuffer[T]) Push(element T) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buffer[r.tail] = element
	r.tail = (r.tail + 1) % r.capacity

	if r.size < r.capacity {
		r.size++
	} else {
		r.head = (r.head + 1) % r.capacity
	}
}

// Size returns the size of the buffer.
func (r *RingBuffer[T]) Size() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// Capacity returns capacity of the buffer.
func (r *RingBuffer[T]) Capacity() uint64 {
	return r.capacity
}

// ReadAll returns a copy of the buffer.
// The length of the returned slice will be the capacity
// of the buffer.
func (r *RingBuffer[T]) ReadAll() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	elements := make(
		[]T,
		r.capacity,
	)
	copy(
		elements,
		r.buffer,
	)
	return elements
}

func (r *RingBuffer[T]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.buffer)
	r.size = 0
	r.head = 0
	r.tail = 0
}
