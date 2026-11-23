// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	defaultSlowEventSampleInterval = 50 * time.Millisecond
	defaultSlowEventThreshold      = 1 * time.Second
	maxSlowEventCheckpoints        = 64
	initialWatchdogStackBufSize    = 64 * 1024
	maxWatchdogStackBufSize        = 1024 * 1024
	maxRelatedSecurityGoroutines   = 5
	defaultWatchdogTargetFrame     = "(*EBPFProbe).handleEvent("
)

// ProcessingCheckpoint records a stack sample captured while an event is being processed.
type ProcessingCheckpoint struct {
	Name          string `json:"name"`
	Stack         string `json:"stack,omitempty"`
	ElapsedUs     int64  `json:"elapsed_us"`
	LastElapsedUs int64  `json:"last_elapsed_us,omitempty"`
	Samples       int    `json:"samples,omitempty"`
}

// SlowProcessingEvent is reported when a kernel event takes longer than the threshold to process.
type SlowProcessingEvent struct {
	events.CustomEventCommonFields
	EventType              string                 `json:"event_type"`
	DurationUs             int64                  `json:"duration_us"`
	ProcessingTimeMicrosec uint32                 `json:"processingtime_microsec"`
	ProcessingTrace        []ProcessingCheckpoint `json:"processing_trace,omitempty"`
	MaxProcessingGapUs     int64                  `json:"max_processing_gap_us,omitempty"`
	MaxProcessingGapFrom   string                 `json:"max_processing_gap_from,omitempty"`
	MaxProcessingGapTo     string                 `json:"max_processing_gap_to,omitempty"`
}

// ToJSON marshals the slow processing event to JSON.
func (e *SlowProcessingEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

func newSlowProcessingEvent(acc *events.AgentContainerContext, eventType model.EventType, duration time.Duration, trace []ProcessingCheckpoint) *SlowProcessingEvent {
	durationUs := duration.Microseconds()

	var maxGapUs int64
	var maxGapFrom string
	var maxGapTo string
	for i := range trace {
		nextElapsedUs := durationUs
		nextName := "done"
		if i+1 < len(trace) {
			nextElapsedUs = trace[i+1].ElapsedUs
			nextName = trace[i+1].Name
		}
		gap := nextElapsedUs - trace[i].ElapsedUs
		if gap > maxGapUs {
			maxGapUs = gap
			maxGapFrom = trace[i].Name
			maxGapTo = nextName
		}
	}

	evt := &SlowProcessingEvent{
		EventType:              eventType.String(),
		DurationUs:             durationUs,
		ProcessingTimeMicrosec: uint32(durationUs),
		ProcessingTrace:        trace,
		MaxProcessingGapUs:     maxGapUs,
		MaxProcessingGapFrom:   maxGapFrom,
		MaxProcessingGapTo:     maxGapTo,
	}
	evt.FillCustomEventCommonFields(acc)
	return evt
}

type slowEventWatchdog struct {
	sampleInterval  time.Duration
	reportThreshold time.Duration
	targetFrame     string

	startNano  atomic.Int64
	activeSeq  atomic.Uint64
	hasSamples atomic.Bool

	mu          sync.Mutex
	sampleSeq   uint64
	checkpoints []ProcessingCheckpoint
	stackBuf    []byte
}

func newSlowEventWatchdog(sampleInterval, reportThreshold time.Duration) *slowEventWatchdog {
	return &slowEventWatchdog{
		sampleInterval:  sampleInterval,
		reportThreshold: reportThreshold,
		targetFrame:     defaultWatchdogTargetFrame,
		stackBuf:        make([]byte, initialWatchdogStackBufSize),
	}
}

func (w *slowEventWatchdog) start(ctx context.Context) {
	if w == nil || w.sampleInterval <= 0 {
		return
	}
	ticker := time.NewTicker(w.sampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			w.sampleOnce(now)
		}
	}
}

func (w *slowEventWatchdog) begin() time.Time {
	now := time.Now()
	if w == nil {
		return now
	}
	w.activeSeq.Add(1)
	w.startNano.Store(now.UnixNano())
	return now
}

func (w *slowEventWatchdog) end(start time.Time) (time.Duration, []ProcessingCheckpoint, bool) {
	if w == nil {
		return 0, nil, false
	}
	w.startNano.Store(0)
	duration := time.Since(start)
	if duration < w.sampleInterval && duration < w.reportThreshold && !w.hasSamples.Load() {
		return duration, nil, false
	}

	seq := w.activeSeq.Load()
	w.mu.Lock()
	var trace []ProcessingCheckpoint
	if duration >= w.reportThreshold && w.sampleSeq == seq && len(w.checkpoints) > 0 {
		trace = slices.Clone(w.checkpoints)
	}
	w.checkpoints = w.checkpoints[:0]
	w.hasSamples.Store(false)
	w.mu.Unlock()

	if duration < w.reportThreshold {
		return duration, nil, false
	}
	return duration, trace, true
}

func (w *slowEventWatchdog) sampleOnce(now time.Time) {
	startNano := w.startNano.Load()
	if startNano == 0 {
		return
	}
	elapsedNano := now.UnixNano() - startNano
	if elapsedNano < int64(w.sampleInterval) {
		return
	}
	seq := w.activeSeq.Load()

	allStacks := w.captureAllStacks()
	if w.startNano.Load() != startNano || w.activeSeq.Load() != seq {
		return
	}

	name, stack := extractTargetStack(allStacks, w.targetFrame)
	if name == "" {
		return
	}
	elapsedUs := elapsedNano / int64(time.Microsecond)

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.startNano.Load() != startNano || w.activeSeq.Load() != seq {
		return
	}
	if w.sampleSeq != seq {
		w.checkpoints = w.checkpoints[:0]
		w.sampleSeq = seq
	}

	if n := len(w.checkpoints); n > 0 && (w.checkpoints[n-1].Name == name || n >= maxSlowEventCheckpoints) {
		last := &w.checkpoints[n-1]
		last.LastElapsedUs = elapsedUs
		last.Samples++
		w.hasSamples.Store(true)
		return
	}

	w.checkpoints = append(w.checkpoints, ProcessingCheckpoint{
		Name:          name,
		Stack:         stack,
		ElapsedUs:     elapsedUs,
		LastElapsedUs: elapsedUs,
		Samples:       1,
	})
	w.hasSamples.Store(true)
}

func (w *slowEventWatchdog) captureAllStacks() string {
	for {
		n := runtime.Stack(w.stackBuf, true)
		if n < len(w.stackBuf) || len(w.stackBuf) >= maxWatchdogStackBufSize {
			return string(w.stackBuf[:n])
		}
		w.stackBuf = make([]byte, min(len(w.stackBuf)*2, maxWatchdogStackBufSize))
	}
}

func extractTargetStack(allStacks, targetFrame string) (string, string) {
	sections := strings.Split(allStacks, "\n\n")
	var targetSection string
	for _, section := range sections {
		if isGoroutineWithFrame(section, targetFrame) {
			targetSection = strings.TrimSpace(section)
			break
		}
	}
	if targetSection == "" {
		return "", ""
	}

	state, callChain := summarizeGoroutineSection(targetSection, targetFrame)
	if callChain == "" {
		return "", ""
	}
	name := callChain
	if state != "" {
		name = "[" + state + "] " + callChain
	}

	if !isBlockedGoroutineState(state) {
		return name, targetSection
	}

	var b strings.Builder
	b.WriteString(targetSection)
	added := 0
	for _, section := range sections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" || trimmed == targetSection {
			continue
		}
		if !isActiveSecurityGoroutine(trimmed) {
			continue
		}
		b.WriteString("\n\n")
		b.WriteString(trimmed)
		added++
		if added >= maxRelatedSecurityGoroutines {
			break
		}
	}
	return name, b.String()
}

func isGoroutineWithFrame(section, targetFrame string) bool {
	for line := range strings.SplitSeq(section, "\n") {
		if strings.HasPrefix(line, "created by ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.Contains(line, targetFrame) {
			return true
		}
	}
	return false
}

func summarizeGoroutineSection(section, targetFrame string) (string, string) {
	lines := strings.Split(section, "\n")
	if len(lines) == 0 {
		return "", ""
	}

	state := parseGoroutineState(lines[0])

	type frame struct {
		fn  string
		loc string
		raw string
	}
	var frames []frame
	for i := 1; i < len(lines); i++ {
		fnLine := lines[i]
		if strings.HasPrefix(fnLine, "created by ") || strings.HasPrefix(fnLine, "\t") || fnLine == "" {
			continue
		}
		var locLine string
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\t") {
			locLine = strings.TrimSpace(lines[i+1])
			i++
		}
		frames = append(frames, frame{
			fn:  trimFrameFunc(fnLine),
			loc: trimFrameLocation(locLine),
			raw: fnLine,
		})
	}
	if len(frames) == 0 {
		return state, ""
	}

	startIdx := len(frames) - 1
	for i, f := range frames {
		if strings.Contains(f.raw, targetFrame) {
			startIdx = i
			break
		}
	}

	var parts []string
	for i := startIdx; i >= 0; i-- {
		f := frames[i]
		if f.loc != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", f.fn, f.loc))
		} else {
			parts = append(parts, f.fn)
		}
	}
	return state, strings.Join(parts, " -> ")
}

func parseGoroutineState(header string) string {
	openIdx := strings.IndexByte(header, '[')
	closeIdx := strings.LastIndexByte(header, ']')
	if openIdx < 0 || closeIdx <= openIdx {
		return ""
	}
	return header[openIdx+1 : closeIdx]
}

func isBlockedGoroutineState(state string) bool {
	head, _, _ := strings.Cut(state, ",")
	switch head {
	case "", "running", "runnable", "syscall":
		return false
	default:
		return true
	}
}

func isActiveSecurityGoroutine(section string) bool {
	if !strings.Contains(section, "pkg/security/") || strings.Contains(section, "slowEventWatchdog") {
		return false
	}
	firstLine, _, _ := strings.Cut(section, "\n")
	state, _, _ := strings.Cut(parseGoroutineState(firstLine), ",")
	switch state {
	case "select", "chan receive", "IO wait", "sleep":
		return false
	default:
		return true
	}
}

func trimFrameFunc(fnLine string) string {
	fn := stripTrailingCallArgs(strings.TrimSpace(fnLine))
	if idx := strings.Index(fn, "pkg/security/"); idx >= 0 {
		return fn[idx+len("pkg/security/"):]
	}
	if idx := strings.Index(fn, "datadog-agent/"); idx >= 0 {
		return fn[idx+len("datadog-agent/"):]
	}
	return fn
}

func stripTrailingCallArgs(fn string) string {
	if !strings.HasSuffix(fn, ")") {
		return fn
	}
	depth := 0
	for i := len(fn) - 1; i >= 0; i-- {
		switch fn[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return fn[:i]
			}
		}
	}
	return fn
}

func trimFrameLocation(locLine string) string {
	if locLine == "" {
		return ""
	}
	fileLine, _, _ := strings.Cut(locLine, " ")
	return filepath.Base(fileLine)
}
