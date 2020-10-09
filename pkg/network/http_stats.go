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
	var requestsQ []*httpRequest

	for _, event := range *s.orderedEvents {
		if req, ok := event.(*httpRequest); ok {
			requestsQ = append(requestsQ, req)
			continue
		}

		if res, ok := event.(*httpResponse); ok {
			if len(requestsQ) == 0 {
				continue
			}

			oldestReq := requestsQ[0]
			requestsQ = requestsQ[1:]

			latency := res.timestamp().Sub(oldestReq.timestamp())
			latencies = append(latencies, latency)
		}
	}

	return latencies
}
