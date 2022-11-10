// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package kafka

import (
	//	"strconv"
	//	"time"
	//
	"github.com/DataDog/datadog-agent/pkg/network/config"
	//	"github.com/DataDog/datadog-agent/pkg/process/util"
	//	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//
type kafkaStatKeeper struct {
	stats map[Key]*RequestStats
	//incomplete *incompleteBuffer
	maxEntries int
	telemetry  *telemetry

	// replace rules for HTTP path
	//replaceRules []*config.ReplaceRule

	// http path buffer
	//buffer []byte

	// map containing interned path strings
	// this is rotated  with the stats map
	//interned map[string]string

	//oversizedLogLimit *util.LogLimit
}

//
func newKAFKAStatkeeper(c *config.Config, telemetry *telemetry) *kafkaStatKeeper {
	return &kafkaStatKeeper{
		stats: make(map[Key]*RequestStats),
		//		incomplete:        newIncompleteBuffer(c, telemetry),
		maxEntries: c.MaxHTTPStatsBuffered,
		//		replaceRules:      c.HTTPReplaceRules,
		//		buffer:            make([]byte, getPathBufferSize(c)),
		//		interned:          make(map[string]string),
		//		telemetry:         telemetry,
		//		oversizedLogLimit: util.NewLogLimit(10, time.Minute*10),
	}
}

func (statKeeper *kafkaStatKeeper) Process(transactions []kafkaTX) {
	for i := range transactions {
		tx := transactions[i]
		//if tx.Incomplete() {
		//	h.incomplete.Add(tx)
		//	continue
		//}

		statKeeper.add(tx)
	}
}

//
func (statKeeper *kafkaStatKeeper) add(transaction kafkaTX) {
	key := Key{
		KeyTuple: KeyTuple{
			SrcIPHigh: transaction.SrcIPHigh(),
			SrcIPLow:  transaction.SrcIPLow(),
			SrcPort:   transaction.SrcPort(),
			DstIPHigh: transaction.DstIPHigh(),
			DstIPLow:  transaction.DstIPLow(),
			DstPort:   transaction.DstPort(),
		},
		TopicName: transaction.TopicName(),
	}
	requestStats, ok := statKeeper.stats[key]
	if !ok {
		if len(statKeeper.stats) >= statKeeper.maxEntries {
			statKeeper.telemetry.dropped.Add(1)
			return
		}
		requestStats = new(RequestStats)
		statKeeper.stats[key] = requestStats
	}
	if requestStats.data[ProduceAPIKey] == nil {
		requestStats.data[ProduceAPIKey] = new(RequestStat)
	}
	requestStats.data[ProduceAPIKey].Count++
	//	rawPath, fullPath := tx.Path(h.buffer)
	//	if rawPath == nil {
	//		h.telemetry.malformed.Add(1)
	//		return
	//	}
	//	path, rejected := h.processHTTPPath(tx, rawPath)
	//	if rejected {
	//		return
	//	}
	//
	//	if Method(tx.RequestMethod()) == MethodUnknown {
	//		h.telemetry.malformed.Add(1)
	//		if h.oversizedLogLimit.ShouldLog() {
	//			log.Warnf("method should never be unknown: %s", tx.String())
	//		}
	//		return
	//	}
	//
	//	latency := tx.RequestLatency()
	//	if latency <= 0 {
	//		h.telemetry.malformed.Add(1)
	//		if h.oversizedLogLimit.ShouldLog() {
	//			log.Warnf("latency should never be equal to 0: %s", tx.String())
	//		}
	//		return
	//	}
	//
	//	key := h.newKey(tx, path, fullPath)
	//	stats, ok := h.stats[key]
	//	if !ok {
	//		if len(h.stats) >= h.maxEntries {
	//			h.telemetry.dropped.Add(1)
	//			return
	//		}
	//		h.telemetry.aggregations.Add(1)
	//		stats = new(RequestStats)
	//		h.stats[key] = stats
	//	}
	//
	//	stats.AddRequest(tx.StatusClass(), latency, tx.StaticTags(), tx.DynamicTags())
}

func (statKeeper *kafkaStatKeeper) GetAndResetAllStats() map[Key]*RequestStats {
	//for _, tx := range h.incomplete.Flush(time.Now()) {
	//	h.add(tx)
	//}

	ret := statKeeper.stats // No deep copy needed since `statKeeper.stats` gets reset
	statKeeper.stats = make(map[Key]*RequestStats)
	//statKeeper.interned = make(map[string]string)
	return ret
}

//
//func (h *httpStatKeeper) newKey(tx httpTX, path string, fullPath bool) Key {
//	return Key{
//		KeyTuple: KeyTuple{
//			SrcIPHigh: tx.SrcIPHigh(),
//			SrcIPLow:  tx.SrcIPLow(),
//			SrcPort:   tx.SrcPort(),
//			DstIPHigh: tx.DstIPHigh(),
//			DstIPLow:  tx.DstIPLow(),
//			DstPort:   tx.DstPort(),
//		},
//		Path: Path{
//			Content:  path,
//			FullPath: fullPath,
//		},
//		Method: tx.Method(),
//	}
//}
//
//func pathIsMalformed(fullPath []byte) bool {
//	for _, r := range fullPath {
//		if !strconv.IsPrint(rune(r)) {
//			return true
//		}
//	}
//	return false
//}
//
//func (h *httpStatKeeper) processHTTPPath(tx httpTX, path []byte) (pathStr string, rejected bool) {
//	match := false
//	for _, r := range h.replaceRules {
//		if r.Re.Match(path) {
//			if r.Repl == "" {
//				// this is a "drop" rule
//				h.telemetry.rejected.Add(1)
//				return "", true
//			}
//
//			path = r.Re.ReplaceAll(path, []byte(r.Repl))
//			match = true
//		}
//	}
//
//	// If the user didn't specify a rule matching this particular path, we can check for its format.
//	// Otherwise, we don't want the custom path to be rejected by our path formatting check.
//	if !match && pathIsMalformed(path) {
//		if h.oversizedLogLimit.ShouldLog() {
//			log.Debugf("http path malformed: %+v %s", h.newKey(tx, "", false).KeyTuple, tx.String())
//		}
//		h.telemetry.malformed.Add(1)
//		return "", true
//	}
//	return h.intern(path), false
//}
//
//func (h *httpStatKeeper) intern(b []byte) string {
//	v, ok := h.interned[string(b)]
//	if !ok {
//		v = string(b)
//		h.interned[v] = v
//	}
//	return v
//}
