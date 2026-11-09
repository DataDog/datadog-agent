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
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/fentry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
)

func TestFailedConnectionTelemetryMapLoads(t *testing.T) {
	tr, err := newEbpfTracer(config.New(), nil, options{})
	require.NoError(t, err, "could not load tracer")
	t.Cleanup(tr.Stop)

	require.NotNil(t, tr.(*ebpfTracer).tcpFailuresTelemetryMap, "error loading tcp failure telemetry map")
}

func TestConntrackNATMapLoadFailureFallsBack(t *testing.T) {
	// a key size the placeholder doesn't have, so loading with it fails
	bad, err := ebpf.NewMap(&ebpf.MapSpec{Type: ebpf.Hash, KeySize: 4, ValueSize: 4, MaxEntries: 1})
	require.NoError(t, err)
	t.Cleanup(func() { bad.Close() })

	// the loaded programs' instructions aren't kept, so record the options the successful load got
	var loaded struct {
		mode   uint64
		shared bool
	}
	record := func(o manager.Options, err error) {
		if err != nil {
			return
		}
		loaded.shared = o.MapEditors[probes.ConntrackMap] != nil
		for _, c := range o.ConstantEditors {
			if c.Name == "nat_lookup_mode" {
				loaded.mode, _ = c.Value.(uint64)
			}
		}
	}
	prevFentryLoader, prevKprobeLoader := fentryTracerLoader, kprobeTracerLoader
	t.Cleanup(func() { fentryTracerLoader, kprobeTracerLoader = prevFentryLoader, prevKprobeLoader })
	fentryTracerLoader = func(c *config.Config, o manager.Options, h *perf.EventHandler) (*ddebpf.Manager, func(), error) {
		m, closeFn, err := prevFentryLoader(c, o, h)
		record(o, err)
		return m, closeFn, err
	}
	kprobeTracerLoader = func(c *config.Config, o manager.Options, h *perf.EventHandler) (*ddebpf.Manager, func(), kprobe.TracerType, error) {
		m, closeFn, tt, err := prevKprobeLoader(c, o, h)
		record(o, err)
		return m, closeFn, tt, err
	}

	cfg := config.New()
	tr, err := newEbpfTracer(cfg, nil, options{conntrackNATMap: bad})
	require.NoError(t, err, "tracer should load without the shared map")
	t.Cleanup(tr.Stop)
	if tr.Type() == TracerTypeSK {
		t.Skip("the sk tracer doesn't use the conntrack map")
	}

	// compare key sizes rather than map IDs, which old kernels don't report
	ct, err := tr.GetMap(probes.ConntrackMap)
	require.NoError(t, err)
	assert.Equal(t, uint32(unsafe.Sizeof(netebpf.ConntrackTuple{})), ct.KeySize(), "tracer kept the bad map")

	// net_dev_queue is loaded again, to be attached on start, with its map at full size
	skb, err := tr.GetMap(probes.ConnectionTupleToSocketSKBConnMap)
	require.NoError(t, err)
	assert.Equal(t, cfg.MaxTrackedConnections, skb.MaxEntries())
	mgr := tr.(*ebpfTracer).m.Manager
	classifies := kprobe.ClassificationSupported(cfg)
	if tr.Type() == TracerTypeFentry {
		classifies = fentry.ClassificationSupported(cfg)
	}
	if classifies {
		assert.True(t, programLoaded(t, mgr, probes.NetDevQueueRawTracepoint, probes.NetDevQueueTracepoint, probes.DevQueueXmitNitKprobe), "net_dev_queue isn't loaded")
	}

	// and the programs were loaded to look up NAT'd tuples in its map, not the conntracker's
	assert.Equal(t, natLookupSKBCache, loaded.mode, "nat_lookup_mode")
	assert.False(t, loaded.shared, "loaded with the conntracker's map")
}

// programLoaded reports whether a program with one of the given function names was loaded. Excluded programs
// aren't, and the tracer attaches the loaded ones when it starts.
func programLoaded(t *testing.T, m *manager.Manager, names ...probes.ProbeFuncName) bool {
	progs, err := m.GetPrograms()
	require.NoError(t, err)
	for _, name := range names {
		if _, ok := progs[string(name)]; ok {
			return true
		}
	}
	return false
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
	// the SK tracer is tried first; if the environment enables it, the fentry loader never runs
	cfg.EnableSKTracer = false
	cfg.EnableFentry = true
	cfg.AllowKprobeFallback = true
	tr, err := newEbpfTracer(cfg, nil, options{})
	if tr != nil {
		t.Cleanup(tr.Stop)
	}
	require.NoError(t, err, "kprobe fallback failed after a fentry load failure")
	assert.NotEqual(t, TracerTypeFentry, tr.Type())

	// on kernels without fentry support the fallback above ran, but the forced failure did not
	if errors.Is(fentryErr, fentry.ErrorUnsupported) {
		t.Logf("kprobe fallback verified; forced post-patching fentry failure not exercised, fentry not supported on this kernel: %v", fentryErr)
		return
	}
	require.ErrorContains(t, fentryErr, probes.ConnMap, "fentry tracer did not fail where the test forced it to")
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
			wantErrIs: []error{fentry.ErrorUnsupported},
		},
		{
			name:          "kprobe failure after a fentry failure reports both causes",
			fentryErr:     assert.AnError,
			kprobeErr:     kprobeErr,
			allowFallback: true,
			wantKprobe:    true,
			wantErrIs:     []error{kprobeErr, assert.AnError},
		},
		{
			name:         "kprobe failure with fentry disabled reports only the kprobe cause",
			fentryErr:    fentry.ErrorDisabled,
			kprobeErr:    kprobeErr,
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
			assert.Equal(t, tt.wantType, tracerType)
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

func TestConntrackNATMapUnsharedBeforeKprobeFallback(t *testing.T) {
	prevFentryLoader := fentryTracerLoader
	prevKprobeLoader := kprobeTracerLoader
	t.Cleanup(func() {
		fentryTracerLoader = prevFentryLoader
		kprobeTracerLoader = prevKprobeLoader
	})

	unsupportedErr := fmt.Errorf("%w: %s", fentry.ErrorUnsupported, "tcp_recvmsg attach probe failed")
	// errs are what each load returns while the map is shared, and once it's unshared
	type errs struct{ shared, unshared error }
	tests := []struct {
		name          string
		fentry        errs
		kprobe        errs
		allowFallback bool
		wantCalls     []string
		wantType      TracerType
		wantErr       bool
	}{
		{
			name:          "fentry retried unshared before kprobe",
			fentry:        errs{assert.AnError, nil},
			allowFallback: true,
			wantCalls:     []string{"fentry/shared", "fentry/unshared"},
			wantType:      TracerTypeFentry,
		},
		{
			name:          "kprobe loads unshared when fentry fails both ways",
			fentry:        errs{assert.AnError, assert.AnError},
			allowFallback: true,
			wantCalls:     []string{"fentry/shared", "fentry/unshared", "kprobe/unshared"},
			wantType:      TracerTypeKProbeCORE,
		},
		{
			name:      "fentry retried unshared without kprobe fallback",
			fentry:    errs{assert.AnError, assert.AnError},
			wantCalls: []string{"fentry/shared", "fentry/unshared"},
			wantErr:   true,
		},
		{
			name:      "disabled fentry isn't retried, kprobe is",
			fentry:    errs{fentry.ErrorDisabled, fentry.ErrorDisabled},
			kprobe:    errs{assert.AnError, nil},
			wantCalls: []string{"fentry/shared", "kprobe/shared", "kprobe/unshared"},
			wantType:  TracerTypeKProbeCORE,
		},
		{
			name:          "unsupported fentry isn't retried",
			fentry:        errs{unsupportedErr, unsupportedErr},
			allowFallback: true,
			wantCalls:     []string{"fentry/shared", "kprobe/shared"},
			wantType:      TracerTypeKProbeCORE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{AllowKprobeFallback: tt.allowFallback, MaxTrackedConnections: 1024}
			var calls []string
			// records the call, checking the map, its lookup mode and the skb cache size agree
			record := func(name string, o manager.Options, e errs) error {
				shared := o.MapEditors[probes.ConntrackMap] != nil
				mode, skbEntries := natLookupSKBCache, cfg.MaxTrackedConnections
				if shared {
					mode, skbEntries = natLookupConntrack, 1
				}
				assert.Contains(t, o.ConstantEditors, manager.ConstantEditor{Name: "nat_lookup_mode", Value: mode}, name)
				assert.Equal(t, skbEntries, o.MapSpecEditors[probes.ConnectionTupleToSocketSKBConnMap].MaxEntries, name)
				if shared {
					calls = append(calls, name+"/shared")
					return e.shared
				}
				calls = append(calls, name+"/unshared")
				return e.unshared
			}
			fentryTracerLoader = func(_ *config.Config, o manager.Options, _ *perf.EventHandler) (*ddebpf.Manager, func(), error) {
				if err := record("fentry", o, tt.fentry); err != nil {
					return nil, nil, err
				}
				return &ddebpf.Manager{}, func() {}, nil
			}
			kprobeTracerLoader = func(_ *config.Config, o manager.Options, _ *perf.EventHandler) (*ddebpf.Manager, func(), kprobe.TracerType, error) {
				if err := record("kprobe", o, tt.kprobe); err != nil {
					return nil, nil, kprobe.TracerTypeCORE, err
				}
				return &ddebpf.Manager{}, func() {}, kprobe.TracerTypeCORE, nil
			}

			o := manager.Options{MapSpecEditors: map[string]manager.MapSpecEditor{}}
			setNATLookupMode(&o, natLookupSKBCache)
			// the loaders are fakes, so the map is never used
			shareConntrackNATMap(&o, &ebpf.Map{})
			_, _, tracerType, err := loadFentryOrKprobeTracer(cfg, o, nil)
			assert.Equal(t, tt.wantCalls, calls)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, tracerType)
		})
	}
}
