// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"strings"
	"testing"

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
	content := make([]byte, len(msg.GetContent()))
	copy(content, msg.GetContent())
	h.logs = append(h.logs, &agentLogView{
		content:     content,
		status:      msg.GetStatus(),
		tags:        tags,
		hostname:    msg.GetHostname(),
		timestampMs: msg.GetTimestampUnixMilli(),
	})
}

func TestInstallAgentLogTap(t *testing.T) {
	t.Cleanup(func() { pkglog.SetLogObserver(nil) })

	h := &captureHandle{}
	installAgentLogTap(h, 1.0, 1.0, 1.0) // all rates = 1.0 → forward everything

	simulateLogEmit(pkglog.InfoLvl, "test info message")
	simulateLogEmit(pkglog.WarnLvl, "test warn message")
	simulateLogEmit(pkglog.DebugLvl, "test debug message")

	require.Len(t, h.logs, 3)

	// Verify info log
	msg := h.logs[0]
	assert.Equal(t, "info", msg.GetStatus())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
	assert.Equal(t, "test info message", payload["msg"])
	assert.True(t, containsAgentLogTag(msg.Tags(), "source:datadog-agent"))
	assert.True(t, containsAgentLogTag(msg.Tags(), "level:info"))

	// Verify warn log
	assert.Equal(t, "warn", h.logs[1].GetStatus())

	// Verify debug log
	assert.Equal(t, "debug", h.logs[2].GetStatus())
}

func TestInstallAgentLogTapSampling(t *testing.T) {
	t.Cleanup(func() { pkglog.SetLogObserver(nil) })

	h := &captureHandle{}
	// Only forward WARN+ and 50% of INFO; block all DEBUG/TRACE
	installAgentLogTap(h, 0.5, 0.0, 0.0)

	// Emit 1000 INFO lines — roughly 500 should pass
	for i := 0; i < 1000; i++ {
		simulateLogEmit(pkglog.InfoLvl, "info")
	}
	// DEBUG should be completely dropped
	for i := 0; i < 100; i++ {
		simulateLogEmit(pkglog.DebugLvl, "debug")
	}
	// WARN always passes
	simulateLogEmit(pkglog.WarnLvl, "warn")
	simulateLogEmit(pkglog.ErrorLvl, "error")
	simulateLogEmit(pkglog.CriticalLvl, "critical")

	infoCount := 0
	for _, l := range h.logs {
		if l.GetStatus() == "info" {
			infoCount++
		}
	}
	// ~500 info, 0 debug, 3 warn+
	assert.InDelta(t, 500, infoCount, 50, "expected ~50%% of INFO to be sampled")

	// No debug should pass
	for _, l := range h.logs {
		assert.NotEqual(t, "debug", l.GetStatus())
	}

	// All warn/error/critical should pass
	warnish := 0
	for _, l := range h.logs {
		s := l.GetStatus()
		if s == "warn" || s == "error" || s == "critical" {
			warnish++
		}
	}
	assert.Equal(t, 3, warnish)
}

func TestSamplePass(t *testing.T) {
	// rate=0 → never pass
	for n := uint64(1); n <= 10; n++ {
		assert.False(t, samplePass(0, n))
	}
	// rate=1 → always pass
	for n := uint64(1); n <= 10; n++ {
		assert.True(t, samplePass(1.0, n))
	}
	// rate=0.5 → roughly half
	passes := 0
	for n := uint64(1); n <= 1000; n++ {
		if samplePass(0.5, n) {
			passes++
		}
	}
	assert.InDelta(t, 500, passes, 10)
}

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
