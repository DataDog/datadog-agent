// +build linux_bpf

package http

import (
	"sort"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// incompleteBuffer is responsible for buffering incomplete transactions
// (eg. httpTX objects that have either only the request or response information)
// this happens only in the context of localhost traffic with NAT.
// Imagine, for example, that you have two containers in the same host:
// * Container A: 1.1.1.1
// * Container B: 3.3.3.3
// And a NAT rule: 2.2.2.2 -> 3.3.3.3
// Now let's say Container A issues a HTTP request to 2.2.2.2;
// The eBPF socket filter program will see two "disjoint" TCP segments:
// (1.1.1.1:ephemeral-port -> 2.2.2.2:server-port) GET / HTTP/1.1
// (3.3.3.3 -> 1.1.1.1:ephemeral-port) HTTP/1.1 200 OK
// Because of that, we join these two parts of the transaction here in userspace using the
// client address (1.1.1.1:ephemeral-port)
// There is, however, another variable that can further complicate this: keep-alives
// You could have, for example, one TCP socket issuing multiple requests:
// t0: (1.1.1.1:ephemeral-port -> 2.2.2.2:server-port) GET / HTTP/1.1
// t1: (3.3.3.3 -> 1.1.1.1:ephemeral-port) HTTP/1.1 200 OK
// t2: (1.1.1.1:ephemeral-port -> 2.2.2.2:server-port) GET / HTTP/1.1
// t3: (3.3.3.3 -> 1.1.1.1:ephemeral-port) HTTP/1.1 200 OK
// The problem is, due to the way our eBPF batching works, there is no guarantee that these
// incomplete events will be read in the order they happened, so if we had a greedy approach
// that joined events as soon as they're sent from eBPF, we could potentially join
// request segment at "t0" with response segment "t3". This is why we buffer data here for 30 seconds
// and then sort all events by their timestamps before joining them.
type incompleteBuffer struct {
	data       map[Key]*txParts
	maxEntries int
	telemetry  *telemetry
}

type txParts struct {
	requests  []httpTX
	responses []httpTX
}

func newIncompleteBuffer(c *config.Config, telemetry *telemetry) *incompleteBuffer {
	return &incompleteBuffer{
		data:       make(map[Key]*txParts),
		maxEntries: c.MaxHTTPStatsBuffered,
		telemetry:  telemetry,
	}
}

func (b *incompleteBuffer) Add(tx httpTX) {
	key := Key{
		SrcIPHigh: uint64(tx.tup.saddr_h),
		SrcIPLow:  uint64(tx.tup.saddr_l),
		SrcPort:   uint16(tx.tup.sport),
	}

	parts, ok := b.data[key]
	if !ok {
		if len(b.data) >= b.maxEntries {
			atomic.AddInt64(&b.telemetry.dropped, 1)
			return
		}

		parts = &txParts{
			requests:  make([]httpTX, 0, 5),
			responses: make([]httpTX, 0, 5),
		}

		b.data[key] = parts
	}

	if tx.StatusClass() == 0 {
		parts.requests = append(parts.requests, tx)
	} else {
		parts.responses = append(parts.responses, tx)
	}
}

func (b *incompleteBuffer) Flush() []*httpTX {
	var joined []*httpTX

	for _, parts := range b.data {
		// TODO: in this loop we're sorting all transactions at once, but we could also
		// consider sorting data during insertion time (using a tree-like structure, for example)
		sort.Sort(byRequestTime(parts.requests))
		sort.Sort(byResponseTime(parts.responses))

		for i := 0; i < len(parts.requests) && i < len(parts.responses); i++ {
			request := &parts.requests[i]
			response := &parts.responses[i]
			if request.request_started > response.response_last_seen {
				break
			}

			// Merge response into request
			request.response_status_code = response.response_status_code
			request.response_last_seen = response.response_last_seen
			joined = append(joined, request)
		}
	}

	b.data = make(map[Key]*txParts)
	return joined
}

type byRequestTime []httpTX

func (rt byRequestTime) Len() int           { return len(rt) }
func (rt byRequestTime) Swap(i, j int)      { rt[i], rt[j] = rt[j], rt[i] }
func (rt byRequestTime) Less(i, j int) bool { return rt[i].request_started < rt[j].request_started }

type byResponseTime []httpTX

func (rt byResponseTime) Len() int      { return len(rt) }
func (rt byResponseTime) Swap(i, j int) { rt[i], rt[j] = rt[j], rt[i] }
func (rt byResponseTime) Less(i, j int) bool {
	return rt[i].response_last_seen < rt[j].response_last_seen
}
