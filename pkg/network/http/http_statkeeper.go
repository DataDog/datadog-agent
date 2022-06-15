// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type httpStatKeeper struct {
	stats      map[Key]*RequestStats
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

	oversizedLogLimit *util.LogLimit
}

func newHTTPStatkeeper(c *config.Config, telemetry *telemetry) *httpStatKeeper {
	return &httpStatKeeper{
		stats:             make(map[Key]*RequestStats),
		incomplete:        newIncompleteBuffer(c, telemetry),
		maxEntries:        c.MaxHTTPStatsBuffered,
		replaceRules:      c.HTTPReplaceRules,
		buffer:            make([]byte, HTTPBufferSize),
		interned:          make(map[string]string),
		telemetry:         telemetry,
		oversizedLogLimit: util.NewLogLimit(10, time.Minute*10),
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for i := range transactions {
		tx := &transactions[i]
		if tx.Incomplete() {
			h.incomplete.Add(tx)
			continue
		}

		h.add(tx)
	}

	atomic.StoreInt64(&h.telemetry.aggregations, int64(len(h.stats)))
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	for _, tx := range h.incomplete.Flush(time.Now()) {
		h.add(tx)
	}

	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]*RequestStats)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) add(tx *httpTX) {
	rawPath, fullPath := tx.Path(h.buffer)
	if rawPath == nil {
		atomic.AddInt64(&h.telemetry.malformed, 1)
		return
	}

	path, rejected := h.processHTTPPath(tx, rawPath)
	if rejected {
		return
	}

	if Method(tx.request_method) == MethodUnknown {
		atomic.AddInt64(&h.telemetry.malformed, 1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("method should never be unknown: %s", tx.String())
		}
		return
	}

	latency := tx.RequestLatency()
	if latency <= 0 {
		atomic.AddInt64(&h.telemetry.malformed, 1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("latency should never be equal to 0: %s", tx.String())
		}
		return
	}

	key := h.newKey(tx, path, fullPath)
	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			atomic.AddInt64(&h.telemetry.dropped, 1)
			return
		}
		stats = new(RequestStats)
		h.stats[key] = stats
	}

	stats.AddRequest(tx.StatusClass(), latency, tx.Tags())
}

func (h *httpStatKeeper) newKey(tx *httpTX, path string, fullPath bool) Key {
	return Key{
		KeyTuple: KeyTuple{
			SrcIPHigh: uint64(tx.tup.saddr_h),
			SrcIPLow:  uint64(tx.tup.saddr_l),
			SrcPort:   uint16(tx.tup.sport),
			DstIPHigh: uint64(tx.tup.daddr_h),
			DstIPLow:  uint64(tx.tup.daddr_l),
			DstPort:   uint16(tx.tup.dport),
		},
		Path: Path{
			Content:  path,
			FullPath: fullPath,
		},
		Method: Method(tx.request_method),
	}
}

func pathIsMalformed(fullPath []byte) bool {
	for _, r := range fullPath {
		if !strconv.IsPrint(rune(r)) {
			return true
		}
	}
	return false
}

func (h *httpStatKeeper) processHTTPPath(tx *httpTX, path []byte) (pathStr string, rejected bool) {
	match := false
	for _, r := range h.replaceRules {
		if r.Re.Match(path) {
			if r.Repl == "" {
				// this is a "drop" rule
				atomic.AddInt64(&h.telemetry.rejected, 1)
				return "", true
			}

			path = r.Re.ReplaceAll(path, []byte(r.Repl))
			match = true
		}
	}

	// If the user didn't specify a rule matching this particular path, we can check for its format.
	// Otherwise, we don't want the custom path to be rejected by our path formatting check.
	if !match && pathIsMalformed(path) {
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("http path malformed: %s", tx.String())
		}
		atomic.AddInt64(&h.telemetry.malformed, 1)
		return "", true
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
