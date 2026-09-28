// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

// Package connection provides tracing for connections
package connection

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
)

func TestFailedConnectionTelemetryMapLoads(t *testing.T) {
	tr, err := newEbpfTracer(config.New(), nil)
	require.NoError(t, err, "could not load tracer")
	t.Cleanup(tr.Stop)

	require.NotNil(t, tr.(*ebpfTracer).tcpFailuresTelemetryMap, "error loading tcp failure telemetry map")
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
