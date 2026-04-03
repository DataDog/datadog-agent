// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// sampleWithSource implements MetricView and sourceProvider.
type sampleWithSource struct {
	name   string
	source metrics.MetricSource
}

func (s *sampleWithSource) GetName() string                 { return s.name }
func (s *sampleWithSource) GetValue() float64               { return 0 }
func (s *sampleWithSource) GetRawTags() []string            { return nil }
func (s *sampleWithSource) GetTimestampUnix() int64         { return 0 }
func (s *sampleWithSource) GetSampleRate() float64          { return 1 }
func (s *sampleWithSource) GetSource() metrics.MetricSource { return s.source }

// sampleNoSource implements MetricView only — no sourceProvider.
type sampleNoSource struct{ name string }

func (s *sampleNoSource) GetName() string         { return s.name }
func (s *sampleNoSource) GetValue() float64       { return 0 }
func (s *sampleNoSource) GetRawTags() []string    { return nil }
func (s *sampleNoSource) GetTimestampUnix() int64 { return 0 }
func (s *sampleNoSource) GetSampleRate() float64  { return 1 }

// countingHandle records how many MetricView observations it receives.
type countingHandle struct {
	received int
}

func (h *countingHandle) ObserveMetric(_ observerdef.MetricView)         { h.received++ }
func (h *countingHandle) ObserveLog(_ observerdef.LogView)               {}
func (h *countingHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (h *countingHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (h *countingHandle) ObserveProfile(_ observerdef.ProfileView)       {}

func TestSystemFilteredHandle(t *testing.T) {
	tests := []struct {
		name     string
		sample   observerdef.MetricView
		wantDrop bool
	}{
		// System check sources — all must be dropped.
		{"cpu dropped", &sampleWithSource{"system.cpu.user", metrics.MetricSourceCPU}, true},
		{"load dropped", &sampleWithSource{"system.load.1", metrics.MetricSourceLoad}, true},
		{"memory dropped", &sampleWithSource{"system.mem.used", metrics.MetricSourceMemory}, true},
		{"io dropped", &sampleWithSource{"system.io.r_s", metrics.MetricSourceIo}, true},
		{"disk dropped", &sampleWithSource{"system.disk.in_use", metrics.MetricSourceDisk}, true},
		{"network dropped", &sampleWithSource{"system.net.bytes_rcvd", metrics.MetricSourceNetwork}, true},
		{"uptime dropped", &sampleWithSource{"system.uptime", metrics.MetricSourceUptime}, true},
		{"filehandle dropped", &sampleWithSource{"system.fs.file_handles.used", metrics.MetricSourceFileHandle}, true},

		// Non-system sources with source metadata — must pass through.
		{"dogstatsd passes", &sampleWithSource{"my.custom.metric", metrics.MetricSourceDogstatsd}, false},
		{"k8s passes", &sampleWithSource{"kubernetes_state.pod.ready", metrics.MetricSourceKubernetesStateCore}, false},
		{"container passes", &sampleWithSource{"container.cpu.usage", metrics.MetricSourceContainer}, false},

		// MetricSourceUnknown with source metadata — must pass through.
		// Dropping on unknown source would be overly aggressive.
		{"unknown source passes", &sampleWithSource{"system.cpu.user", metrics.MetricSourceUnknown}, false},

		// No sourceProvider at all — must pass through.
		// There is no string-prefix fallback; absence of metadata means we
		// cannot confidently identify the source, so we let it through.
		{"no source metadata passes", &sampleNoSource{"system.cpu.user"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &countingHandle{}
			f := &systemFilteredHandle{inner: inner}
			f.ObserveMetric(tt.sample)

			if tt.wantDrop && inner.received != 0 {
				t.Errorf("expected metric to be dropped, but inner handle received %d observation(s)", inner.received)
			}
			if !tt.wantDrop && inner.received != 1 {
				t.Errorf("expected metric to pass through, but inner handle received %d observation(s)", inner.received)
			}
		})
	}
}
