// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type StatKeeper struct {
	mux                         sync.Mutex
	cfg                         *config.Config
	stats                       map[Key]*RequestStats
	incomplete                  *incompleteBuffer
	maxEntries                  int
	quantizer                   *URLQuantizer
	telemetry                   *Telemetry
	enableStatusCodeAggregation bool

	// replace rules for HTTP path
	replaceRules []*config.ReplaceRule

	// http path buffer
	buffer []byte

	oversizedLogLimit *util.LogLimit
}

func NewStatkeeper(c *config.Config, telemetry *Telemetry) *StatKeeper {
	return &StatKeeper{
		cfg:                         c,
		stats:                       make(map[Key]*RequestStats),
		incomplete:                  newIncompleteBuffer(c, telemetry),
		maxEntries:                  c.MaxHTTPStatsBuffered,
		quantizer:                   NewURLQuantizer(),
		replaceRules:                c.HTTPReplaceRules,
		enableStatusCodeAggregation: c.EnableHTTPStatsByStatusCode,
		buffer:                      make([]byte, getPathBufferSize(c)),
		telemetry:                   telemetry,
		oversizedLogLimit:           util.NewLogLimit(10, time.Minute*10),
	}
}

func (h *StatKeeper) Process(tx Transaction) {
	h.mux.Lock()
	defer h.mux.Unlock()

	if tx.Incomplete() {
		h.incomplete.Add(tx)
		return
	}

	h.add(tx)
}

func (h *StatKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	h.mux.Lock()
	defer h.mux.Unlock()

	for _, tx := range h.incomplete.Flush(time.Now()) {
		h.add(tx)
	}

	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]*RequestStats)
	return ret
}

func (h *StatKeeper) Close() {
	h.oversizedLogLimit.Close()
}

func (h *StatKeeper) add(tx Transaction) {
	rawPath, fullPath := tx.Path(h.buffer)
	if rawPath == nil {
		h.telemetry.emptyPath.Add(1)
		return
	}

	// Quantize HTTP path
	// (eg. this turns /orders/123/view` into `/orders/*/view`)
	if h.cfg.EnableUSMQuantization {
		rawPath = h.quantizer.Quantize(rawPath)
	}

	path, rejected := h.processHTTPPath(tx, rawPath)
	if rejected {
		return
	}

	if tx.Method() == MethodUnknown {
		h.telemetry.unknownMethod.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("method should never be unknown: %s", tx.String())
		}
		return
	}

	latency := tx.RequestLatency()
	if latency <= 0 {
		h.telemetry.invalidLatency.Add(1)
		if h.oversizedLogLimit.ShouldLog() {
			log.Warnf("latency should never be equal to 0: %s", tx.String())
		}
		return
	}

	key := NewKeyWithConnection(tx.ConnTuple(), path, fullPath, tx.Method())
	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			h.telemetry.dropped.Add(1)
			return
		}
		h.telemetry.aggregations.Add(1)
		stats = NewRequestStats(h.enableStatusCodeAggregation)
		h.stats[key] = stats
	}

	stats.AddRequest(tx.StatusCode(), latency, tx.StaticTags(), tx.DynamicTags())
}

func pathIsMalformed(fullPath []byte) bool {
	for _, r := range fullPath {
		if !strconv.IsPrint(rune(r)) {
			return true
		}
	}
	return false
}

func (h *StatKeeper) processHTTPPath(tx Transaction, path []byte) ([]byte, bool) {
	match := false
	for _, r := range h.replaceRules {
		if r.Re.Match(path) {
			if r.Repl == "" {
				// this is a "drop" rule
				h.telemetry.rejected.Add(1)
				return nil, true
			}

			path = r.Re.ReplaceAll(path, []byte(r.Repl))
			match = true
		}
	}

	// If the user didn't specify a rule matching this particular path, we can check for its format.
	// Otherwise, we don't want the custom path to be rejected by our path formatting check.
	if !match && pathIsMalformed(path) {
		if h.oversizedLogLimit.ShouldLog() {
			log.Debugf("http path malformed: %+v %s", tx.ConnTuple(), tx.String())
		}
		h.telemetry.nonPrintableCharacters.Add(1)
		return nil, true
	}
	return path, false
}
