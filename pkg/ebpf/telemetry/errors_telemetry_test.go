package telemetry

import (
	"math"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"

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

func getMapsTelemetry(e *EBPFTelemetry) map[string]interface{} {
	t := make(map[string]interface{})
	if e.mapErrMap == nil {
		return t
	}

	var val mapErrTelemetry
	for m, k := range e.mapKeys {
		err := e.mapErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", m, k)
			continue
		}
		if count := getErrCount(val.Count[:]); len(count) > 0 {
			t[m] = count
		}
	}
	return t
}

func getHelpersTelemetry(e *EBPFTelemetry) map[string]interface{} {
	helperTelemMap := make(map[string]interface{})
	if e.helperErrMap == nil {
		return helperTelemMap
	}

	var val helperErrTelemetry
	for probeName, k := range e.probeKeys {
		err := e.helperErrMap.Lookup(&k, &val)
		if err != nil {
			log.Debugf("failed to get telemetry for map:key %s:%d\n", probeName, k)
			continue
		}

		t := make(map[string]interface{})
		for indx, helperName := range helperNames {
			base := maxErrno * indx
			if count := getErrCount(val.Count[base : base+maxErrno]); len(count) > 0 {
				t[helperName] = count
			}
		}
		if len(t) > 0 {
			helperTelemMap[probeName] = t
		}
	}
	return helperTelemMap
}

func triggerTestAndGetMapsTelemetry(t *testing.T, getTelemetry func(ebpfTelemetry *EBPFTelemetry) map[string]interface{}) map[string]interface{} {
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

	errTelemetry := getTelemetry(ebpfTelemetry)
	t.Logf("Errors telemetry: %v\n", errTelemetry)

	return errTelemetry

}

func TestMapsTelemetry(t *testing.T) {
	mapsTelemetry := triggerTestAndGetMapsTelemetry(t, getMapsTelemetry)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	errors, ok := mapsTelemetry["error_map"].(map[string]uint64)
	require.True(t, ok)
	assert.NotZero(t, errors["E2BIG"])
}

func TestMapsTelemetrySuppressError(t *testing.T) {
	mapsTelemetry := triggerTestAndGetMapsTelemetry(t, getMapsTelemetry)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	_, ok := mapsTelemetry["suppress_map"].(map[string]uint64)
	require.True(t, !ok)
}

func TestHelpersTelemetry(t *testing.T) {
	helpersTelemetry := triggerTestAndGetMapsTelemetry(t, getHelpersTelemetry)
	t.Cleanup(func() {
		m.Stop(manager.CleanAll)
	})

	openErrors, ok := helpersTelemetry["kprobe__vfs_open"].(map[string]interface{})
	require.True(t, ok)

	probeReadErrors, ok := openErrors["bpf_probe_read"].(map[string]uint64)
	require.True(t, ok)

	badAddressCnt, ok := probeReadErrors["EFAULT"]
	require.True(t, ok)
	assert.NotZero(t, badAddressCnt)
}
