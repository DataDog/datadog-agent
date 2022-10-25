// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package http

import (
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type httpStatKeeper struct {
	stats      map[transaction.Key]*RequestStats
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
		stats:             make(map[transaction.Key]*RequestStats),
		incomplete:        newIncompleteBuffer(c, telemetry),
		maxEntries:        c.MaxHTTPStatsBuffered,
		replaceRules:      c.HTTPReplaceRules,
		buffer:            make([]byte, getPathBufferSize(c)),
		interned:          make(map[string]string),
		telemetry:         telemetry,
		oversizedLogLimit: util.NewLogLimit(10, time.Minute*10),
	}
}

func (h *httpStatKeeper) Process(transactions []transaction.HttpTX) {
	for i := range transactions {
		tx := transactions[i]
		if tx.Incomplete() {
			h.incomplete.Add(tx)
			continue
		}

		h.add(tx)
	}
}

func (h *httpStatKeeper) GetAndResetAllStats() map[transaction.Key]*RequestStats {
	for _, tx := range h.incomplete.Flush(time.Now()) {
		h.add(tx)
	}

	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[transaction.Key]*RequestStats)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) add(tx transaction.HttpTX) {
	rawPath, fullPath := tx.Path(h.buffer)
	if rawPath == nil {
		h.telemetry.malformed.Add(1)
		return
	}
	path, rejected := h.processHTTPPath(tx, rawPath)
	if rejected {
		return
	}

	if transaction.Method(tx.RequestMethod()) == transaction.MethodUnknown {
		h.telemetry.malformed.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("method should never be unknown: %s", tx.String())
		}
		return
	}

	latency := tx.RequestLatency()
	if latency <= 0 {
		h.telemetry.malformed.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("latency should never be equal to 0: %s", tx.String())
		}
		return
	}

	key := tx.NewKey(path, fullPath)
	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			h.telemetry.dropped.Add(1)
			return
		}
		h.telemetry.aggregations.Add(1)
		stats = new(RequestStats)
		h.stats[key] = stats
	}

	stats.AddRequest(tx.StatusClass(), latency, tx.StaticTags(), tx.DynamicTags())
}

func pathIsMalformed(fullPath []byte) bool {
	for _, r := range fullPath {
		if !strconv.IsPrint(rune(r)) {
			return true
		}
	}
	return false
}

func (h *httpStatKeeper) processHTTPPath(tx transaction.HttpTX, path []byte) (pathStr string, rejected bool) {
	match := false
	for _, r := range h.replaceRules {
		if r.Re.Match(path) {
			if r.Repl == "" {
				// this is a "drop" rule
				h.telemetry.rejected.Add(1)
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
			log.Debugf("http path malformed: %+v %s", tx.NewKey("", false).KeyTuple, tx.String())
		}
		h.telemetry.malformed.Add(1)
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
