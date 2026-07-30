// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// detectDigestRecorder writes detect digests to a JSONL file.
type detectDigestRecorder struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

func newDetectDigestRecorder(path string) (*detectDigestRecorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create detect digest log %s: %w", path, err)
	}
	return &detectDigestRecorder{
		f:   f,
		enc: json.NewEncoder(f),
	}, nil
}

func (r *detectDigestRecorder) record(d detectDigest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.enc.Encode(d); err != nil {
		pkglog.Warnf("[observer] detect digest record error: %v", err)
	}
}

func (r *detectDigestRecorder) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}

// enableDetectDigestRecordingToFile creates a recorder and wires it to the engine.
// Returns a cleanup function that disables recording and closes the file.
func enableDetectDigestRecordingToFile(e *engine, path string) (func(), error) {
	rec, err := newDetectDigestRecorder(path)
	if err != nil {
		return nil, err
	}
	e.enableDetectDigestRecording(rec.record)
	pkglog.Infof("[observer] detect digest recording enabled: %s", path)
	return func() {
		e.enableDetectDigestRecording(nil)
		_ = rec.close()
	}, nil
}

// DetectDivergence describes a mismatch between live and replay detection results.
type DetectDivergence struct {
	DetectorName       string
	DataTime           int64
	LiveAnomalyCount   int
	ReplayAnomalyCount int
	LiveFingerprints   []string
	ReplayFingerprints []string
	InputHashMatch     bool
	LiveInputHash      uint64
	ReplayInputHash    uint64
}

func (d DetectDivergence) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %s @ dataTime=%d: live=%d anomalies, replay=%d anomalies\n",
		d.DetectorName, d.DataTime, d.LiveAnomalyCount, d.ReplayAnomalyCount)

	// Show fingerprints only present on one side.
	liveSet := make(map[string]bool, len(d.LiveFingerprints))
	for _, fp := range d.LiveFingerprints {
		liveSet[fp] = true
	}
	replaySet := make(map[string]bool, len(d.ReplayFingerprints))
	for _, fp := range d.ReplayFingerprints {
		replaySet[fp] = true
	}
	var liveOnly, replayOnly []string
	for _, fp := range d.LiveFingerprints {
		if !replaySet[fp] {
			liveOnly = append(liveOnly, fp)
		}
	}
	for _, fp := range d.ReplayFingerprints {
		if !liveSet[fp] {
			replayOnly = append(replayOnly, fp)
		}
	}
	if len(liveOnly) > 0 {
		fmt.Fprintf(&b, "    live-only:   %v\n", liveOnly)
	}
	if len(replayOnly) > 0 {
		fmt.Fprintf(&b, "    replay-only: %v\n", replayOnly)
	}

	if d.InputHashMatch {
		fmt.Fprintf(&b, "    (input hash matches — same data, different result)\n")
	} else {
		fmt.Fprintf(&b, "    (input hash differs: live=%016x replay=%016x)\n",
			d.LiveInputHash, d.ReplayInputHash)
	}
	return b.String()
}
