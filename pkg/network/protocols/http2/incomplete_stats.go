// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultMinAge = 30 * time.Second
)

// incompleteBuffer is responsible for buffering incomplete transactions
type incompleteBuffer struct {
	data              []*ebpfTXWrapper
	maxEntries        int
	minAgeNano        int64
	oversizedLogLimit *util.LogLimit
}

// NewIncompleteBuffer returns a new incompleteBuffer.
func NewIncompleteBuffer(c *config.Config) http.IncompleteBuffer {
	return &incompleteBuffer{
		data:              make([]*ebpfTXWrapper, 0),
		maxEntries:        c.MaxHTTPStatsBuffered,
		minAgeNano:        defaultMinAge.Nanoseconds(),
		oversizedLogLimit: util.NewLogLimit(10, time.Minute*10),
	}
}

// Add adds a transaction to the buffer.
func (b *incompleteBuffer) Add(tx http.Transaction) {
	if len(b.data) >= b.maxEntries {
		if b.oversizedLogLimit.ShouldLog() {
			log.Warnf("http2 incomplete buffer is full (%d), dropping transaction", b.maxEntries)
		}
		return
	}

	// Copy underlying EbpfTx value.
	ebpfTX, ok := tx.(*ebpfTXWrapper)
	if !ok {
		if b.oversizedLogLimit.ShouldLog() {
			log.Warnf("http2 incomplete buffer received a non-EbpfTx transaction (%T), dropping transaction", tx)
		}
		return
	}

	ebpfTxCopy := new(ebpfTXWrapper)
	*ebpfTxCopy = *ebpfTX

	b.data = append(b.data, ebpfTxCopy)
}

// Flush flushes the buffer and returns the joined transactions.
func (b *incompleteBuffer) Flush(now time.Time) []http.Transaction {
	var (
		joined   []http.Transaction
		previous = b.data
		nowUnix  = now.UnixNano()
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
	diff := now - then
	return diff < b.minAgeNano
}
