// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

// Package connection provides tracing for connections
package connection

import (
	"errors"
	"fmt"
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

	verifierErr := &ebpf.VerifierError{Cause: assert.AnError, Log: []string{"invalid access to map value"}}
	// wrapped the same way fentry.LoadTracer wraps it
	unsupportedErr := fmt.Errorf("%w: %s", fentry.ErrorUnsupported, "tcp_recvmsg attach probe failed")
	kprobeErr := errors.New("kprobe load error")

	tests := []struct {
		name          string
		fentryErr     error
		kprobeErr     error
		allowFallback bool
		wantType      TracerType
		wantKprobe    bool
		// errors the returned error must wrap; empty means the load must succeed
		wantErrIs    []error
		wantNotErrIs []error
	}{
		{
			name:          "fentry succeeds",
			allowFallback: true,
			wantType:      TracerTypeFentry,
		},
		{
			// fallback disallowed, to show that disabled fentry bypasses the setting
			name:       "fentry disabled loads kprobe",
			fentryErr:  fentry.ErrorDisabled,
			wantType:   TracerTypeKProbeCORE,
			wantKprobe: true,
		},
		{
			name:          "fentry error falls back to kprobe when allowed",
			fentryErr:     assert.AnError,
			allowFallback: true,
			wantType:      TracerTypeKProbeCORE,
			wantKprobe:    true,
		},
		{
			name:      "fentry error is a hard failure when fallback disallowed",
			fentryErr: assert.AnError,
			wantType:  TracerTypeFentry,
			wantErrIs: []error{assert.AnError},
		},
		{
			name:          "fentry verifier error falls back to kprobe when allowed",
			fentryErr:     verifierErr,
			allowFallback: true,
			wantType:      TracerTypeKProbeCORE,
			wantKprobe:    true,
		},
		{
			name:      "fentry verifier error is a hard failure when fallback disallowed",
			fentryErr: verifierErr,
			wantType:  TracerTypeFentry,
			wantErrIs: []error{verifierErr},
		},
		{
			name:          "fentry unsupported falls back to kprobe when allowed",
			fentryErr:     unsupportedErr,
			allowFallback: true,
			wantType:      TracerTypeKProbeCORE,
			wantKprobe:    true,
		},
		{
			name:      "fentry unsupported is a hard failure when fallback disallowed",
			fentryErr: unsupportedErr,
			wantType:  TracerTypeFentry,
			wantErrIs: []error{fentry.ErrorUnsupported},
		},
		{
			name:          "kprobe failure after a fentry failure reports both causes",
			fentryErr:     assert.AnError,
			kprobeErr:     kprobeErr,
			allowFallback: true,
			wantType:      TracerTypeKProbeCORE,
			wantKprobe:    true,
			wantErrIs:     []error{kprobeErr, assert.AnError},
		},
		{
			name:         "kprobe failure with fentry disabled reports only the kprobe cause",
			fentryErr:    fentry.ErrorDisabled,
			kprobeErr:    kprobeErr,
			wantType:     TracerTypeKProbeCORE,
			wantKprobe:   true,
			wantErrIs:    []error{kprobeErr},
			wantNotErrIs: []error{fentry.ErrorDisabled},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fentryMgr, kprobeMgr := &ddebpf.Manager{}, &ddebpf.Manager{}
			var fentryClosed, kprobeClosed, kprobeCalled bool
			fentryTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), error) {
				if tt.fentryErr != nil {
					return nil, nil, tt.fentryErr
				}
				return fentryMgr, func() { fentryClosed = true }, nil
			}
			kprobeTracerLoader = func(*config.Config, manager.Options, *perf.EventHandler) (*ddebpf.Manager, func(), kprobe.TracerType, error) {
				kprobeCalled = true
				if tt.kprobeErr != nil {
					return nil, nil, kprobe.TracerTypeCORE, tt.kprobeErr
				}
				return kprobeMgr, func() { kprobeClosed = true }, kprobe.TracerTypeCORE, nil
			}

			m, closeFn, tracerType, err := loadFentryOrKprobeTracer(&config.Config{AllowKprobeFallback: tt.allowFallback}, manager.Options{}, nil)
			assert.Equal(t, tt.wantKprobe, kprobeCalled, "whether the kprobe loader was called")
			assert.Equal(t, tt.wantType, tracerType)

			if len(tt.wantErrIs) > 0 {
				require.Error(t, err)
				for _, target := range tt.wantErrIs {
					assert.ErrorIs(t, err, target)
				}
				for _, target := range tt.wantNotErrIs {
					assert.NotErrorIs(t, err, target)
				}
				assert.Nil(t, m)
				assert.Nil(t, closeFn)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, closeFn)
			closeFn()
			if tt.wantType == TracerTypeFentry {
				assert.Same(t, fentryMgr, m)
				assert.True(t, fentryClosed, "returned close func should be the fentry loader's")
			} else {
				assert.Same(t, kprobeMgr, m)
				assert.True(t, kprobeClosed, "returned close func should be the kprobe loader's")
			}
		})
	}
}
