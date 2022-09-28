package api

import (
	"fmt"
	"sync"

	"github.com/ef-ds/deque"
)

type queue struct {
	mu   sync.Mutex
	data deque.Deque
}

func (q *queue) push(payload *Payload) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.data.PushBack(payload)
}

func (q *queue) pop() (*Payload, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	size := q.data.Len()
	if size == 0 {
		return nil, nil
	}
	data, ok := q.data.PopFront()
	if !ok {
		return nil, fmt.Errorf("queue: PopFront failure. queuesize=%d", size)
	}
	ret, ok := data.(*Payload)
	if !ok {
		return nil, fmt.Errorf("%v is not of the type *Payload", ret)
	}
	return ret, nil
}
