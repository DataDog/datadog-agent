// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && bpf

package telemetry

import (
	"os"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/prometheus/client_golang/prometheus"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"

	"github.com/stretchr/testify/require"
)

func TestModifierAppliesMultipleTimes(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)

	// set up the collector outside of the loop, as that's the usual usage in
	// system-probe
	collector := NewEBPFErrorsCollector()

	numTries := 4 // Just to be sure
	for i := 0; i < numTries; i++ {
		// Init the manager
		mgr := &manager.Manager{
			Probes: []*manager.Probe{
				{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: "kprobe__vfs_open",
					},
				},
			},
			Maps: []*manager.Map{
				{
					Name: "error_map",
				},
				{
					Name: "suppress_map",
				},
			},
		}
		t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) }) // Ensure we stop the manager, if it's already stopped it will be a no-op

		modifier := ErrorsTelemetryModifier{}
		mname := names.NewModuleName("ebpf")
		err := ddebpf.LoadCOREAsset("error_telemetry.o", func(buf bytecode.AssetReader, opts manager.Options) error {
			opts.RemoveRlimit = true
			opts.ActivatedProbes = []manager.ProbesSelector{
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFFuncName: "kprobe__vfs_open",
					},
				},
			}

			err := mgr.LoadELF(buf)
			require.NoError(t, err)

			err = modifier.BeforeInit(mgr, mname, &opts)
			require.NoError(t, err, "BeforeInit failed on try %d", i)
			err = mgr.InitWithOptions(nil, opts)
			require.NoError(t, err, "InitWithOptions failed on try %d", i)
			err = modifier.AfterInit(mgr, mname, &opts)
			require.NoError(t, err, "AfterInit failed on try %d", i)

			err = mgr.Start()
			require.NoError(t, err, "Start failed on try %d", i)

			return nil
		})
		require.NoError(t, err)

		// Trigger our kprobe
		_, err = os.Open("/proc/self/exe")
		require.NoError(t, err)

		ch := make(chan prometheus.Metric)
		go func() {
			collector.Collect(ch)
			close(ch)
		}()

		// Collect metrics from the channel
		var metrics []prometheus.Metric
		for m := range ch {
			metrics = append(metrics, m)
		}

		// Ensure we have metrics
		require.NotEmpty(t, metrics, "No metrics collected on try %d", i)

		// Run our BeforeStop
		err = modifier.BeforeStop(mgr, mname, manager.CleanAll)
		require.NoError(t, err, "BeforeStop failed on try %d", i)

		// Stop the manager
		require.NoError(t, mgr.Stop(manager.CleanAll), "Stop failed on try %d", i)
	}
}

// TestFailedLoadDoesNotLeakHelperKeys checks that a manager which fails to load after
// instruction patching leaves no helper-error keys behind under its module name, so the
// next manager initialized under the same module name (e.g. a fallback) can fill its own
// helper map, which is sized to its own program count.
func TestFailedLoadDoesNotLeakHelperKeys(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)
	NewEBPFErrorsCollector()

	modifier := ErrorsTelemetryModifier{}
	mname := names.NewModuleName("failed_load")

	err := ddebpf.LoadCOREAsset("error_telemetry.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		failed := &manager.Manager{}
		require.NoError(t, failed.LoadELF(buf))
		// distinct program names, so leaked keys can't collide with the second manager's keys
		specs, err := failed.GetProgramSpecs()
		require.NoError(t, err)
		for _, spec := range specs {
			spec.Name = "stale_" + spec.Name
		}

		opts.RemoveRlimit = true
		require.NoError(t, modifier.BeforeInit(failed, mname, &opts))
		// an invalid map size fails the collection load, which runs after instruction patching
		opts.MapSpecEditors["error_map"] = manager.MapSpecEditor{MaxEntries: 0, EditorFlag: manager.EditMaxEntries}
		require.Error(t, failed.InitWithOptions(nil, opts), "the first manager was expected to fail to load")
		return nil
	})
	require.NoError(t, err)

	err = ddebpf.LoadCOREAsset("error_telemetry.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		mgr := &manager.Manager{
			Probes: []*manager.Probe{
				{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__vfs_open"}},
			},
		}
		opts.RemoveRlimit = true
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__vfs_open"}},
		}
		require.NoError(t, mgr.LoadELF(buf))
		require.NoError(t, modifier.BeforeInit(mgr, mname, &opts))
		require.NoError(t, mgr.InitWithOptions(nil, opts))
		t.Cleanup(func() {
			_ = modifier.BeforeStop(mgr, mname, manager.CleanAll)
			_ = mgr.Stop(manager.CleanAll)
		})
		require.NoError(t, modifier.AfterInit(mgr, mname, &opts), "second manager under the same module name failed to fill its telemetry maps")
		return nil
	})
	require.NoError(t, err)
}

// TestRejectedFillLeavesNoKeys checks that when a second manager is initialized under a
// module name that is still registered, its fill is rejected without registering any of
// its helper-error keys.
func TestRejectedFillLeavesNoKeys(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)
	NewEBPFErrorsCollector()

	modifier := ErrorsTelemetryModifier{}
	mname := names.NewModuleName("rejected_fill")
	probeKeysForModule := func() int {
		et := errorsTelemetry.(*ebpfTelemetry)
		et.mtx.Lock()
		defer et.mtx.Unlock()
		n := 0
		for k := range et.probeKeys {
			if k.moduleName == mname {
				n++
			}
		}
		return n
	}

	initManager := func(namePrefix string) error {
		var afterInitErr error
		err := ddebpf.LoadCOREAsset("error_telemetry.o", func(buf bytecode.AssetReader, opts manager.Options) error {
			// no probes: the collection is loaded but nothing is attached
			mgr := &manager.Manager{}
			opts.RemoveRlimit = true
			require.NoError(t, mgr.LoadELF(buf))
			// distinct program names, so the second manager's keys can't collide with the first's
			specs, err := mgr.GetProgramSpecs()
			require.NoError(t, err)
			for _, spec := range specs {
				spec.Name = namePrefix + spec.Name
			}
			require.NoError(t, modifier.BeforeInit(mgr, mname, &opts))
			require.NoError(t, mgr.InitWithOptions(nil, opts))
			t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })
			afterInitErr = modifier.AfterInit(mgr, mname, &opts)
			return nil
		})
		require.NoError(t, err)
		return afterInitErr
	}

	require.NoError(t, initManager("first_"))
	registered := probeKeysForModule()
	require.NotZero(t, registered)

	require.ErrorContains(t, initManager("second_"), "already exists")
	require.Equal(t, registered, probeKeysForModule(), "rejected fill registered helper-error keys")
}
