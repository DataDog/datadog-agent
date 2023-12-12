// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

const (
	defaultMinAge = 30 * time.Second
)

// incompleteBuffer is responsible for buffering incomplete transactions
type incompleteBuffer struct {
	data       []*ebpfTXWrapper
	maxEntries int
	minAgeNano int64
}

// NewIncompleteBuffer returns a new incompleteBuffer.
func NewIncompleteBuffer(c *config.Config) http.IncompleteBuffer {
	return &incompleteBuffer{
		data:       make([]*ebpfTXWrapper, 0),
		maxEntries: c.MaxHTTPStatsBuffered,
		minAgeNano: defaultMinAge.Nanoseconds(),
	}
}

// Add adds a transaction to the buffer.
func (b *incompleteBuffer) Add(tx http.Transaction) {
	if len(b.data) >= b.maxEntries {
		return
	}

	// copy underlying httpTX value. this is now needed because these objects are
	// now coming directly from pooled perf records
	ebpfTX, ok := tx.(*ebpfTXWrapper)
	if !ok {
		// should never happen
		return
	}

	ebpfTxCopy := new(ebpfTXWrapper)
	*ebpfTxCopy = *ebpfTX

	b.data = append(b.data, ebpfTxCopy)
}

// Flush flushes the buffer and returns the joined transactions.
func (b *incompleteBuffer) Flush(time.Time) []http.Transaction {
	var (
		joined     []http.Transaction
		previous   = b.data
		nowUnix, _ = ebpf.NowNanoseconds()
	)

	b.data = make([]*ebpfTXWrapper, 0)
	for _, entry := range previous {
		// now that we have finished matching requests and responses
		// we check if we should keep orphan requests a little longer
		if !entry.Incomplete() {
			joined = append(joined, entry)
		} else if b.shouldKeep(entry, nowUnix) {
			b.data = append(b.data, entry)
		}
	}

	return joined
}

func (b *incompleteBuffer) shouldKeep(tx http.Transaction, now int64) bool {
	then := int64(tx.RequestStarted())
	return (now - then) < b.minAgeNano
}
