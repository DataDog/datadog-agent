package telemetry

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
)

type bpfPrograms struct {
	KprobeVfsOpen *ebpf.Program `ebpf:"kprobe__vfs_open"`
}

func initTelemetryCollector(spec *ebpf.CollectionSpec) {
	bpfTelemetry := newEBPFTelemetry()
	require.NotNil(t, bpfTelemetry)

	patchEBPFTelemetry(spec.Programs, true, bpfTelemetry)
}

func TestGetMapsTelemetry(t *testing.T) {
	bpfDir := os.Getenv("DD_SYSTEM_PROBE_BPF_DIR")
	require.True(t, bpfDir != "")
	buf, err := bytecode.GetReader(bpfDir, "error_telemetry.o")
	require.NoError(t, err)
	t.Cleanup(func() { _ = buf.Close })

	spec, err := ebpf.LoadCollectionSpecFromReader(buf)
	require.NoError(t, err)

	initTelemetryCollector(spec)

	var obj bpfPrograms
	err = spec.LoadAndAssign(&obj, nil)
	require.NoError(t, err)

}
