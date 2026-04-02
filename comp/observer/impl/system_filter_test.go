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

func makeHFHandle(sources map[metrics.MetricSource]struct{}) *hfFilteredHandle {
	return &hfFilteredHandle{inner: &countingHandle{}, sources: sources}
}

func TestHFFilteredHandle_SystemSources(t *testing.T) {
	tests := []struct {
		name     string
		sample   observerdef.MetricView
		wantDrop bool
	}{
		// System check sources — all must be dropped when system HF is active.
		{"cpu dropped", &sampleWithSource{"system.cpu.user", metrics.MetricSourceCPU}, true},
		{"load dropped", &sampleWithSource{"system.load.1", metrics.MetricSourceLoad}, true},
		{"memory dropped", &sampleWithSource{"system.mem.used", metrics.MetricSourceMemory}, true},
		{"io dropped", &sampleWithSource{"system.io.r_s", metrics.MetricSourceIo}, true},
		{"disk dropped", &sampleWithSource{"system.disk.in_use", metrics.MetricSourceDisk}, true},
		{"network dropped", &sampleWithSource{"system.net.bytes_rcvd", metrics.MetricSourceNetwork}, true},
		{"uptime dropped", &sampleWithSource{"system.uptime", metrics.MetricSourceUptime}, true},
		{"filehandle dropped", &sampleWithSource{"system.fs.file_handles.used", metrics.MetricSourceFileHandle}, true},

		// Non-system sources must pass through.
		{"dogstatsd passes", &sampleWithSource{"my.custom.metric", metrics.MetricSourceDogstatsd}, false},
		{"k8s passes", &sampleWithSource{"kubernetes_state.pod.ready", metrics.MetricSourceKubernetesStateCore}, false},
		// Container source not in system filter set — passes through.
		{"container passes", &sampleWithSource{"container.cpu.usage", metrics.MetricSourceContainer}, false},

		// MetricSourceUnknown and no-sourceProvider must always pass through.
		{"unknown source passes", &sampleWithSource{"system.cpu.user", metrics.MetricSourceUnknown}, false},
		{"no source metadata passes", &sampleNoSource{"system.cpu.user"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := makeHFHandle(systemCheckSources)
			f.ObserveMetric(tt.sample)
			inner := f.inner.(*countingHandle)
			if tt.wantDrop && inner.received != 0 {
				t.Errorf("expected drop, got %d observation(s)", inner.received)
			}
			if !tt.wantDrop && inner.received != 1 {
				t.Errorf("expected pass-through, got %d observation(s)", inner.received)
			}
		})
	}
}

func TestHFFilteredHandle_ContainerSources(t *testing.T) {
	tests := []struct {
		name     string
		sample   observerdef.MetricView
		wantDrop bool
	}{
		// Generic container check source — dropped when container HF is active.
		// Only MetricSourceContainer is suppressed because the HF runner uses the
		// generic "container" check, not the legacy per-runtime checks.
		{"container dropped", &sampleWithSource{"container.cpu.usage", metrics.MetricSourceContainer}, true},

		// Legacy per-runtime sources are NOT suppressed (not run by HF runner).
		{"containerd passes", &sampleWithSource{"container.mem.usage", metrics.MetricSourceContainerd}, false},
		{"cri passes", &sampleWithSource{"container.io.read", metrics.MetricSourceCri}, false},
		{"docker passes", &sampleWithSource{"docker.cpu.usage", metrics.MetricSourceDocker}, false},

		// Non-container sources must pass through.
		{"dogstatsd passes", &sampleWithSource{"my.custom.metric", metrics.MetricSourceDogstatsd}, false},
		// System source not in container filter set — passes through.
		{"cpu passes", &sampleWithSource{"system.cpu.user", metrics.MetricSourceCPU}, false},
		{"unknown passes", &sampleWithSource{"container.cpu.usage", metrics.MetricSourceUnknown}, false},
		{"no source passes", &sampleNoSource{"container.cpu.usage"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := makeHFHandle(containerCheckSources)
			f.ObserveMetric(tt.sample)
			inner := f.inner.(*countingHandle)
			if tt.wantDrop && inner.received != 0 {
				t.Errorf("expected drop, got %d observation(s)", inner.received)
			}
			if !tt.wantDrop && inner.received != 1 {
				t.Errorf("expected pass-through, got %d observation(s)", inner.received)
			}
		})
	}
}

func TestHFFilteredHandle_BothEnabled(t *testing.T) {
	// When both flags are active the combined source set suppresses both categories.
	combined := make(map[metrics.MetricSource]struct{})
	for src := range systemCheckSources {
		combined[src] = struct{}{}
	}
	for src := range containerCheckSources {
		combined[src] = struct{}{}
	}

	f := makeHFHandle(combined)

	// System source dropped.
	f.ObserveMetric(&sampleWithSource{"system.cpu.user", metrics.MetricSourceCPU})
	// Container source dropped.
	f.ObserveMetric(&sampleWithSource{"container.cpu.usage", metrics.MetricSourceContainer})
	// DogStatsD passes through.
	f.ObserveMetric(&sampleWithSource{"my.metric", metrics.MetricSourceDogstatsd})

	inner := f.inner.(*countingHandle)
	if inner.received != 1 {
		t.Errorf("expected 1 observation (dogstatsd only), got %d", inner.received)
	}
}
