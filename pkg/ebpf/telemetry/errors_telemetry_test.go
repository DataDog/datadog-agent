// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"os"
	"syscall"
	"testing"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
)

var m1 = &manager.Manager{
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
		{
			Name: "shared_map",
		},
	},
}

var m2 = &manager.Manager{
	Probes: []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "kprobe__do_vfs_ioctl",
			},
		},
	},
}

func skipTestIfEBPFTelemetryNotSupported(t *testing.T) {
	ok, err := EBPFTelemetrySupported()
	require.NoError(t, err)
	if !ok {
		t.Skip("EBPF telemetry is not supported for this kernel version")
	}
}

func startManager(t *testing.T, m *manager.Manager, options manager.Options, name string, buf bytecode.AssetReader) {
	err := m.LoadELF(buf)
	require.NoError(t, err)

	modifier := ErrorsTelemetryModifier{}
	err = modifier.BeforeInit(m, names.NewModuleName(name), &options)
	require.NoError(t, err)
	err = m.InitWithOptions(nil, options)
	require.NoError(t, err)
	err = modifier.AfterInit(m, names.NewModuleName(name), &options)
	require.NoError(t, err)
	m.Start()

	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})
}

func triggerTestAndGetTelemetry(t *testing.T) []prometheus.Metric {
	collector := NewEBPFErrorsCollector()

	err := ddebpf.LoadCOREAsset("error_telemetry.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe__vfs_open",
				},
			},
		}
		startManager(t, m1, opts, "m1", buf)

		return nil
	})
	require.NoError(t, err)

	sharedMap, found, err := m1.GetMap("shared_map")
	require.True(t, found, "'shared_map' not found in manager")
	require.NoError(t, err)

	err = ddebpf.LoadCOREAsset("error_telemetry.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.ActivatedProbes = []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe__do_vfs_ioctl",
				},
			},
		}
		opts.MapEditors = map[string]*ebpf.Map{
			"shared_map": sharedMap,
		}

		startManager(t, m2, opts, "m2", buf)

		return nil
	})
	require.NoError(t, err)

	_, err = os.Open("/proc/self/exe")
	require.NoError(t, err)

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(0), 0xfafadead, uintptr(0)); errno != 0 {
		// Only valid return value is ENOTTY (invalid ioctl for device) because indeed we
		// are not doing any valid ioctl, we just want to trigger the kprobe
		require.Equal(t, syscall.ENOTTY, errno)
	}

	ch := make(chan prometheus.Metric)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()

	//collect metrics from the channel
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	return metrics
}

func TestMapsTelemetry(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)

	mapsTelemetry := triggerTestAndGetTelemetry(t)

	errorMapEntryFound, e2bigErrorFound := false, false
	for _, promMetric := range mapsTelemetry {
		dtoMetric := dto.Metric{}
		require.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		require.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

		for _, label := range dtoMetric.GetLabel() {
			switch label.GetName() {
			case "map_name":
				if label.GetValue() == "error_map" {
					errorMapEntryFound = true
				}
			case "error":
				if label.GetValue() == "E2BIG" {
					e2bigErrorFound = true
				}
			}
		}

		// check error value immediately if map is discovered
		if errorMapEntryFound {
			require.True(t, e2bigErrorFound)
		}
	}

	// ensure test fails if map telemetry not found
	require.True(t, errorMapEntryFound)
}

func TestMapsTelemetrySuppressError(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)

	mapsTelemetry := triggerTestAndGetTelemetry(t)

	suppressMapEntryFound := false
	for _, promMetric := range mapsTelemetry {
		dtoMetric := dto.Metric{}
		require.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		require.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

		for _, label := range dtoMetric.GetLabel() {
			switch label.GetName() {
			case "map_name":
				if label.GetValue() == "suppress_map" {
					suppressMapEntryFound = true
				}
			}
		}

		require.False(t, suppressMapEntryFound)
	}
}

func TestHelpersTelemetry(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)

	helperTelemetry := triggerTestAndGetTelemetry(t)

	probeReadHelperFound, efaultErrorFound := false, false
	for _, promMetric := range helperTelemetry {
		dtoMetric := dto.Metric{}
		require.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		require.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

		for _, label := range dtoMetric.GetLabel() {
			switch label.GetName() {
			case "helper":
				if label.GetValue() == "bpf_probe_read" {
					probeReadHelperFound = true
				}
			case "error":
				if label.GetValue() == "EFAULT" {
					efaultErrorFound = true
				}
			}

			// make sure bpf_probe_read helper has an associated EFAULT error
			if probeReadHelperFound {
				require.True(t, efaultErrorFound)
			}
		}
	}

	// ensure test fails if helper metric not found
	require.True(t, probeReadHelperFound)
}

func testSharedMaps(t *testing.T, mapsTelemetry []prometheus.Metric, errorCount float64, sharedMap, moduleToTest string) bool {
	testComplete := false
	for _, promMetric := range mapsTelemetry {
		sharedMapFound, moduleFound, e2bigErrorFound := false, false, false

		dtoMetric := dto.Metric{}
		require.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		require.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

		for _, label := range dtoMetric.GetLabel() {
			switch label.GetName() {
			case "map_name":
				if label.GetValue() == sharedMap {
					sharedMapFound = true
				}
			case "error":
				if label.GetValue() == "E2BIG" {
					e2bigErrorFound = true
				}
			case "module":
				if label.GetValue() == moduleToTest {
					moduleFound = true
				}
			}
		}

		// check error value immediately if map is discovered
		if sharedMapFound && moduleFound {
			testComplete = true
			require.True(t, e2bigErrorFound)
			require.Equal(t, dtoMetric.GetCounter().GetValue(), errorCount)
			break
		}
	}

	return testComplete
}

func TestSharedMapsTelemetry(t *testing.T) {
	skipTestIfEBPFTelemetryNotSupported(t)

	mapsTelemetry := triggerTestAndGetTelemetry(t)
	require.True(t, testSharedMaps(t, mapsTelemetry, float64(1), "shared_map", "m1"))
	require.True(t, testSharedMaps(t, mapsTelemetry, float64(3), "shared_map", "m2"))
}
