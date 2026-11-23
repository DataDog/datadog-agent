// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestSlowEventWatchdogFastEventSkipped(t *testing.T) {
	w := newSlowEventWatchdog(20*time.Millisecond, 100*time.Millisecond)
	w.targetFrame = "TestSlowEventWatchdogFastEventSkipped"

	start := w.begin()
	w.sampleOnce(start.Add(5 * time.Millisecond))
	_, trace, ok := w.end(start)
	assert.False(t, ok)
	assert.Empty(t, trace)
}

func TestSlowEventWatchdogBelowReportThresholdResetsSamples(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 500*time.Millisecond)
	w.targetFrame = "TestSlowEventWatchdogBelowReportThresholdResetsSamples"

	start := w.begin()
	w.sampleOnce(start.Add(25 * time.Millisecond))
	assert.True(t, w.hasSamples.Load())

	_, trace, ok := w.end(start)
	assert.False(t, ok)
	assert.Empty(t, trace)
	assert.False(t, w.hasSamples.Load())
}

//go:noinline
func simulatedSlowStageA(entered chan<- struct{}, release <-chan struct{}) {
	close(entered)
	<-release
}

//go:noinline
func simulatedSlowStageB(entered chan<- struct{}, release <-chan struct{}) {
	close(entered)
	<-release
}

//go:noinline
func runSimulatedSlowEvent(w *slowEventWatchdog, enterA, releaseA, enterB, releaseB chan struct{}, done chan<- []ProcessingCheckpoint) {
	start := w.begin()
	simulatedSlowStageA(enterA, releaseA)
	simulatedSlowStageB(enterB, releaseB)
	_, trace, ok := w.end(start)
	if !ok {
		done <- nil
		return
	}
	done <- trace
}

func TestSlowEventWatchdogCapturesAndCollapsesStacks(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 0)
	w.targetFrame = "runSimulatedSlowEvent"

	enterA := make(chan struct{})
	releaseA := make(chan struct{})
	enterB := make(chan struct{})
	releaseB := make(chan struct{})
	done := make(chan []ProcessingCheckpoint, 1)

	go runSimulatedSlowEvent(w, enterA, releaseA, enterB, releaseB, done)

	<-enterA
	start := time.Unix(0, w.startNano.Load())
	w.sampleOnce(start.Add(50 * time.Millisecond))
	w.sampleOnce(start.Add(100 * time.Millisecond))
	close(releaseA)

	<-enterB
	w.sampleOnce(start.Add(150 * time.Millisecond))
	close(releaseB)

	trace := <-done
	require.Len(t, trace, 2)
	assert.Contains(t, trace[0].Name, "simulatedSlowStageA")
	assert.Equal(t, 2, trace[0].Samples)
	assert.Equal(t, int64(50000), trace[0].ElapsedUs)
	assert.Equal(t, int64(100000), trace[0].LastElapsedUs)
	assert.Contains(t, trace[1].Name, "simulatedSlowStageB")
	assert.Equal(t, 1, trace[1].Samples)

	evt := newSlowProcessingEvent(nil, model.FileOpenEventType, 200*time.Millisecond, trace)
	assert.Equal(t, "open", evt.EventType)
	assert.Equal(t, int64(200000), evt.DurationUs)
	assert.Equal(t, int64(100000), evt.MaxProcessingGapUs)
	assert.Contains(t, evt.MaxProcessingGapFrom, "simulatedSlowStageA")
	assert.Contains(t, evt.MaxProcessingGapTo, "simulatedSlowStageB")

	raw, err := evt.ToJSON()
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "open", decoded["event_type"])
	assert.NotEmpty(t, decoded["processing_trace"])
}

func TestExtractTargetStackIncludesActiveSecurityGoroutinesAndStripsArgs(t *testing.T) {
	fakeStacks := strings.Join([]string{
		"goroutine 17 [semacquire]:\n" +
			"sync.runtime_SemacquireMutex(0xc000123456?, 0x0?, 0x0?)\n" +
			"\t/usr/local/go/src/runtime/sema.go:95 +0x25\n" +
			"sync.(*Mutex).Lock(...)\n" +
			"\t/usr/local/go/src/sync/mutex.go:92\n" +
			"github.com/DataDog/datadog-agent/pkg/security/security_profile.(*ManagerV2).getOrCreateProfile(0xc000111000, {{0x1, 0x2}}, 0xc000222000)\n" +
			"\t/work/pkg/security/security_profile/manager_v2.go:625 +0x85\n" +
			"github.com/DataDog/datadog-agent/pkg/security/probe.(*EBPFProbe).handleEvent(0xc000333000, 0x0, {0xc000444000, 0x40, 0x40})\n" +
			"\t/work/pkg/security/probe/probe_ebpf.go:1726 +0x310",
		"goroutine 42 [running]:\n" +
			"github.com/DataDog/datadog-agent/pkg/security/security_profile.(*ManagerV2).evictUnusedNodes(0xc000111000)\n" +
			"\t/work/pkg/security/security_profile/manager_v2.go:955 +0x140",
		"goroutine 99 [select]:\n" +
			"github.com/DataDog/datadog-agent/pkg/security/probe.(*EBPFProbe).startSysCtlSnapshotLoop(0xc000333000)\n" +
			"\t/work/pkg/security/probe/probe_ebpf.go:2935 +0x90",
	}, "\n\n")

	name, stack := extractTargetStack(fakeStacks, defaultWatchdogTargetFrame)
	assert.Equal(t,
		"[semacquire] probe.(*EBPFProbe).handleEvent (probe_ebpf.go:1726) -> security_profile.(*ManagerV2).getOrCreateProfile (manager_v2.go:625) -> sync.(*Mutex).Lock (mutex.go:92) -> sync.runtime_SemacquireMutex (sema.go:95)",
		name,
	)
	assert.Contains(t, stack, "goroutine 17 [semacquire]:")
	assert.Contains(t, stack, "goroutine 42 [running]:")
	assert.Contains(t, stack, "evictUnusedNodes")
	assert.NotContains(t, stack, "0xc000")
	assert.NotContains(t, stack, "goroutine 99 [select]:")
}

func TestSlowEventWatchdogSampleCapPerEvent(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 0)
	w.targetFrame = "TestSlowEventWatchdogSampleCapPerEvent"

	start := w.begin()
	for i := 1; i <= maxSlowEventTotalSamples+10; i++ {
		w.sampleOnce(start.Add(time.Duration(i*10) * time.Millisecond))
	}
	_, trace, ok := w.end(start)
	require.True(t, ok)
	require.Len(t, trace, 1)
	assert.Equal(t, maxSlowEventTotalSamples, trace[0].Samples)
}

func TestSlowEventWatchdogConcurrentMutexContention(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 0)
	w.targetFrame = "TestSlowEventWatchdogConcurrentMutexContention"

	var mu sync.Mutex
	mu.Lock()
	sampled := make(chan struct{})

	start := w.begin()
	go func() {
		w.sampleOnce(start.Add(20 * time.Millisecond))
		mu.Unlock()
		close(sampled)
	}()

	mu.Lock()
	//nolint:staticcheck // intentional critical section to wait for lock release
	mu.Unlock()
	<-sampled

	_, trace, ok := w.end(start)
	require.True(t, ok)
	require.NotEmpty(t, trace)
	assert.GreaterOrEqual(t, trace[0].Samples, 1)
}
