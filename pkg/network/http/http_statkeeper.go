// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

type httpStatKeeper struct {
	stats      map[Key]RequestStats
	incomplete *incompleteBuffer
	maxEntries int
	telemetry  *telemetry

	// replace rules for HTTP path
	replaceRules []*config.ReplaceRule

	// http path buffer
	buffer []byte

	// map containing interned path strings
	// this is rotated  with the stats map
	interned map[string]string
}

func newHTTPStatkeeper(c *config.Config, telemetry *telemetry) *httpStatKeeper {
	return &httpStatKeeper{
		stats:        make(map[Key]RequestStats),
		incomplete:   newIncompleteBuffer(c, telemetry),
		maxEntries:   c.MaxHTTPStatsBuffered,
		replaceRules: c.HTTPReplaceRules,
		buffer:       make([]byte, HTTPBufferSize),
		interned:     make(map[string]string),
		telemetry:    telemetry,
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for _, tx := range transactions {
		if tx.Incomplete() {
			h.incomplete.Add(tx)
			continue
		}

		h.add(tx)
	}
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]RequestStats {
	for _, tx := range h.incomplete.Flush(time.Now()) {
		h.add(*tx)
	}

	atomic.StoreInt64(&h.telemetry.aggregations, int64(len(h.stats)))
	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]RequestStats)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) add(tx httpTX) {
	path, rejected := h.processHTTPPath(tx)
	if rejected {
		atomic.AddInt64(&h.telemetry.rejected, 1)
		return
	}

	key := h.newKey(tx, path)
	stats, ok := h.stats[key]
	if !ok && len(h.stats) >= h.maxEntries {
		atomic.AddInt64(&h.telemetry.dropped, 1)
		return
	}

	stats.AddRequest(tx.StatusClass(), tx.RequestLatency(), tx.Tags())
	h.stats[key] = stats
}

func (h *httpStatKeeper) newKey(tx httpTX, path string) Key {
	return Key{
		SrcIPHigh: uint64(tx.tup.saddr_h),
		SrcIPLow:  uint64(tx.tup.saddr_l),
		SrcPort:   uint16(tx.tup.sport),
		DstIPHigh: uint64(tx.tup.daddr_h),
		DstIPLow:  uint64(tx.tup.daddr_l),
		DstPort:   uint16(tx.tup.dport),
		Path:      path,
		Method:    Method(tx.request_method),
	}
}

func (h *httpStatKeeper) processHTTPPath(tx httpTX) (pathStr string, rejected bool) {
	path := tx.Path(h.buffer)

	for _, r := range h.replaceRules {
		if r.Re.Match(path) {
			if r.Repl == "" {
				// this is a "drop" rule
				return "", true
			}

			path = r.Re.ReplaceAll(path, []byte(r.Repl))
		}
	}

	return h.intern(path), false
}

func (h *httpStatKeeper) intern(b []byte) string {
	v, ok := h.interned[string(b)]
	if !ok {
		v = string(b)
		h.interned[v] = v
	}
	return v
}
