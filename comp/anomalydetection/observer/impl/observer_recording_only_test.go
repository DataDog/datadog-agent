// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
)

type recordingOnlyTestRecorder struct {
	inner   observerdef.Handle
	metrics int
	logs    int
}

func (r *recordingOnlyTestRecorder) GetHandle(inner observerdef.HandleFunc) observerdef.HandleFunc {
	return func(name string) observerdef.Handle {
		r.inner = inner(name)
		return &recordingOnlyTestHandle{inner: r.inner, recorder: r}
	}
}

type recordingOnlyTestHandle struct {
	inner    observerdef.Handle
	recorder *recordingOnlyTestRecorder
}

func (h *recordingOnlyTestHandle) ObserveMetric(metric observerdef.MetricView, contextKey uint64) {
	h.inner.ObserveMetric(metric, contextKey)
	h.recorder.metrics++
}

func (h *recordingOnlyTestHandle) ObserveLog(log observerdef.LogView) {
	h.inner.ObserveLog(log)
	h.recorder.logs++
}

func TestRecordingOnlyHandleRouting(t *testing.T) {
	for _, tt := range []struct {
		name         string
		only         bool
		recorder     bool
		consumer     bool
		wantAnalysis bool
	}{
		{name: "recording only", only: true, recorder: true},
		{name: "recording only overrides analysis consumer", only: true, recorder: true, consumer: true},
		{name: "recording with analysis consumer", recorder: true, consumer: true, wantAnalysis: true},
		{name: "recording without only", recorder: true, wantAnalysis: true},
		{name: "only without recorder keeps analysis", only: true, consumer: true, wantAnalysis: true},
		{name: "only alone does not activate observer", only: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.NewFromYAML(t, fmt.Sprintf(`
anomaly_detection:
  recording:
    enabled: %t
    only: %t
    output_dir: ""
  reporting:
    events:
      enabled: %t
  logs:
    internal:
      enabled: false
`, tt.recorder, tt.only, tt.consumer))
			recorder := &recordingOnlyTestRecorder{}
			recorderOption := option.None[recorderdef.Component]()
			if tt.recorder {
				recorderOption = option.New[recorderdef.Component](recorder)
			}
			provides, err := NewComponent(Requires{
				Lifecycle: &testLifecycle{},
				Config:    cfg,
				Telemetry: telemetryimpl.NewMock(t),
				Recorder:  recorderOption,
			})
			require.NoError(t, err)

			if !tt.recorder && !tt.consumer {
				_, disabled := provides.Comp.(*disabledObserver)
				require.True(t, disabled)
				return
			}
			obs, ok := provides.Comp.(*observerImpl)
			require.True(t, ok)
			t.Cleanup(func() { close(obs.obsCh) })

			h := obs.GetHandle("check")
			h.ObserveMetric(&metricObs{name: "system.cpu", value: 2.5, timestamp: 1000}, 1)
			h.ObserveLog(&logObs{content: "hello", status: "info", timestampMs: 1_000_000})
			obs.Flush()

			series := obs.engine.storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"})
			if tt.wantAnalysis {
				require.NotEmpty(t, series)
			} else {
				require.Empty(t, series)
			}
			if tt.recorder {
				require.Equal(t, 1, recorder.metrics)
				require.Equal(t, 1, recorder.logs)
				_, noAnalysis := recorder.inner.(*noopObserveHandle)
				require.Equal(t, !tt.wantAnalysis, noAnalysis)
			}
		})
	}
}
