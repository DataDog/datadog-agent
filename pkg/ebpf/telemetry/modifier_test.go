// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

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
