// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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
		pkglog.Warnf("[observer] advance log record error: %v", err)
	}
}

func (r *advanceLogRecorder) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
