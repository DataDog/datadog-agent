// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

type httpStatKeeper struct {
	stats      map[Key]*RequestStats
	incomplete map[KeyTuple]*httpTX
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
		stats:        make(map[Key]*RequestStats),
		incomplete:   make(map[KeyTuple]*httpTX),
		maxEntries:   c.MaxHTTPStatsBuffered,
		replaceRules: c.HTTPReplaceRules,
		buffer:       make([]byte, HTTPBufferSize),
		interned:     make(map[string]string),
		telemetry:    telemetry,
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for i := range transactions {
		tx := &transactions[i]
		if tx.Incomplete() {
			h.handleIncomplete(tx)
			continue
		}

		h.add(tx)
	}

	atomic.StoreInt64(&h.telemetry.aggregations, int64(len(h.stats)))
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]*RequestStats)
	h.incomplete = make(map[KeyTuple]*httpTX)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) add(tx *httpTX) {
	rawPath := tx.Path(h.buffer)
	if rawPath == nil {
		atomic.AddInt64(&h.telemetry.malformed, 1)
		return
	}
	path, rejected := h.processHTTPPath(rawPath)
	if rejected {
		atomic.AddInt64(&h.telemetry.rejected, 1)
		return
	}

	key := h.newKey(tx, path)
	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			atomic.AddInt64(&h.telemetry.dropped, 1)
			return
		}
		stats = new(RequestStats)
		h.stats[key] = stats
	}

	stats.AddRequest(tx.StatusClass(), tx.RequestLatency())
}

// handleIncomplete is responsible for handling incomplete transactions
// (eg. httpTX objects that have either only the request or response information)
// this happens only in the context of localhost traffic with NAT and these disjoint
// parts of the transactions are joined here by src port
func (h *httpStatKeeper) handleIncomplete(tx *httpTX) {
	key := KeyTuple{
		SrcIPHigh: uint64(tx.tup.saddr_h),
		SrcIPLow:  uint64(tx.tup.saddr_l),
		SrcPort:   uint16(tx.tup.sport),
	}

	otherHalf, ok := h.incomplete[key]
	if !ok {
		if len(h.incomplete) >= h.maxEntries {
			atomic.AddInt64(&h.telemetry.dropped, 1)
		} else {
			h.incomplete[key] = tx
		}
		return
	}

	request, response := tx, otherHalf
	if response.StatusClass() == 0 {
		request, response = response, request
	}

	if request.request_started == 0 || response.response_status_code == 0 || request.request_started > response.response_last_seen {
		// This means we can't join these parts as they don't belong to the same transaction.
		// In this case, as a best-effort we override the incomplete entry with the latest one
		// we got from eBPF, so it can be joined by it's other half at a later moment.
		// This can happen because we can get out-of-order half transactions from eBPF
		atomic.AddInt64(&h.telemetry.dropped, 1)
		h.incomplete[key] = tx
		return
	}

	// Merge response into request
	request.response_status_code = response.response_status_code
	request.response_last_seen = response.response_last_seen
	h.add(request)
	delete(h.incomplete, key)
}

func (h *httpStatKeeper) newKey(tx *httpTX, path string) Key {
	return Key{
		KeyTuple: KeyTuple{
			SrcIPHigh: uint64(tx.tup.saddr_h),
			SrcIPLow:  uint64(tx.tup.saddr_l),
			SrcPort:   uint16(tx.tup.sport),
			DstIPHigh: uint64(tx.tup.daddr_h),
			DstIPLow:  uint64(tx.tup.daddr_l),
			DstPort:   uint16(tx.tup.dport),
		},
		Path:   path,
		Method: Method(tx.request_method),
	}
}

func (h *httpStatKeeper) processHTTPPath(path []byte) (pathStr string, rejected bool) {
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
