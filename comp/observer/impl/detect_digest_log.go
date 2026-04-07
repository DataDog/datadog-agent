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
	"strings"
	"sync"
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
		log.Printf("[observer] detect digest record error: %v", err)
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
	log.Printf("[observer] detect digest recording enabled: %s", path)
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

// detectDigestComparator compares replay digests against a live recording.
type detectDigestComparator struct {
	mu             sync.Mutex
	expected       map[string]detectDigest // keyed by "detector|dataTime"
	visited        map[string]bool         // keys seen during replay
	divergences    []DetectDivergence
	inputOnlyDiffs int
	matched        int
	replayOnly     int
}

func newDetectDigestComparator(path string) (*detectDigestComparator, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open detect digest log %s: %w", path, err)
	}
	defer f.Close()

	expected := make(map[string]detectDigest)
	dec := json.NewDecoder(f)
	for dec.More() {
		var d detectDigest
		if err := dec.Decode(&d); err != nil {
			return nil, fmt.Errorf("parse detect digest entry: %w", err)
		}
		expected[detectDigestKey(d.DetectorName, d.DataTime)] = d
	}

	return &detectDigestComparator{
		expected: expected,
		visited:  make(map[string]bool, len(expected)),
	}, nil
}

// compare is called for each replay Detect() result.
func (c *detectDigestComparator) compare(actual detectDigest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := detectDigestKey(actual.DetectorName, actual.DataTime)
	live, ok := c.expected[key]
	if !ok {
		c.replayOnly++
		return
	}
	c.visited[key] = true

	// Check result divergence (anomaly count or fingerprints differ).
	resultMatch := live.AnomalyCount == actual.AnomalyCount &&
		fingerprintsEqual(live.AnomalyFingerprints, actual.AnomalyFingerprints)
	inputMatch := live.InputHash == actual.InputHash

	if resultMatch && inputMatch {
		c.matched++
		return
	}

	if resultMatch && !inputMatch {
		// Input differs but results are the same — benign.
		c.matched++
		c.inputOnlyDiffs++
		return
	}

	// Result divergence.
	c.divergences = append(c.divergences, DetectDivergence{
		DetectorName:       actual.DetectorName,
		DataTime:           actual.DataTime,
		LiveAnomalyCount:   live.AnomalyCount,
		ReplayAnomalyCount: actual.AnomalyCount,
		LiveFingerprints:   live.AnomalyFingerprints,
		ReplayFingerprints: actual.AnomalyFingerprints,
		InputHashMatch:     inputMatch,
		LiveInputHash:      live.InputHash,
		ReplayInputHash:    actual.InputHash,
	})
}

func (c *detectDigestComparator) printSummary() {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := len(c.expected)
	liveOnly := total - len(c.visited)

	if len(c.divergences) == 0 && liveOnly == 0 {
		fmt.Printf("[testbench] Detection digest: %d/%d matched", c.matched, total)
		if c.inputOnlyDiffs > 0 {
			fmt.Printf(", %d input-only diffs (benign)", c.inputOnlyDiffs)
		}
		if c.replayOnly > 0 {
			fmt.Printf(", %d replay-only steps", c.replayOnly)
		}
		fmt.Println()
		return
	}

	fmt.Println()
	fmt.Println("=== DETECTION RESULT DIVERGENCES ===")
	for _, d := range c.divergences {
		fmt.Print(d.String())
	}
	if liveOnly > 0 {
		fmt.Printf("\n  %d live-only digest entries (replay never advanced to these):\n", liveOnly)
		printed := 0
		for key, d := range c.expected {
			if c.visited[key] {
				continue
			}
			if printed < 10 {
				fmt.Printf("    %s @ dataTime=%d: %d anomalies\n", d.DetectorName, d.DataTime, d.AnomalyCount)
			}
			printed++
		}
		if printed > 10 {
			fmt.Printf("    ... and %d more\n", printed-10)
		}
	}
	fmt.Printf("\nSummary: %d/%d matched, %d result divergences",
		c.matched, total, len(c.divergences))
	if c.inputOnlyDiffs > 0 {
		fmt.Printf(", %d input-only diffs (benign)", c.inputOnlyDiffs)
	}
	if liveOnly > 0 {
		fmt.Printf(", %d live-only", liveOnly)
	}
	if c.replayOnly > 0 {
		fmt.Printf(", %d replay-only", c.replayOnly)
	}
	fmt.Println()
	fmt.Println("====================================")
}

// fingerprintsEqual compares two sorted fingerprint slices.
func fingerprintsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
