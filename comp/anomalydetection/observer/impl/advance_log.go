// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
)

const advanceLogFileName = "advances.jsonl"

// advanceEntry records a single advance call, including data ingestion counters
// accumulated since the previous advance.
type advanceEntry struct {
	DataTime           int64            `json:"data_time"`
	Reason             string           `json:"reason"`
	LatePoints         int64            `json:"late_points,omitempty"`           // points ingested after their timestamp was analyzed
	LatePointsBySource map[string]int64 `json:"late_points_by_source,omitempty"` // per-source breakdown
	DroppedObs         int64            `json:"dropped_obs,omitempty"`           // observations dropped due to full channel
	DroppedBySource    map[string]int64 `json:"dropped_by_source,omitempty"`     // per-source breakdown
}

func advanceReasonString(r advanceReason) string {
	switch r {
	case advanceReasonInputDriven:
		return "input"
	case advanceReasonPeriodicFlush:
		return "periodic"
	case advanceReasonReplayEnd:
		return "replay_end"
	case advanceReasonManual:
		return "manual"
	default:
		return "unknown"
	}
}

// advanceLogRecorder writes advance entries to a JSONL file.
type advanceLogRecorder struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

func newAdvanceLogRecorder(path string) (*advanceLogRecorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create advance log %s: %w", path, err)
	}
	return &advanceLogRecorder{f: f, enc: json.NewEncoder(f)}, nil
}

func (r *advanceLogRecorder) record(e advanceEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.enc.Encode(e); err != nil {
		log.Printf("[observer] advance log record error: %v", err)
	}
}

func (r *advanceLogRecorder) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}

// advanceLogComparator compares advance sequences between live and replay.
type advanceLogComparator struct {
	liveAdvances         map[int64]advanceEntry // dataTime → full entry
	replayOnly           []advanceEntry
	liveOnly             []advanceEntry
	matched              int
	totalLatePoints      int64
	totalDroppedObs      int64
	totalLateBySource    map[string]int64
	totalDroppedBySource map[string]int64
}

func newAdvanceLogComparator(path string) (*advanceLogComparator, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open advance log %s: %w", path, err)
	}
	defer f.Close()

	advances := make(map[int64]advanceEntry)
	var totalLate, totalDropped int64
	lateBySource := make(map[string]int64)
	droppedBySource := make(map[string]int64)
	dec := json.NewDecoder(f)
	for dec.More() {
		var e advanceEntry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("parse advance log entry: %w", err)
		}
		advances[e.DataTime] = e
		totalLate += e.LatePoints
		totalDropped += e.DroppedObs
		for src, count := range e.LatePointsBySource {
			lateBySource[src] += count
		}
		for src, count := range e.DroppedBySource {
			droppedBySource[src] += count
		}
	}

	return &advanceLogComparator{
		liveAdvances:         advances,
		totalLatePoints:      totalLate,
		totalDroppedObs:      totalDropped,
		totalLateBySource:    lateBySource,
		totalDroppedBySource: droppedBySource,
	}, nil
}

// liveAdvanceTimes returns the sorted list of data_time values from the live advance log.
func (c *advanceLogComparator) liveAdvanceTimes() []int64 {
	times := make([]int64, 0, len(c.liveAdvances))
	for t := range c.liveAdvances {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	return times
}

// compare is called for each replay advance.
func (c *advanceLogComparator) compare(e advanceEntry) {
	if _, ok := c.liveAdvances[e.DataTime]; ok {
		c.matched++
		delete(c.liveAdvances, e.DataTime) // mark as seen
	} else {
		c.replayOnly = append(c.replayOnly, e)
	}
}

// finalize collects live-only advances (those not matched by replay).
func (c *advanceLogComparator) finalize() {
	for _, entry := range c.liveAdvances {
		c.liveOnly = append(c.liveOnly, entry)
	}
}

func (c *advanceLogComparator) printSummary() {
	c.finalize()
	total := c.matched + len(c.liveOnly)

	// Always report ingestion anomalies from the live run — these are the
	// most likely explanation for detection divergences.
	if c.totalLatePoints > 0 || c.totalDroppedObs > 0 {
		fmt.Printf("[testbench] Live ingestion anomalies: %d late points, %d dropped observations\n",
			c.totalLatePoints, c.totalDroppedObs)
		if len(c.totalLateBySource) > 0 {
			fmt.Printf("[testbench] Late points by source:")
			for src, count := range c.totalLateBySource {
				fmt.Printf(" %s=%d", src, count)
			}
			fmt.Println()
		}
		if len(c.totalDroppedBySource) > 0 {
			fmt.Printf("[testbench] Dropped observations by source:")
			for src, count := range c.totalDroppedBySource {
				fmt.Printf(" %s=%d", src, count)
			}
			fmt.Println()
		}
	}

	if len(c.replayOnly) == 0 && len(c.liveOnly) == 0 {
		fmt.Printf("[testbench] Advance log: %d/%d advances matched\n", c.matched, total)
		return
	}

	fmt.Println()
	fmt.Println("=== ADVANCE SEQUENCE DIVERGENCES ===")
	if len(c.liveOnly) > 0 {
		fmt.Printf("  %d advances in live but NOT in replay:\n", len(c.liveOnly))
		limit := len(c.liveOnly)
		if limit > 10 {
			limit = 10
		}
		for _, e := range c.liveOnly[:limit] {
			fmt.Printf("    dataTime=%d reason=%s\n", e.DataTime, e.Reason)
		}
		if len(c.liveOnly) > 10 {
			fmt.Printf("    ... and %d more\n", len(c.liveOnly)-10)
		}
	}
	if len(c.replayOnly) > 0 {
		fmt.Printf("  %d advances in replay but NOT in live:\n", len(c.replayOnly))
		limit := len(c.replayOnly)
		if limit > 10 {
			limit = 10
		}
		for _, e := range c.replayOnly[:limit] {
			fmt.Printf("    dataTime=%d reason=%s\n", e.DataTime, e.Reason)
		}
		if len(c.replayOnly) > 10 {
			fmt.Printf("    ... and %d more\n", len(c.replayOnly)-10)
		}
	}
	fmt.Printf("\nSummary: %d matched, %d live-only, %d replay-only\n",
		c.matched, len(c.liveOnly), len(c.replayOnly))
	fmt.Println("====================================")
}
