// +build linux_bpf

package network

import (
	"sync"
	"time"

	"github.com/google/gopacket"
)

type httpStatKeeper struct {
	stats *sync.Map
	// sync.Map guarantees safety for multiple goroutines to read/write
	// map entries for disjoint keys without the use of locks

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

func (s httpStats) getLatencies() []time.Duration {
	var latencies []time.Duration

	if s.orderedEvents.Len() <= 1 {
		return latencies
	}

	lastEventTime := time.Time{}
	for _, event := range *s.orderedEvents {
		latency := event.timestamp().Sub(lastEventTime)
		latencies = append(latencies, latency)
		lastEventTime = event.timestamp()
	}

	// the first latency value is garbage
	latencies = latencies[1:]

	return latencies
}
