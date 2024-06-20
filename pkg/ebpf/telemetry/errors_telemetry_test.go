package telemetry

import (
	"math"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bpfPrograms struct {
	KprobeVfsOpen *ebpf.Program `ebpf:"kprobe__vfs_open"`
}

var m = &manager.Manager{
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

func triggerTestAndGetTelemetry(t *testing.T) []prometheus.Metric {
	bpfDir := os.Getenv("DD_SYSTEM_PROBE_BPF_DIR")
	require.True(t, bpfDir != "")

	buf, err := bytecode.GetReader(bpfDir, "error_telemetry.o")
	require.NoError(t, err)
	t.Cleanup(func() { _ = buf.Close })

	collector := NewEBPFErrorsCollector().(*EBPFErrorsCollector)

	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe__vfs_open",
				},
			},
		},
	}

	modifier := ErrorsTelemetryModifier{}
	err = modifier.BeforeInit(m, &options)
	require.NoError(t, err)
	err = m.InitWithOptions(buf, options)
	require.NoError(t, err)
	err = modifier.AfterInit(m, &options)
	require.NoError(t, err)
	m.Start()

	_, err = os.Open("/proc/self/exe")
	require.NoError(t, err)

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
	mapsTelemetry := triggerTestAndGetTelemetry(t)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	errorMapEntryFound, e2bigErrorFound := false, false
	for _, promMetric := range mapsTelemetry {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

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
	mapsTelemetry := triggerTestAndGetTelemetry(t)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	suppressMapEntryFound := false
	for _, promMetric := range mapsTelemetry {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

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
	helperTelemetry := triggerTestAndGetTelemetry(t)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	probeReadHelperFound, efaultErrorFound := false, false
	for _, promMetric := range helperTelemetry {
		dtoMetric := dto.Metric{}
		assert.NoError(t, promMetric.Write(&dtoMetric), "Failed to parse metric %v", promMetric.Desc())
		assert.NotNilf(t, dtoMetric.GetCounter(), "expected metric %v to be of a counter type", promMetric.Desc())

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
