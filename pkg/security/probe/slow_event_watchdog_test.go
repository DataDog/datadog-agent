// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"context"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.start(ctx)

	start := w.begin()
	_, trace, ok := w.end(start)
	assert.False(t, ok)
	assert.Empty(t, trace)
}

func TestSlowEventWatchdogBelowReportThresholdResetsSamples(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 500*time.Millisecond)
	w.targetFrame = "TestSlowEventWatchdogBelowReportThresholdResetsSamples"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.start(ctx)

	start := w.begin()
	time.Sleep(45 * time.Millisecond)
	_, trace, ok := w.end(start)
	assert.False(t, ok)
	assert.Empty(t, trace)
	assert.False(t, w.hasSamples.Load())
}

//go:noinline
func simulatedSlowStageA(d time.Duration) {
	time.Sleep(d)
}

//go:noinline
func simulatedSlowStageB(d time.Duration) {
	time.Sleep(d)
}

func TestSlowEventWatchdogCapturesAndCollapsesStacks(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 60*time.Millisecond)
	w.targetFrame = "TestSlowEventWatchdogCapturesAndCollapsesStacks"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.start(ctx)

	start := w.begin()
	simulatedSlowStageA(45 * time.Millisecond)
	simulatedSlowStageB(45 * time.Millisecond)
	duration, trace, ok := w.end(start)

	require.True(t, ok)
	require.NotEmpty(t, trace)

	var sawStageA, sawStageB bool
	for _, cp := range trace {
		if assert.NotEmpty(t, cp.Name) && assert.NotEmpty(t, cp.Stack) {
			if strings.Contains(cp.Name, "simulatedSlowStageA") {
				sawStageA = true
			}
			if strings.Contains(cp.Name, "simulatedSlowStageB") {
				sawStageB = true
			}
		}
	}
	assert.True(t, sawStageA, "expected stage A in trace: %+v", trace)
	assert.True(t, sawStageB, "expected stage B in trace: %+v", trace)

	evt := newSlowProcessingEvent(nil, model.FileOpenEventType, duration, trace)
	assert.Equal(t, "open", evt.EventType)
	assert.Greater(t, evt.DurationUs, int64(0))
	assert.Greater(t, evt.MaxProcessingGapUs, int64(0))
	assert.NotEmpty(t, evt.MaxProcessingGapFrom)
	assert.NotEmpty(t, evt.MaxProcessingGapTo)

	raw, err := evt.ToJSON()
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "open", decoded["event_type"])
	assert.NotEmpty(t, decoded["processing_trace"])
}

func TestExtractTargetStackIncludesActiveSecurityGoroutinesWhenBlocked(t *testing.T) {
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
	assert.NotContains(t, stack, "goroutine 99 [select]:")
}

func TestSlowEventWatchdogConcurrentMutexContention(t *testing.T) {
	w := newSlowEventWatchdog(10*time.Millisecond, 40*time.Millisecond)
	w.targetFrame = "TestSlowEventWatchdogConcurrentMutexContention"

	var mu sync.Mutex
	mu.Lock()
	var holderReady sync.WaitGroup
	holderReady.Add(1)
	go func() {
		holderReady.Done()
		time.Sleep(55 * time.Millisecond)
		mu.Unlock()
	}()
	holderReady.Wait()

	start := w.begin()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				w.sampleOnce(now)
			}
		}
	}()

	mu.Lock()
	//nolint:staticcheck // intentional critical section to wait for lock release
	mu.Unlock()
	close(done)

	_, trace, ok := w.end(start)
	require.True(t, ok)
	require.NotEmpty(t, trace)
	assert.GreaterOrEqual(t, trace[0].Samples, 1)
}
