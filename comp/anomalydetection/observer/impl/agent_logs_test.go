// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureHandle struct {
	logs []observerdef.LogView
}

func (h *captureHandle) ObserveMetric(_ observerdef.MetricView) {}
func (h *captureHandle) ObserveLog(msg observerdef.LogView) {
	// Copy tags so the captured view remains valid after the callback returns.
	tags := make([]string, len(msg.Tags()))
	copy(tags, msg.Tags())
	h.logs = append(h.logs, &agentLogView{
		content:     msg.GetContent(),
		status:      msg.GetStatus(),
		tags:        tags,
		hostname:    msg.GetHostname(),
		timestampMs: msg.GetTimestampUnixMilli(),
	})
}

type blockingHandle struct {
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
	observed    atomic.Int64
}

func newBlockingHandle() *blockingHandle {
	return &blockingHandle{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (h *blockingHandle) ObserveMetric(_ observerdef.MetricView) {}
func (h *blockingHandle) ObserveLog(_ observerdef.LogView) {
	h.startOnce.Do(func() { close(h.started) })
	<-h.release
	h.observed.Add(1)
}

func (h *blockingHandle) unblock() {
	h.releaseOnce.Do(func() { close(h.release) })
}

func TestInstallAgentLogTap(t *testing.T) {
	h := &captureHandle{}
	// -1 = unlimited for all priorities, no min_severity → forward everything including trace
	tap := installAgentLogTap(h, "", -1, -1, -1, nil, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.InfoLvl, "test info message")
	simulateLogEmit(pkglog.WarnLvl, "test warn message")
	simulateLogEmit(pkglog.DebugLvl, "test debug message")
	tap.stop()

	require.Len(t, h.logs, 3)

	// Verify info log
	msg := h.logs[0]
	assert.Equal(t, "info", msg.GetStatus())
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(msg.GetContent()), &payload))
	assert.Equal(t, "test info message", payload["msg"])
	assert.True(t, containsAgentLogTag(msg.Tags(), "source:datadog-agent"))
	assert.True(t, containsAgentLogTag(msg.Tags(), "level:info"))

	// Verify warn log
	assert.Equal(t, "warn", h.logs[1].GetStatus())

	// Verify debug log
	assert.Equal(t, "debug", h.logs[2].GetStatus())
}

func TestInstallAgentLogTapTraceTreatedLikeDebug(t *testing.T) {
	h := &captureHandle{}
	// min_severity="" → no filtering; trace falls into the low-priority bucket (same as debug).
	tap := installAgentLogTap(h, "", -1, -1, -1, nil, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.TraceLvl, "trace message")
	simulateLogEmit(pkglog.DebugLvl, "debug message")
	simulateLogEmit(pkglog.InfoLvl, "info message")
	tap.stop()

	require.Len(t, h.logs, 3, "trace, debug, and info must all be forwarded when min_severity is empty")
	assert.Equal(t, "trace", h.logs[0].GetStatus())
	assert.Equal(t, "debug", h.logs[1].GetStatus())
	assert.Equal(t, "info", h.logs[2].GetStatus())
}

func TestInstallAgentLogTapTraceFilteredByMinSeverity(t *testing.T) {
	h := &captureHandle{}
	var droppedPriorities []string
	// min_severity="warn" (default) → trace and debug are filtered before rate-limiting.
	tap := installAgentLogTap(h, "warn", -1, -1, -1, func(priority string) {
		droppedPriorities = append(droppedPriorities, priority)
	}, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.TraceLvl, "trace message")
	simulateLogEmit(pkglog.DebugLvl, "debug message")
	simulateLogEmit(pkglog.WarnLvl, "warn message")
	tap.stop()

	assert.Len(t, h.logs, 1, "only warn should be forwarded with min_severity=warn")
	assert.Equal(t, "warn", h.logs[0].GetStatus())
	assert.Empty(t, droppedPriorities, "severity-filtered drops must not trigger onDropped")
}

func TestInstallAgentLogTapDebugMinSeverity(t *testing.T) {
	h := &captureHandle{}
	// min_severity="debug" → trace is filtered out, debug and above pass.
	tap := installAgentLogTap(h, "debug", -1, -1, -1, nil, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.TraceLvl, "trace message")
	simulateLogEmit(pkglog.DebugLvl, "debug message")
	simulateLogEmit(pkglog.InfoLvl, "info message")
	tap.stop()

	require.Len(t, h.logs, 2, "debug and info should pass; trace should be filtered")
	assert.Equal(t, "debug", h.logs[0].GetStatus())
	assert.Equal(t, "info", h.logs[1].GetStatus())
}

func TestInstallAgentLogTapSeverityFilterDoesNotTriggerOnDropped(t *testing.T) {
	var dropped []string
	h := &captureHandle{}
	tap := installAgentLogTap(h, "error", -1, -1, -1, func(priority string) {
		dropped = append(dropped, priority)
	}, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.TraceLvl, "trace")
	simulateLogEmit(pkglog.DebugLvl, "debug")
	simulateLogEmit(pkglog.InfoLvl, "info")
	simulateLogEmit(pkglog.WarnLvl, "warn")
	tap.stop()

	assert.Empty(t, h.logs, "no logs should be forwarded below min_severity=error")
	assert.Empty(t, dropped, "severity-filtered drops must not fire onDropped")
}

func TestInstallAgentLogTapErrorMinSeverity(t *testing.T) {
	h := &captureHandle{}
	// min_severity="error" → only error/critical pass; warn is filtered out.
	tap := installAgentLogTap(h, "error", -1, -1, -1, nil, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.WarnLvl, "warn message")
	simulateLogEmit(pkglog.ErrorLvl, "error message")
	simulateLogEmit(pkglog.CriticalLvl, "critical message")
	tap.stop()

	require.Len(t, h.logs, 2, "only error and critical should pass min_severity=error")
	assert.Equal(t, "error", h.logs[0].GetStatus())
	assert.Equal(t, "critical", h.logs[1].GetStatus())
}

func TestInstallAgentLogTapRateLimit(t *testing.T) {
	var droppedPriorities []string
	h := &captureHandle{}
	// maxRateHigh=-1 (unlimited), maxRateMedium=10 (→100 per 10s window), maxRateLow=0.1 (→1 per 10s window)
	tap := installAgentLogTapWithQueueSize(
		h,
		"",
		-1,
		10,
		0.1,
		func(priority string) { droppedPriorities = append(droppedPriorities, priority) },
		nil,
		nil,
		300,
	)
	t.Cleanup(tap.stop)

	// Emit 200 INFO — should be capped at 100 (10 logs/s × 10s window)
	for i := 0; i < 200; i++ {
		simulateLogEmit(pkglog.InfoLvl, "info")
	}
	// Emit 10 DEBUG — should be capped at 1 (0.1 logs/s × 10s window)
	for i := 0; i < 10; i++ {
		simulateLogEmit(pkglog.DebugLvl, "debug")
	}
	// Emit 50 WARN — should all pass (unlimited)
	for i := 0; i < 50; i++ {
		simulateLogEmit(pkglog.WarnLvl, "warn")
	}
	tap.stop()

	var infoCount, debugCount, warnCount int
	for _, l := range h.logs {
		switch l.GetStatus() {
		case "info":
			infoCount++
		case "debug":
			debugCount++
		case "warn":
			warnCount++
		}
	}

	assert.Equal(t, 100, infoCount, "info should be rate-limited to 100 per 10s window")
	assert.Equal(t, 1, debugCount, "debug should be rate-limited to 1 per 10s window")
	assert.Equal(t, 50, warnCount, "warn should not be rate-limited (unlimited)")

	// Verify onDropped was called for rate-limited logs
	mediumDropped := 0
	lowDropped := 0
	for _, p := range droppedPriorities {
		switch p {
		case "medium":
			mediumDropped++
		case "low":
			lowDropped++
		}
	}
	assert.Equal(t, 100, mediumDropped, "100 info logs should have fired onDropped(medium)")
	assert.Equal(t, 9, lowDropped, "9 debug logs should have fired onDropped(low)")
}

func TestInstallAgentLogTapProcessingRulesExcludeByTag(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "drop_debug", Type: "exclude_at_match", Tags: []string{"level:debug"}},
	})
	require.NoError(t, err)

	h := &captureHandle{}
	tap := installAgentLogTap(h, "", -1, -1, -1, nil, nil, rules)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.DebugLvl, "debug message")
	simulateLogEmit(pkglog.InfoLvl, "info message")
	simulateLogEmit(pkglog.WarnLvl, "warn message")
	tap.stop()

	require.Len(t, h.logs, 2, "debug should be excluded by processing rule; info and warn should pass")
	assert.Equal(t, "info", h.logs[0].GetStatus())
	assert.Equal(t, "warn", h.logs[1].GetStatus())
}

func TestInstallAgentLogTapProcessingRulesExcludeBySource(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "drop_agent", Type: "exclude_at_match", Source: agentLogSource},
	})
	require.NoError(t, err)

	h := &captureHandle{}
	tap := installAgentLogTap(h, "", -1, -1, -1, nil, nil, rules)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.InfoLvl, "should be dropped")
	simulateLogEmit(pkglog.WarnLvl, "should also be dropped")
	tap.stop()

	assert.Empty(t, h.logs, "all agent logs should be excluded by source rule")
}

func TestInstallAgentLogTapDropsAnomalyDetectionLogsWithoutRateCharge(t *testing.T) {
	h := &captureHandle{}
	// 0.1/s allows exactly one info log in the 10-second rate window.
	tap := installAgentLogTap(h, "", -1, 0.1, -1, nil, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.InfoLvl, "[anomalydetection] observer configured")
	simulateLogEmit(pkglog.InfoLvl, "ordinary agent log")
	tap.stop()

	require.Len(t, h.logs, 1)
	assert.Contains(t, h.logs[0].GetContent(), "ordinary agent log")
}

func TestInstallAgentLogTapNilRulesAllowsAll(t *testing.T) {
	h := &captureHandle{}
	tap := installAgentLogTap(h, "", -1, -1, -1, nil, nil, nil)
	t.Cleanup(tap.stop)

	simulateLogEmit(pkglog.InfoLvl, "info")
	simulateLogEmit(pkglog.WarnLvl, "warn")
	tap.stop()

	require.Len(t, h.logs, 2, "nil rules should allow all logs")
}

func TestAgentLogTapDoesNotBlockWhenWorkerAndQueueAreFull(t *testing.T) {
	h := newBlockingHandle()
	var queueDropped atomic.Uint64
	tap := installAgentLogTapWithQueueSize(
		h,
		"",
		-1,
		-1,
		-1,
		nil,
		func(count uint64) { queueDropped.Add(count) },
		nil,
		1,
	)
	t.Cleanup(func() {
		h.unblock()
		tap.stop()
	})

	// The worker takes the first log and blocks in ObserveLog. The second log
	// fills the one-entry tap queue, so the third must be dropped immediately.
	simulateLogEmit(pkglog.InfoLvl, "worker blocked")
	select {
	case <-h.started:
	case <-time.After(time.Second):
		t.Fatal("agent-log worker did not begin processing")
	}
	simulateLogEmit(pkglog.InfoLvl, "queue full")

	callbackReturned := make(chan struct{})
	go func() {
		simulateLogEmit(pkglog.InfoLvl, "must not block")
		close(callbackReturned)
	}()
	select {
	case <-callbackReturned:
	case <-time.After(time.Second):
		t.Fatal("agent-log callback blocked on a full queue")
	}

	h.unblock()
	tap.stop()
	assert.Equal(t, int64(2), h.observed.Load(), "accepted logs should drain during shutdown")
	assert.Equal(t, uint64(1), queueDropped.Load(), "the full queue should drop exactly one log")
}

func TestAgentLogTapLogLevelsMatchPriorityBuckets(t *testing.T) {
	tests := []struct {
		name     string
		level    pkglog.LogLevel
		priority logsfilter.PriorityBucket
	}{
		{"trace", pkglog.TraceLvl, logsfilter.TracePriority},
		{"debug", pkglog.DebugLvl, logsfilter.DebugPriority},
		{"info", pkglog.InfoLvl, logsfilter.InfoPriority},
		{"warn", pkglog.WarnLvl, logsfilter.WarnPriority},
		{"error", pkglog.ErrorLvl, logsfilter.ErrorPriority},
		{"critical", pkglog.CriticalLvl, logsfilter.CriticalPriority},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.priority, logsfilter.PriorityBucket(test.level))
		})
	}
}

func TestAgentLogTapConcurrentEnqueueAndStop(t *testing.T) {
	const (
		iterations = 25
		producers  = 8
		emits      = 100
	)

	for range iterations {
		tap := installAgentLogTapWithQueueSize(
			&captureHandle{},
			"",
			-1,
			-1,
			-1,
			nil,
			nil,
			nil,
			1,
		)

		start := make(chan struct{})
		var producersDone sync.WaitGroup
		producersDone.Add(producers)
		for range producers {
			go func() {
				defer producersDone.Done()
				<-start
				for range emits {
					tap.enqueue(pkglog.InfoLvl, "concurrent shutdown")
				}
			}()
		}

		stopDone := make(chan struct{})
		go func() {
			<-start
			tap.stop()
			close(stopDone)
		}()

		close(start)
		producersDone.Wait()
		select {
		case <-stopDone:
		case <-time.After(time.Second):
			t.Fatal("agent-log tap shutdown deadlocked with concurrent producers")
		}
	}
}

func BenchmarkAgentLogTapEnqueueFullQueue(b *testing.B) {
	tap := &agentLogTap{
		minBucket: logsfilter.TracePriority - 1,
		queue:     make(chan queuedAgentLog, 1),
	}
	tap.queue <- queuedAgentLog{}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tap.enqueue(pkglog.InfoLvl, "benchmark")
	}
}

// RateWindow unit tests live in comp/anomalydetection/internal/logsfilter/logsfilter_test.go.

// simulateLogEmit calls the registered LogObserver directly, simulating what
// maybeObserve does inside pkg/util/log at every emit site.
func simulateLogEmit(level pkglog.LogLevel, message string) {
	pkglog.SimulateLogEmit(level, message) //nolint:staticcheck
}

func containsAgentLogTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}
