// +build linux_bpf

package network

import (
	"container/heap"
	"time"
)

type httpEvent interface {
	timestamp() time.Time
}

// httpEventHeap is a min-heap of http events ordered by timestamp
type httpEventHeap []httpEvent

func (h httpEventHeap) Len() int            { return len(h) }
func (h httpEventHeap) Less(i, j int) bool  { return h[i].timestamp().Before(h[j].timestamp()) }
func (h httpEventHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *httpEventHeap) Push(x interface{}) { *h = append(*h, x.(httpEvent)) }
func (h *httpEventHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

var _ heap.Interface = &httpEventHeap{}

type httpRequest struct {
	method    string
	bodyBytes int
	ts        time.Time
}

func (r httpRequest) timestamp() time.Time { return r.ts }

var _ httpEvent = httpRequest{}

type httpResponse struct {
	status     string
	statusCode int
	bodyBytes  int
	ts         time.Time
}

func (r httpResponse) timestamp() time.Time { return r.ts }

var _ httpEvent = httpResponse{}
