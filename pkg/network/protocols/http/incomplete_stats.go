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
	// maxEntries is the number of requests, with their responses, the buffer holds across all connections.
	maxEntries int
	// size counts the buffered requests and responses: at most 2*maxEntries, plus responses to requests kept by Flush.
	size       int
	telemetry  *Telemetry
	minAgeNano int64
}

type txParts struct {
	requests  []Transaction
	responses []Transaction
	// cutoff is the earliest timestamp of a half of this connection dropped since the last Flush, if hasCutoff.
	cutoff    uint64
	hasCutoff bool
	// reserved counts the responses still admitted beyond the cap for requests the last Flush kept.
	reserved int
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
		maxEntries: c.MaxHTTPStatsBuffered,
		telemetry:  telemetry,
		minAgeNano: defaultMinAge.Nanoseconds(),
	}
}

// Add buffers tx unless the buffer already holds 2*maxEntries requests and responses, or tx's connection holds
// three quarters of that; a connection with requests kept by the last Flush always gets that many responses.
func (b *incompleteBuffer) Add(tx Transaction) {
	ebpfTX, ok := tx.(*EbpfEvent)
	if !ok {
		// should never happen
		return
	}

	connTuple := tx.ConnTuple()
	key := types.ConnectionKey{
		SrcIPHigh: connTuple.SrcIPHigh,
		SrcIPLow:  connTuple.SrcIPLow,
		SrcPort:   connTuple.SrcPort,
	}

	limit := 2 * b.maxEntries
	parts, ok := b.data[key]
	isResponse := tx.StatusCode() != 0
	seen := tx.RequestStarted()
	if isResponse {
		seen = tx.ResponseLastSeen()
	}
	if ok && parts.hasCutoff && seen >= parts.cutoff {
		// Flush discards a connection's halves from its cutoff on.
		b.telemetry.dropped.Add(1)
		return
	}
	if ok && isResponse && parts.reserved > 0 {
		parts.reserved--
	} else if b.size >= limit || (ok && len(parts.requests)+len(parts.responses) >= limit*3/4) {
		// The last quarter of the room is kept for other connections.
		b.telemetry.dropped.Add(1)
		if ok && (!parts.hasCutoff || seen < parts.cutoff) {
			// A missing half shifts Flush's time-ordered pairing, so Flush pairs only this connection's earlier halves.
			parts.cutoff, parts.hasCutoff = seen, true
		}
		return
	}

	if !ok {
		parts = newTXParts(defaultArraySize, defaultArraySize)
		b.data[key] = parts
	}

	// copy underlying httpTX value. this is now needed because these objects are
	// now coming directly from pooled perf records
	ebpfTxCopy := new(EbpfEvent)
	*ebpfTxCopy = *ebpfTX
	tx = ebpfTxCopy
	b.size++

	if tx.StatusCode() == 0 {
		b.telemetry.joiner.requests.Add(1)
		parts.requests = append(parts.requests, tx)
	} else {
		b.telemetry.joiner.responses.Add(1)
		parts.responses = append(parts.responses, tx)
	}
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
	for key, parts := range previous {
		if parts.hasCutoff {
			joined = b.joinBeforeCutoff(parts, joined)
			continue
		}
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
					b.data[key].responses = nil
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
	b.trimKept()
	for _, parts := range b.data {
		parts.reserved = len(parts.requests)
	}

	return joined
}

// joinBeforeCutoff joins a connection's halves read before its cutoff and drops the rest, which may belong to the
// dropped half's transaction.
func (b *incompleteBuffer) joinBeforeCutoff(parts *txParts, joined []Transaction) []Transaction {
	requests := slices.DeleteFunc(parts.requests, func(tx Transaction) bool { return tx.RequestStarted() >= parts.cutoff })
	responses := slices.DeleteFunc(parts.responses, func(tx Transaction) bool { return tx.ResponseLastSeen() >= parts.cutoff })
	dropped := len(parts.requests) + len(parts.responses)
	slices.SortFunc(requests, func(a, b Transaction) int { return cmp.Compare(a.RequestStarted(), b.RequestStarted()) })
	slices.SortFunc(responses, func(a, b Transaction) int { return cmp.Compare(a.ResponseLastSeen(), b.ResponseLastSeen()) })

	i, j := 0, 0
	for i < len(requests) && j < len(responses) {
		request, response := requests[i], responses[j]
		if request.RequestStarted() > response.ResponseLastSeen() {
			b.telemetry.joiner.responsesDropped.Add(1)
			dropped--
			j++
			continue
		}
		request.SetStatusCode(response.StatusCode())
		request.SetResponseLastSeen(response.ResponseLastSeen())
		joined = append(joined, request)
		dropped -= 2
		i++
		j++
		b.telemetry.joiner.requestJoined.Add(1)
	}
	b.telemetry.dropped.Add(int64(dropped))
	return joined
}

// trimKept drops the connections holding the oldest requests Flush kept beyond maxEntries, bounding the responses
// admitted beyond the cap.
func (b *incompleteBuffer) trimKept() {
	limit := b.maxEntries
	if limit <= 0 {
		b.telemetry.dropped.Add(int64(b.size))
		b.data = make(map[types.ConnectionKey]*txParts)
		b.size = 0
		return
	}
	if b.size <= limit {
		return
	}
	starts := make([]uint64, 0, b.size)
	for _, parts := range b.data {
		for _, req := range parts.requests {
			starts = append(starts, req.RequestStarted())
		}
	}
	slices.Sort(starts)
	cutoff := starts[len(starts)-limit]
	// Requests started at cutoff are kept only as far as they fit within limit.
	dropAtCutoff := 0
	for _, s := range starts[:len(starts)-limit] {
		if s == cutoff {
			dropAtCutoff++
		}
	}
	for key, parts := range b.data {
		// Flush leaves each connection's requests sorted by start, so the dropped ones are a prefix.
		i := 0
		for i < len(parts.requests) && parts.requests[i].RequestStarted() < cutoff {
			i++
		}
		for i < len(parts.requests) && parts.requests[i].RequestStarted() == cutoff && dropAtCutoff > 0 {
			dropAtCutoff--
			i++
		}
		if i == 0 {
			continue
		}
		// A response to a dropped request could pair with a kept pipelined one, so the connection is discarded until
		// the next Flush.
		b.telemetry.dropped.Add(int64(len(parts.requests)))
		b.size -= len(parts.requests)
		b.data[key] = &txParts{hasCutoff: true}
	}
}

func (b *incompleteBuffer) shouldKeep(tx Transaction, now int64) bool {
	then := int64(tx.RequestStarted())
	return (now - then) < b.minAgeNano
}
