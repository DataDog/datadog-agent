// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profilerec

import (
	"expvar"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestProfileNamesExistInCatalog(t *testing.T) {
	assert.True(t, pkgconfigsetup.LogsPerformanceProfileExists(ProfileHighThroughput))
	assert.True(t, pkgconfigsetup.LogsPerformanceProfileExists(ProfileHighConcurrency))
}

func TestForBottleneck(t *testing.T) {
	tests := []struct {
		name        string
		component   string
		latencyMs   int64
		wantProfile string
		wantCode    string
		wantInText  string
	}{
		{"processor", "processor", 0, ProfileHighThroughput, ReasonProcessorSaturated, "processor"},
		{"strategy", "strategy", 0, ProfileHighThroughput, ReasonStrategySaturated, "compression"},
		{"worker", "worker", 0, ProfileHighConcurrency, ReasonSendStageSaturated, "network send"},
		{"sender", logsmetrics.SenderTlmName, 0, ProfileHighConcurrency, ReasonSendStageSaturated, "network send"},
		{"destination", "destination_reliable_0", 10, ProfileHighConcurrency, ReasonSendStageSaturated, "network send"},
		{"high latency", "destination_reliable_0", 400, ProfileHighConcurrency, ReasonSendStageSaturatedHighLatency, "400ms"},
		{"latency at threshold", "worker", SenderLatencyHighThresholdMs, ProfileHighConcurrency, ReasonSendStageSaturatedHighLatency, "latency"},
		{"unknown", "", 0, ProfileHighThroughput, ReasonPipelineSaturated, "saturated"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, code, reason := ForBottleneck(tt.component, tt.latencyMs)
			assert.Equal(t, tt.wantProfile, profile)
			assert.Equal(t, tt.wantCode, code)
			assert.Contains(t, reason, tt.wantInText)
			if code != ReasonSendStageSaturatedHighLatency {
				assert.NotContains(t, reason, "latency")
			}
		})
	}
}

func TestBottleneck(t *testing.T) {
	tests := []struct {
		name   string
		stages []Stage
		want   string
	}{
		{"none", []Stage{{Name: "processor"}, {Name: "worker"}}, ""},
		{
			"deepest currently saturated wins",
			[]Stage{
				{Name: "processor", CurrentlySaturated: true},
				{Name: "strategy", CurrentlySaturated: true},
				{Name: "worker", CurrentlySaturated: true},
				{Name: "destination_reliable_0", CurrentlySaturated: true},
			},
			"destination_reliable_0",
		},
		{
			"current saturation beats deeper recent saturation",
			[]Stage{
				{Name: "processor", CurrentlySaturated: true},
				{Name: "worker", Saturated30mSeconds: 100},
			},
			"processor",
		},
		{
			"recent saturation localizes when nothing is current",
			[]Stage{
				{Name: "processor", Saturated30mSeconds: 10},
				{Name: "strategy", Saturated1mSeconds: 30},
			},
			"strategy",
		},
		{
			"non-blocking destination cannot explain upstream loss",
			[]Stage{{Name: "destination_unreliable_0", CurrentlySaturated: true}},
			"",
		},
		{
			"non-blocking destination does not hide a blocking stage",
			[]Stage{
				{Name: "processor", CurrentlySaturated: true},
				{Name: "destination_unreliable_0", CurrentlySaturated: true},
			},
			"processor",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Bottleneck(tt.stages))
		})
	}
}

func TestIsSendStage(t *testing.T) {
	for _, c := range []string{"worker", logsmetrics.SenderTlmName, "destination_reliable_0", "destination_unreliable_0"} {
		assert.True(t, IsSendStage(c), c)
	}
	for _, c := range []string{"processor", "strategy", ""} {
		assert.False(t, IsSendStage(c), c)
	}
}

func TestStagesFromBackpressure(t *testing.T) {
	stages := StagesFromBackpressure([]logsmetrics.ComponentBackpressure{
		{Component: "worker", Instance: "0", CurrentlySaturated: true, Saturated1mSeconds: 5, Saturated30mSeconds: 50},
	})
	assert.Equal(t, []Stage{{Name: "worker", CurrentlySaturated: true, Saturated1mSeconds: 5, Saturated30mSeconds: 50}}, stages)
}

func TestRecommend(t *testing.T) {
	processorSat := []Stage{{Name: "processor", CurrentlySaturated: true, Saturated30mSeconds: 120}}
	workerSat := []Stage{
		{Name: "strategy", CurrentlySaturated: true, Saturated30mSeconds: 110},
		{Name: "worker", CurrentlySaturated: true, Saturated30mSeconds: 110},
	}
	tests := []struct {
		name       string
		stages     []Stage
		active     string
		signals    Signals
		wantNil    bool
		wantProf   string
		wantCode   string
		wantBottle string
	}{
		{
			name:       "missed with processor saturated",
			stages:     processorSat,
			signals:    Signals{MissedRecently: true, Delivering: true},
			wantProf:   ProfileHighThroughput,
			wantCode:   ReasonProcessorSaturated,
			wantBottle: "processor",
		},
		{
			name:       "missed with send stage saturated and high latency",
			stages:     workerSat,
			signals:    Signals{MissedRecently: true, Delivering: true, SenderLatencyMs: 500},
			wantProf:   ProfileHighConcurrency,
			wantCode:   ReasonSendStageSaturatedHighLatency,
			wantBottle: "worker",
		},
		{name: "no loss", stages: workerSat, signals: Signals{Delivering: true}, wantNil: true},
		{name: "missed with nothing saturated", stages: []Stage{{Name: "processor"}}, signals: Signals{MissedRecently: true, Delivering: true}, wantNil: true},
		{name: "missed with only non-blocking destination saturated", stages: []Stage{{Name: "destination_unreliable_0", CurrentlySaturated: true}}, signals: Signals{MissedRecently: true, Delivering: true}, wantNil: true},
		{name: "send stage not delivering", stages: workerSat, signals: Signals{MissedRecently: true}, wantNil: true},
		{name: "already on recommended profile", stages: workerSat, active: ProfileHighConcurrency, signals: Signals{MissedRecently: true, Delivering: true}, wantNil: true},
		{name: "active profile covers recommendation", stages: workerSat, active: ProfileHighThroughput, signals: Signals{MissedRecently: true, Delivering: true}, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := Recommend(tt.stages, tt.active, tt.signals)
			if tt.wantNil {
				assert.Nil(t, rec)
				return
			}
			require.NotNil(t, rec)
			assert.Equal(t, tt.wantProf, rec.Profile)
			assert.Equal(t, tt.wantCode, rec.ReasonCode)
			assert.Equal(t, tt.wantBottle, rec.Bottleneck)
			assert.Contains(t, rec.Reason, "Logs are being lost. ")
			assert.True(t, pkgconfigsetup.LogsPerformanceProfileExists(rec.Profile))
		})
	}
}

func TestReadCounters(t *testing.T) {
	assert.Equal(t, Counters{}, ReadCounters(nil))
	assert.Equal(t, Counters{}, ReadCounters(&expvar.Map{}))

	m := &expvar.Map{}
	m.Init()
	set := func(key string, v int64) {
		i := &expvar.Int{}
		i.Set(v)
		m.Set(key, i)
	}
	set("BytesMissed", 8192)
	set("MonitoredPipelineBytesMissed", 4096)
	set("LogsProcessed", 100)
	set("LogsSent", 90)
	set("DestinationErrors", 4)
	set("SenderLatency", 42)
	dropped := &expvar.Map{}
	dropped.Init()
	a, b := &expvar.Int{}, &expvar.Int{}
	a.Set(7)
	b.Set(3)
	dropped.Set("host-a", a)
	dropped.Set("host-b", b)
	m.Set("DestinationLogsDropped", dropped)

	assert.Equal(t, Counters{Dropped: 10, Missed: 4096, Processed: 100, Sent: 90, Errors: 4, SenderLatencyMs: 42}, ReadCounters(m))
}

func TestLossWindowRecency(t *testing.T) {
	var w LossWindow
	base := time.Unix(1000, 0)

	missed, _ := w.Observe(Counters{Missed: 5}, base)
	assert.False(t, missed, "the first observation only seeds the baseline")

	missed, _ = w.Observe(Counters{Missed: 7}, base.Add(time.Second))
	assert.True(t, missed)

	missed, _ = w.Observe(Counters{Missed: 7}, base.Add(2*time.Minute))
	assert.True(t, missed)

	missed, _ = w.Observe(Counters{Missed: 7}, base.Add(time.Second+LossRecencyWindow+time.Second))
	assert.False(t, missed, "stale loss must age out of the recency window")

	missed, _ = w.Observe(Counters{Missed: 8}, base.Add(time.Second+LossRecencyWindow+2*time.Second))
	assert.True(t, missed)
}

func TestLossWindowDelivering(t *testing.T) {
	var w LossWindow
	base := time.Unix(1000, 0)
	at := func(sec int) time.Time { return base.Add(time.Duration(sec) * time.Second) }

	_, delivering := w.Observe(Counters{}, at(0))
	assert.True(t, delivering, "the seed assumes delivery")

	_, delivering = w.Observe(Counters{Processed: 100}, at(1))
	assert.False(t, delivering, "processing without a send")

	_, delivering = w.Observe(Counters{Processed: 100, Sent: 50}, at(2))
	assert.True(t, delivering)
	_, delivering = w.Observe(Counters{Processed: 200, Sent: 50}, at(3))
	assert.False(t, delivering, "a warm outage is detected despite lifetime sends")

	_, delivering = w.Observe(Counters{Processed: 200, Sent: 150}, at(4))
	assert.True(t, delivering)
	_, delivering = w.Observe(Counters{Processed: 200, Sent: 150}, at(5))
	assert.True(t, delivering, "an idle pipeline keeps delivering")

	_, delivering = w.Observe(Counters{Processed: 200, Sent: 150, Errors: 3}, at(6))
	assert.False(t, delivering, "send errors without a send, with processing stalled behind them")
	_, delivering = w.Observe(Counters{Processed: 200, Sent: 150, Errors: 3}, at(7))
	assert.False(t, delivering, "a stalled pipeline during backoff stays not delivering")

	_, delivering = w.Observe(Counters{Processed: 200, Sent: 160, Errors: 5}, at(8))
	assert.True(t, delivering, "a send in the interval wins over errors from another destination")
}

func TestSamplerSignalsDoNotDependOnReads(t *testing.T) {
	counters := Counters{Missed: 10, Processed: 100, Sent: 100}
	base := time.Unix(1000, 0)
	s := newSampler(func() Counters { return counters }, base)

	missed, delivering := s.Signals()
	assert.False(t, missed)
	assert.True(t, delivering)

	counters.Missed, counters.Processed = 20, 200
	s.observe(base.Add(SampleInterval))
	for range 3 {
		missed, delivering = s.Signals()
		assert.True(t, missed, "repeated reads return the same observation")
		assert.False(t, delivering)
	}

	s.observe(base.Add(SampleInterval + LossRecencyWindow + time.Second))
	missed, _ = s.Signals()
	assert.False(t, missed, "loss ages out on the sampling clock")
}

func TestSamplerStop(t *testing.T) {
	s := StartSampler(func() Counters { return Counters{} })
	s.Stop()
	s.Stop()

	var nilSampler *Sampler
	nilSampler.Stop()
	missed, delivering := nilSampler.Signals()
	assert.False(t, missed)
	assert.True(t, delivering)
}
