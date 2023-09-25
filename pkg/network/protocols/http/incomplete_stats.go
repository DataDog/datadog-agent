// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"sort"
	"time"

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
	data       map[types.ConnectionKey]*txParts
	maxEntries int
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

func newIncompleteBuffer(c *config.Config, telemetry *Telemetry) *incompleteBuffer {
	return &incompleteBuffer{
		data:       make(map[types.ConnectionKey]*txParts),
		maxEntries: c.MaxHTTPStatsBuffered,
		telemetry:  telemetry,
		minAgeNano: defaultMinAge.Nanoseconds(),
	}
}

func (b *incompleteBuffer) Add(tx Transaction) {
	connTuple := tx.ConnTuple()
	key := types.ConnectionKey{
		SrcIPHigh: connTuple.SrcIPHigh,
		SrcIPLow:  connTuple.SrcIPLow,
		SrcPort:   connTuple.SrcPort,
	}

	parts, ok := b.data[key]
	if !ok {
		if len(b.data) >= b.maxEntries {
			b.telemetry.dropped.Add(1)
			return
		}

		parts = newTXParts(defaultArraySize, defaultArraySize)
		b.data[key] = parts
	}

	// copy underlying httpTX value. this is now needed because these objects are
	// now coming directly from pooled perf records
	ebpfTX, ok := tx.(*EbpfTx)
	if !ok {
		// should never happen
		return
	}

	ebpfTxCopy := new(EbpfTx)
	*ebpfTxCopy = *ebpfTX
	tx = ebpfTxCopy

	if tx.StatusCode() == 0 {
		b.telemetry.joiner.requests.Add(1)
		parts.requests = append(parts.requests, tx)
	} else {
		b.telemetry.joiner.responses.Add(1)
		parts.responses = append(parts.responses, tx)
	}
}

func (b *incompleteBuffer) Flush(now time.Time) []Transaction {
	var (
		joined   []Transaction
		previous = b.data
		nowUnix  = now.UnixNano()
	)

	b.data = make(map[types.ConnectionKey]*txParts)
	for key, parts := range previous {
		// TODO: in this loop we're sorting all transactions at once, but we could also
		// consider sorting data during insertion time (using a tree-like structure, for example)
		sort.Sort(byRequestTime(parts.requests))
		sort.Sort(byResponseTime(parts.responses))

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

type byRequestTime []Transaction

func (rt byRequestTime) Len() int      { return len(rt) }
func (rt byRequestTime) Swap(i, j int) { rt[i], rt[j] = rt[j], rt[i] }
func (rt byRequestTime) Less(i, j int) bool {
	return rt[i].RequestStarted() < rt[j].RequestStarted()
}

type byResponseTime []Transaction

func (rt byResponseTime) Len() int      { return len(rt) }
func (rt byResponseTime) Swap(i, j int) { rt[i], rt[j] = rt[j], rt[i] }
func (rt byResponseTime) Less(i, j int) bool {
	return rt[i].ResponseLastSeen() < rt[j].ResponseLastSeen()
}
