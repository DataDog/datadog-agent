// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux && bpf

package http

import (
	"cmp"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

const (
	defaultMinAge    = 30 * time.Second
	defaultArraySize = 5
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
	data map[types.ConnectionKey]*txParts
	// maxEntries caps the transactions buffered across all connections, not the number of connections.
	maxEntries int
	size       int
	// addsSinceJoin counts Add calls, dropped ones included, since the last joinBuffered.
	addsSinceJoin int
	// changed holds the connections that buffered a half since the last joinBuffered; only they can join.
	changed    map[types.ConnectionKey]struct{}
	telemetry  *Telemetry
	minAgeNano int64
}

type txParts struct {
	requests  []Transaction
	responses []Transaction
}

func newTXParts(requestCapacity, responseCapacity int) *txParts {
	return &txParts{
		requests:  make([]Transaction, 0, requestCapacity),
		responses: make([]Transaction, 0, responseCapacity),
	}
}

// NewIncompleteBuffer returns a new incompleteBuffer instance
func NewIncompleteBuffer(c *config.Config, telemetry *Telemetry) IncompleteBuffer {
	return &incompleteBuffer{
		data:       make(map[types.ConnectionKey]*txParts),
		changed:    make(map[types.ConnectionKey]struct{}),
		maxEntries: c.MaxHTTPStatsBuffered,
		telemetry:  telemetry,
		minAgeNano: defaultMinAge.Nanoseconds(),
	}
}

// Add buffers tx. When the buffer is full, it periodically joins the buffered pairs that a later request on
// the same connection confirms and returns them, so paired traffic is not dropped while waiting for Flush.
func (b *incompleteBuffer) Add(tx Transaction) []Transaction {
	// copy underlying httpTX value. this is now needed because these objects are
	// now coming directly from pooled perf records
	ebpfTX, ok := tx.(*EbpfEvent)
	if !ok {
		// should never happen
		return nil
	}

	var joined []Transaction
	// Join at most once per maxEntries/16 Add calls to bound the cost of re-sorting changed connections.
	if b.size >= b.maxEntries && b.addsSinceJoin >= b.maxEntries/16 {
		joined = b.joinBuffered()
		b.addsSinceJoin = 0
	}
	b.addsSinceJoin++
	if b.size >= b.maxEntries {
		b.telemetry.dropped.Add(1)
		return joined
	}

	connTuple := tx.ConnTuple()
	key := types.ConnectionKey{
		SrcIPHigh: connTuple.SrcIPHigh,
		SrcIPLow:  connTuple.SrcIPLow,
		SrcPort:   connTuple.SrcPort,
	}

	parts, ok := b.data[key]
	if !ok {
		parts = newTXParts(defaultArraySize, defaultArraySize)
		b.data[key] = parts
	}

	ebpfTxCopy := new(EbpfEvent)
	*ebpfTxCopy = *ebpfTX
	tx = ebpfTxCopy
	b.size++
	b.changed[key] = struct{}{}

	if tx.StatusCode() == 0 {
		b.telemetry.joiner.requests.Add(1)
		parts.requests = append(parts.requests, tx)
	} else {
		b.telemetry.joiner.responses.Add(1)
		parts.responses = append(parts.responses, tx)
	}
	return joined
}

// joinBuffered joins a buffered request with the earliest response after it only when the connection's next
// buffered request started after that response: an HTTP/1.1 client sends the next request once the previous
// response arrives, so a response read later cannot belong to the earlier request. Other halves stay buffered.
func (b *incompleteBuffer) joinBuffered() []Transaction {
	var joined []Transaction
	for key := range b.changed {
		parts := b.data[key]
		slices.SortFunc(parts.requests, func(a, b Transaction) int { return cmp.Compare(a.RequestStarted(), b.RequestStarted()) })
		slices.SortFunc(parts.responses, func(a, b Transaction) int { return cmp.Compare(a.ResponseLastSeen(), b.ResponseLastSeen()) })

		var unpaired []Transaction
		i, j := 0, 0
		for i+1 < len(parts.requests) && j < len(parts.responses) {
			request := parts.requests[i]
			response := parts.responses[j]
			if request.RequestStarted() > response.ResponseLastSeen() {
				unpaired = append(unpaired, response)
				j++
				continue
			}
			if parts.requests[i+1].RequestStarted() <= response.ResponseLastSeen() {
				break
			}
			request.SetStatusCode(response.StatusCode())
			request.SetResponseLastSeen(response.ResponseLastSeen())
			joined = append(joined, request)
			i++
			j++
			b.telemetry.joiner.requestJoined.Add(1)
		}
		if i == 0 {
			continue
		}

		kept := newTXParts(len(parts.requests)-i, len(unpaired)+len(parts.responses)-j)
		kept.requests = append(kept.requests, parts.requests[i:]...)
		kept.responses = append(append(kept.responses, unpaired...), parts.responses[j:]...)
		b.data[key] = kept
		b.size -= len(parts.requests) + len(parts.responses) - len(kept.requests) - len(kept.responses)
	}
	clear(b.changed)
	return joined
}

func (b *incompleteBuffer) Flush() []Transaction {
	var (
		joined   []Transaction
		previous = b.data
	)

	nowUnix, err := ebpf.NowNanoseconds()
	if err != nil {
		nowUnix = 0
	}

	b.data = make(map[types.ConnectionKey]*txParts)
	b.size = 0
	clear(b.changed)
	for key, parts := range previous {
		// TODO: in this loop we're sorting all transactions at once, but we could also
		// consider sorting data during insertion time (using a tree-like structure, for example)
		slices.SortFunc(parts.requests, func(a, b Transaction) int { return cmp.Compare(a.RequestStarted(), b.RequestStarted()) })
		slices.SortFunc(parts.responses, func(a, b Transaction) int { return cmp.Compare(a.ResponseLastSeen(), b.ResponseLastSeen()) })

		i := 0
		j := 0
		for i < len(parts.requests) && j < len(parts.responses) {
			request := parts.requests[i]
			response := parts.responses[j]
			if request.RequestStarted() > response.ResponseLastSeen() {
				b.telemetry.joiner.responsesDropped.Add(1)
				j++
				continue
			}

			// Merge response into request
			request.SetStatusCode(response.StatusCode())
			request.SetResponseLastSeen(response.ResponseLastSeen())
			joined = append(joined, request)
			i++
			j++
			b.telemetry.joiner.requestJoined.Add(1)
		}

		// now that we have finished matching requests and responses
		// we check if we should keep orphan requests a little longer
		for i < len(parts.requests) {
			if b.shouldKeep(parts.requests[i], nowUnix) {
				// if `i` is 0, then we are keeping all requests and zeroing the responses.
				// We're dropping the responses as either they are too old, or already matched to a request by the loop
				// above.
				if i == 0 {
					b.data[key] = parts
					b.data[key].responses = parts.responses[:0]
				} else {
					keep := parts.requests[i:]
					parts := newTXParts(len(keep), defaultArraySize)
					parts.requests = append(parts.requests, keep...)
					b.data[key] = parts
				}
				b.size += len(parts.requests) - i
				break
			}
			b.telemetry.joiner.agedRequest.Add(1)
			i++
		}
	}

	return joined
}

func (b *incompleteBuffer) shouldKeep(tx Transaction, now int64) bool {
	then := int64(tx.RequestStarted())
	return (now - then) < b.minAgeNano
}
