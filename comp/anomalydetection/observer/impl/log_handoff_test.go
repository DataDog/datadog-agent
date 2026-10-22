// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type precheckOnlyLogView struct {
	source string
	status string
}

func (l *precheckOnlyLogView) GetSource() string          { return l.source }
func (l *precheckOnlyLogView) GetStatus() string          { return l.status }
func (*precheckOnlyLogView) Tags() []string               { panic("tags must not be read") }
func (*precheckOnlyLogView) GetContent() string           { panic("content must not be read") }
func (*precheckOnlyLogView) GetHostname() string          { panic("hostname must not be read") }
func (*precheckOnlyLogView) GetTimestampUnixMilli() int64 { panic("timestamp must not be read") }

type sourceOnlyPrecheckLogView struct {
	source string
}

func (l *sourceOnlyPrecheckLogView) GetSource() string          { return l.source }
func (*sourceOnlyPrecheckLogView) GetStatus() string            { panic("status must not be read") }
func (*sourceOnlyPrecheckLogView) Tags() []string               { panic("tags must not be read") }
func (*sourceOnlyPrecheckLogView) GetContent() string           { panic("content must not be read") }
func (*sourceOnlyPrecheckLogView) GetHostname() string          { panic("hostname must not be read") }
func (*sourceOnlyPrecheckLogView) GetTimestampUnixMilli() int64 { panic("timestamp must not be read") }

func TestLogHandoffRejectsSourceOnlyAndMinSeverityBeforeSnapshot(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "drop_containerd", Type: "exclude_at_match", Source: "containerd"},
	})
	require.NoError(t, err)

	telComp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(telComp)
	filter := &logAdmissionFilter{
		rules:      rules,
		containers: newLogSourceRateState("warn", -1, -1, -1),
	}

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "logs", telemetry: tel, logFilter: filter}

	// Conclusive source-only exclusion rejects before reading status, tags, or content.
	h.ObserveLog(&sourceOnlyPrecheckLogView{source: "containerd"})
	assert.Empty(t, ch)
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryFilteredLogs, nil).GetCounter().GetValue())

	// Below min_severity rejects before copying tags or reading content.
	h.ObserveLog(&precheckOnlyLogView{source: "docker", status: "info"})
	assert.Empty(t, ch)
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryFilteredLogs, nil).GetCounter().GetValue())
}

func TestLogHandoffSnapshotsReusableLogFieldsAndPreservesTagOrder(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "keep_prod_api", Type: "include_at_match", Tags: []string{"env:prod", "service:api"}},
		{Name: "drop_all", Type: "exclude_at_match"},
	})
	require.NoError(t, err)

	filter := &logAdmissionFilter{
		rules:      rules,
		containers: newLogSourceRateState("info", -1, -1, -1),
	}

	ch := make(chan observation, 1)
	h := &handle{ch: ch, source: "logs", logFilter: filter}

	callerTags := []string{"zeta:1", "service:api", "alpha:2", "env:prod"}
	sample := &logObs{
		source:      "nginx",
		content:     "GET /health 200",
		status:      "info",
		tags:        callerTags,
		hostname:    "host-a",
		timestampMs: 0,
	}

	before := time.Now()
	h.ObserveLog(sample)
	after := time.Now()

	// Mutate caller-owned slice and struct fields after ObserveLog returns.
	callerTags[0] = "mutated:true"
	callerTags[1] = "service:other"
	sample.source = "mutated-source"
	sample.content = "mutated-content"
	sample.status = "debug"
	sample.hostname = "host-b"

	queued := <-ch
	require.NotNil(t, queued.log)
	assert.Equal(t, "nginx", queued.log.GetSource())
	assert.Equal(t, "containers", queued.log.logSource)
	assert.Equal(t, "GET /health 200", queued.log.GetContent())
	assert.Equal(t, "info", queued.log.GetStatus())
	assert.Equal(t, "host-a", queued.log.GetHostname())
	assert.Equal(t, []string{"zeta:1", "service:api", "alpha:2", "env:prod"}, queued.log.Tags())
	assert.GreaterOrEqual(t, queued.log.GetTimestampUnixMilli(), before.UnixMilli())
	assert.LessOrEqual(t, queued.log.GetTimestampUnixMilli(), after.UnixMilli())

	admitted, reason := filter.admit(queued.log)
	assert.True(t, admitted)
	assert.Empty(t, reason)
	assert.Equal(t, []string{"zeta:1", "service:api", "alpha:2", "env:prod"}, queued.log.Tags())
}

func TestLogAdmissionExcludedLogsDoNotChargeRateBudgetAndUseAdmissionTime(t *testing.T) {
	rules, err := logsfilter.NewRules([]logsfilter.ProcessingRule{
		{Name: "keep_prod", Type: "include_at_match", Tags: []string{"env:prod"}},
		{Name: "drop_containerd", Type: "exclude_at_match", Source: "containerd"},
	})
	require.NoError(t, err)

	filter := &logAdmissionFilter{
		rules:      rules,
		containers: newLogSourceRateState("info", 0.1, 0.1, 0.1), // 1 per 10s window per tier
		kubelet:    newLogSourceRateState("info", 0.1, 0.1, 0.1),
	}

	t0 := time.Unix(1_700_000_000, 0)

	// Excluded by deferred tag rule: must not consume the 1-log container high-tier budget.
	admitted, reason := filter.admit(&logObs{
		source:        "containerd",
		logSource:     "containers",
		status:        "error",
		tags:          []string{"env:dev"},
		admissionTime: t0,
	})
	assert.False(t, admitted)
	assert.Equal(t, "filtered", reason)

	// Admitted prod container error consumes the single container high-tier slot at t0.
	admitted, reason = filter.admit(&logObs{
		source:        "containerd",
		logSource:     "containers",
		status:        "error",
		tags:          []string{"env:prod"},
		admissionTime: t0.Add(time.Second),
	})
	assert.True(t, admitted)
	assert.Empty(t, reason)

	// Second prod container error in the same 10s window is rate-limited.
	admitted, reason = filter.admit(&logObs{
		source:        "containerd",
		logSource:     "containers",
		status:        "error",
		tags:          []string{"env:prod"},
		admissionTime: t0.Add(2 * time.Second),
	})
	assert.False(t, admitted)
	assert.Equal(t, "high", reason)

	// Kubelet has an independent rate budget.
	admitted, reason = filter.admit(&logObs{
		source:        "kubelet",
		logSource:     "kubelet",
		status:        "error",
		tags:          []string{"source:kubelet", "env:prod"},
		admissionTime: t0.Add(2 * time.Second),
	})
	assert.True(t, admitted)
	assert.Empty(t, reason)

	// Container log admitted 10s after the first admitted log opens the next window based on admissionTime.
	admitted, reason = filter.admit(&logObs{
		source:        "containerd",
		logSource:     "containers",
		status:        "error",
		tags:          []string{"env:prod"},
		admissionTime: t0.Add(time.Second + logsfilter.RateWindowDuration),
	})
	assert.True(t, admitted)
	assert.Empty(t, reason)
}

type capturingLogRecorder struct {
	recorded []recorderdef.LogData
}

func (r *capturingLogRecorder) GetHandle(inner observerdef.HandleFunc) observerdef.HandleFunc {
	return inner
}

func (r *capturingLogRecorder) RecordLog(source string, msg observerdef.LogView) {
	r.recorded = append(r.recorded, recorderdef.LogData{
		Source:      source,
		TimestampMs: msg.GetTimestampUnixMilli(),
		Content:     []byte(msg.GetContent()),
		Status:      msg.GetStatus(),
		Hostname:    msg.GetHostname(),
		Tags:        copyTags(msg.Tags()),
	})
}

func TestLogHandoffBackpressureDropsWithoutRecordingOrRateCharge(t *testing.T) {
	telComp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(telComp)
	tel.initLogsInFlight()

	filter := &logAdmissionFilter{
		rules:      &logsfilter.Rules{},
		containers: newLogSourceRateState("warn", 0.1, 0.1, 0.1), // 1 per 10s window
	}

	ch := make(chan observation, 1)
	ch <- observation{} // Fill channel
	h := &handle{ch: ch, source: "logs", telemetry: tel, logFilter: filter}

	h.ObserveLog(&logObs{
		source:      "containerd",
		content:     "dropped on full queue",
		status:      "error",
		tags:        []string{"env:prod"},
		timestampMs: 1_000_000,
	})

	assert.Equal(t, int64(1), h.dropCount.Load())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsDropped, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
	assert.Equal(t, 0.0, observerMetric(t, telComp, telemetryLogsInFlightCount, map[string]string{"log_source": "containers"}).GetGauge().GetValue())

	// Rate window was not charged by the queue-dropped log.
	admitted, _ := filter.admit(&logObs{
		source:        "containerd",
		logSource:     "containers",
		status:        "error",
		tags:          []string{"env:prod"},
		admissionTime: time.Now(),
	})
	assert.True(t, admitted)
}

func TestObserverFilteredAsyncLogRecordingAndShutdownFlush(t *testing.T) {
	for _, recordingOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "analysis_and_recording", true: "recording_only"}[recordingOnly], func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  recording:
    enabled: true
    only: `+map[bool]string{false: "false", true: "true"}[recordingOnly]+`
    output_dir: ""
  reporting:
    events:
      enabled: true
  logs:
    internal:
      enabled: false
    containers:
      min_severity: warn
      max_rate_high_priority: 0.1
    processing_rules:
      - type: include_at_match
        name: keep_nginx_prod
        source: nginx
        tags: ["env:prod"]
      - type: exclude_at_match
        name: drop_containerd
        source: containerd
      - type: exclude_at_match
        name: drop_dev
        tags: ["env:dev"]
`)
			lc := &testLifecycle{}
			rec := &capturingLogRecorder{}
			telComp := telemetryimpl.NewMock(t)

			provides, err := NewComponent(Requires{
				Lifecycle: lc,
				Config:    cfg,
				Telemetry: telComp,
				Recorder:  option.New[recorderdef.Component](rec),
			})
			require.NoError(t, err)
			obs := provides.Comp.(*observerImpl)
			t.Cleanup(func() { close(obs.obsCh) })

			h := obs.GetHandle("logs")
			// 1. Source-only excluded (precheck).
			h.ObserveLog(&logObs{source: "containerd", content: "dropped-source", status: "error", tags: []string{"env:prod"}, timestampMs: 1_000})
			// 2. Tag-dependent excluded (deferred).
			h.ObserveLog(&logObs{source: "docker", content: "dropped-tag", status: "error", tags: []string{"env:dev"}, timestampMs: 2_000})
			// 3. Below min_severity=warn.
			h.ObserveLog(&logObs{source: "nginx", content: "dropped-severity", status: "info", tags: []string{"env:prod"}, timestampMs: 3_000})
			// 4. Admitted (first high-priority log in 10s window).
			h.ObserveLog(&logObs{source: "nginx", content: "admitted-1", status: "error", tags: []string{"service:web", "env:prod"}, timestampMs: 4_000})
			// 5. Rate-limited (second high-priority log in same 10s window at 0.1/s).
			h.ObserveLog(&logObs{source: "nginx", content: "dropped-rate", status: "error", tags: []string{"service:web", "env:prod"}, timestampMs: 5_000})

			require.Len(t, lc.hooks, 1)
			require.NoError(t, lc.hooks[0].OnStop(context.Background()))

			require.Len(t, rec.recorded, 1)
			assert.Equal(t, "logs", rec.recorded[0].Source)
			assert.Equal(t, []byte("admitted-1"), rec.recorded[0].Content)
			assert.Equal(t, []string{"service:web", "env:prod"}, rec.recorded[0].Tags)
			assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryFilteredLogs, nil).GetCounter().GetValue())
			assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryLogsInputRateLimiterDropped, map[string]string{"source": "containers", "priority": "high"}).GetCounter().GetValue())
			assert.Equal(t, 0.0, observerMetric(t, telComp, telemetryLogsInFlightCount, map[string]string{"log_source": "containers"}).GetGauge().GetValue())
		})
	}
}

func TestObserverFlushCtxHonorsExpiredContext(t *testing.T) {
	obs := &observerImpl{
		obsCh: make(chan observation), // unbuffered and no consumer running
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, obs.flushCtx(ctx), context.Canceled)
}
