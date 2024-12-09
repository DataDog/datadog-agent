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
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StatKeeper is responsible for aggregating HTTP stats.
type StatKeeper struct {
	mux                  sync.Mutex
	stats                map[Key]*RequestStats
	incomplete           IncompleteBuffer
	maxEntries           int
	quantizer            *URLQuantizer
	telemetry            *Telemetry
	connectionAggregator *utils.ConnectionAggregator

	// replace rules for HTTP path
	replaceRules []*config.ReplaceRule

	// http path buffer
	buffer []byte

	oversizedLogLimit *log.Limit
}

// NewStatkeeper returns a new StatKeeper.
func NewStatkeeper(c *config.Config, telemetry *Telemetry, incompleteBuffer IncompleteBuffer) *StatKeeper {
	var quantizer *URLQuantizer
	// For now we're only enabling path quantization for HTTP/1 traffic
	if c.EnableUSMQuantization && telemetry.protocol == "http" {
		quantizer = NewURLQuantizer()
	}

	var connectionAggregator *utils.ConnectionAggregator
	if c.EnableUSMConnectionRollup {
		connectionAggregator = utils.NewConnectionAggregator()
	}

	return &StatKeeper{
		stats:                make(map[Key]*RequestStats),
		incomplete:           incompleteBuffer,
		maxEntries:           c.MaxHTTPStatsBuffered,
		quantizer:            quantizer,
		replaceRules:         c.HTTPReplaceRules,
		connectionAggregator: connectionAggregator,
		buffer:               make([]byte, getPathBufferSize(c)),
		telemetry:            telemetry,
		oversizedLogLimit:    log.NewLogLimit(10, time.Minute*10),
	}
}

// Process processes a transaction and updates the stats accordingly.
func (h *StatKeeper) Process(tx Transaction) {
	h.mux.Lock()
	defer h.mux.Unlock()

	if tx.Incomplete() {
		h.incomplete.Add(tx)
		return
	}

	h.add(tx)
}

// GetAndResetAllStats returns all the stats and resets the internal state.
func (h *StatKeeper) GetAndResetAllStats() (stats map[Key]*RequestStats) {
	var previousAggregationState *utils.ConnectionAggregator
	func() {
		h.mux.Lock()
		defer h.mux.Unlock()

		for _, tx := range h.incomplete.Flush(time.Now()) {
			h.add(tx)
		}

		// Rotate stats
		stats = h.stats
		h.stats = make(map[Key]*RequestStats)

		// Rotate ConnectionAggregator
		if h.connectionAggregator == nil {
			// Feature not enabled
			return
		}

		previousAggregationState = h.connectionAggregator
		h.connectionAggregator = utils.NewConnectionAggregator()
	}()

	h.clearEphemeralPorts(previousAggregationState, stats)
	return stats
}

// Close closes the stat keeper.
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
	if h.quantizer != nil {
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
	if h.connectionAggregator != nil {
		key.ConnectionKey = h.connectionAggregator.RollupKey(key.ConnectionKey)
	}

	stats, ok := h.stats[key]
	if !ok {
		if len(h.stats) >= h.maxEntries {
			h.telemetry.dropped.Add(1)
			return
		}
		h.telemetry.aggregations.Add(1)
		stats = NewRequestStats()
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

func (h *StatKeeper) clearEphemeralPorts(aggregator *utils.ConnectionAggregator, stats map[Key]*RequestStats) {
	if aggregator == nil {
		return
	}

	// Re-index entries that were generated from multiple connections
	// See comments on `ConnectionAggregator.ClearEphemeralPort()` for more context
	for key, aggregation := range stats {
		newConnKey := aggregator.ClearEphemeralPort(key.ConnectionKey)
		if newConnKey == key.ConnectionKey {
			continue
		}

		delete(stats, key)
		key.ConnectionKey = newConnKey
		stats[key] = aggregation
	}
}
