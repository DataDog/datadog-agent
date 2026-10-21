// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

// Package connection provides tracing for connections
package connection

import (
	"errors"
	"maps"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
)

func TestFailedConnectionTelemetryMapLoads(t *testing.T) {
	tr, err := newEbpfTracer(config.New(), nil)
	require.NoError(t, err, "could not load tracer")
	t.Cleanup(tr.Stop)

	require.NotNil(t, tr.(*ebpfTracer).tcpFailuresTelemetryMap, "error loading tcp failure telemetry map")
}

// TestFentryLoadFailureFallsBackToKprobe loads the real fentry tracer, forces it to fail after
// eBPF telemetry instruction patching has run, and checks that the real kprobe tracer still
// loads. A failed fentry load must not leave state behind that breaks the fallback.
func TestFentryLoadFailureFallsBackToKprobe(t *testing.T) {
	// system-probe initializes the eBPF errors collector at startup; without it the telemetry
	// modifier never fills its maps, which hides failures that only happen in production
	ebpftelemetry.NewEBPFErrorsCollector()

	prevFentryLoader := fentryTracerLoader
	t.Cleanup(func() { fentryTracerLoader = prevFentryLoader })
	var fentryErr error
	fentryTracerLoader = func(c *config.Config, o manager.Options, h *perf.EventHandler) (*ddebpf.Manager, func(), error) {
		// an invalid map size fails inside the manager's collection load, after patching
		o.MapSpecEditors = maps.Clone(o.MapSpecEditors)
		o.MapSpecEditors[probes.ConnMap] = manager.MapSpecEditor{MaxEntries: 0, EditorFlag: manager.EditMaxEntries}
		var m *ddebpf.Manager
		var closeFn func()
		m, closeFn, fentryErr = fentry.LoadTracer(c, o, h)
		return m, closeFn, fentryErr
	}

	cfg := config.New()
	cfg.EnableFentry = true
	cfg.AllowKprobeFallback = true
	tr, err := newEbpfTracer(cfg, nil)
	if tr != nil {
		t.Cleanup(tr.Stop)
	}
	if errors.Is(fentryErr, fentry.ErrorUnsupported) {
		t.Skipf("fentry tracer not supported on this kernel: %v", fentryErr)
	}
	require.ErrorContains(t, fentryErr, probes.ConnMap, "fentry tracer did not fail where the test forced it to")
	require.NoError(t, err, "kprobe fallback failed after a fentry load failure")
	assert.NotEqual(t, TracerTypeFentry, tr.Type())
}

func TestFentryKprobeFallback(t *testing.T) {
	prevFentryLoader := fentryTracerLoader
	prevKprobeLoader := kprobeTracerLoader
	t.Cleanup(func() {
		fentryTracerLoader = prevFentryLoader
		kprobeTracerLoader = prevKprobeLoader
	})

	fakeManager := &ddebpf.Manager{}
	kprobeCalled := false

	fakeKprobeLoader := func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), kprobe.TracerType, error) {
		kprobeCalled = true
		return fakeManager, nil, kprobe.TracerTypeCORE, nil
	}

	t.Run("fentry succeeds", func(t *testing.T) {
		kprobeCalled = false
		fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
			return fakeManager, nil, nil
		}
		kprobeTracerLoader = fakeKprobeLoader

		m, _, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: true}, manager.Options{}, nil)
		require.NoError(t, err)
		assert.Equal(t, fakeManager, m)
		assert.Equal(t, TracerTypeFentry, tracerType)
		assert.False(t, kprobeCalled, "kprobe loader should not be called when fentry succeeds")
	})

	t.Run("fentry disabled falls back to kprobe", func(t *testing.T) {
		kprobeCalled = false
		fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
			return nil, nil, fentry.ErrorDisabled
		}
		kprobeTracerLoader = fakeKprobeLoader

		m, _, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: false}, manager.Options{}, nil)
		require.NoError(t, err)
		assert.Equal(t, fakeManager, m)
		assert.Equal(t, TracerTypeKProbeCORE, tracerType)
		assert.True(t, kprobeCalled)
	})

	t.Run("fentry error falls back to kprobe when allowed", func(t *testing.T) {
		kprobeCalled = false
		fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
			return nil, nil, assert.AnError
		}
		kprobeTracerLoader = fakeKprobeLoader

		m, _, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: true}, manager.Options{}, nil)
		require.NoError(t, err)
		assert.Equal(t, fakeManager, m)
		assert.Equal(t, TracerTypeKProbeCORE, tracerType)
		assert.True(t, kprobeCalled)
	})

	t.Run("fentry error is a hard failure when fallback disallowed", func(t *testing.T) {
		kprobeCalled = false
		fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
			return nil, nil, assert.AnError
		}
		kprobeTracerLoader = fakeKprobeLoader

		m, _, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: false}, manager.Options{}, nil)
		require.Error(t, err)
		assert.Nil(t, m)
		assert.Equal(t, TracerTypeFentry, tracerType)
		assert.False(t, kprobeCalled, "kprobe loader should not be called when fallback is disallowed")
	})

	t.Run("fentry verifier error falls back to kprobe when allowed", func(t *testing.T) {
		kprobeCalled = false
		verifierErr := &ebpf.VerifierError{Cause: assert.AnError, Log: []string{"invalid access to map value"}}
		fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
			return nil, nil, verifierErr
		}
		kprobeTracerLoader = fakeKprobeLoader

		m, _, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: true}, manager.Options{}, nil)
		require.NoError(t, err)
		assert.Equal(t, fakeManager, m)
		assert.Equal(t, TracerTypeKProbeCORE, tracerType)
		assert.True(t, kprobeCalled, "kprobe loader should be called on a verifier error when fallback is allowed")
	})

	t.Run("fentry verifier error is a hard failure when fallback disallowed", func(t *testing.T) {
		kprobeCalled = false
		verifierErr := &ebpf.VerifierError{Cause: assert.AnError, Log: []string{"invalid access to map value"}}
		fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
			return nil, nil, verifierErr
		}
		kprobeTracerLoader = fakeKprobeLoader

		m, _, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: false}, manager.Options{}, nil)
		require.Error(t, err)
		assert.Nil(t, m)
		assert.Equal(t, TracerTypeFentry, tracerType)
		assert.False(t, kprobeCalled, "kprobe loader should not be called when fallback is disallowed")
	})
}
