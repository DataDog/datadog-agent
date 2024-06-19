package telemetry

import (
	"math"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/alecthomas/assert/v2"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
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

func triggerTestAndGetMapsTelemetry(t *testing.T) map[string]interface{} {
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

	ebpfTelemetry, ok := collector.T.(*EBPFTelemetry)
	require.True(t, ok)

	mapsTelemetry := ebpfTelemetry.GetMapsTelemetry()
	t.Logf("EBPF Maps telemetry: %v\n", mapsTelemetry)

	return mapsTelemetry

}

func TestGetMapsTelemetry(t *testing.T) {
	mapsTelemetry := triggerTestAndGetMapsTelemetry(t)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	errors, ok := mapsTelemetry["error_map"].(map[string]uint64)
	require.True(t, ok)
	assert.NotZero(t, errors["E2BIG"])
}

func TestMapsTelemetrySuppressError(t *testing.T) {
	mapsTelemetry := triggerTestAndGetMapsTelemetry(t)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	_, ok := mapsTelemetry["suppress_map"].(map[string]uint64)
	require.True(t, !ok)
}
