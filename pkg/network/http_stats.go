// +build linux_bpf

package network

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/google/gopacket"
)

type httpStatKeeper struct {
	stats  map[httpKey]httpStats
	muxMap map[httpKey]*sync.Mutex // protects concurrent edits of a single streamStat

	// Telemetry
	messagesRead int64
	readErrors   int64
}

type httpKey struct {
	net, transport gopacket.Flow
}

type httpStats struct {
	sourceIP   gopacket.Endpoint
	destIP     gopacket.Endpoint
	sourcePort gopacket.Endpoint
	destPort   gopacket.Endpoint

	orderedEvents *httpEventHeap

	numRequests  int64
	numResponses int64
	successes    int64
	errors       int64
}

func (s httpStats) getEventsAndLatencies() ([]string, []time.Duration) {
	var events []string
	var latencies []time.Duration

	if s.orderedEvents.Len() == 0 {
		return events, latencies
	}

	lastReqTime := time.Time{}
	tempHeap := &httpEventHeap{}

	for s.orderedEvents.Len() > 0 {
		event := heap.Pop(s.orderedEvents).(httpEvent)

		latency := event.timestamp().Sub(lastReqTime)
		latencies = append(latencies, latency)
		lastReqTime = event.timestamp()

		if req, ok := event.(*httpRequest); ok {
			events = append(events, req.method+fmt.Sprintf(" (%v bytes)", req.bodyBytes))
		}

		if res, ok := event.(*httpResponse); ok {
			events = append(events, res.status+fmt.Sprintf(" (%v bytes)", res.bodyBytes))
		}

		heap.Push(tempHeap, event)
	}

	for tempHeap.Len() > 0 {
		event := heap.Pop(tempHeap)
		heap.Push(s.orderedEvents, event)
	}

	// the first latency value is garbage
	latencies = latencies[1:]

	return events, latencies
}
