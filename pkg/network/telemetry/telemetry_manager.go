package telemetry

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

type ManagerWithTelemetry struct {
	*manager.Manager
	bpfTelemetry *EBPFTelemetry
}

func NewManagerWithTelemetry(mgr *manager.Manager, bt *EBPFTelemetry) *ManagerWithTelemetry {
	return &ManagerWithTelemetry{
		Manager:      mgr,
		bpfTelemetry: bt,
	}
}

func (m *ManagerWithTelemetry) InitManagerWithTelemetry(bytecode io.ReaderAt, opts manager.Options) error {
	telemetryMapKeys := BuildTelemetryKeys(m.Manager)
	opts.ConstantEditors = append(opts.ConstantEditors, telemetryMapKeys...)

	if (m.bpfTelemetry.MapErrMap != nil) || (m.bpfTelemetry.HelperErrMap != nil) {
		if opts.MapEditors != nil {
			opts.MapEditors = make(map[string]*ebpf.Map)
		}
	}
	if m.bpfTelemetry.MapErrMap != nil {
		opts.MapEditors[string(probes.MapErrTelemetryMap)] = m.bpfTelemetry.MapErrMap
	}
	if m.bpfTelemetry.HelperErrMap != nil {
		opts.MapEditors[string(probes.HelperErrTelemetryMap)] = m.bpfTelemetry.HelperErrMap
	}

	if err := m.InitWithOptions(bytecode, opts); err != nil {
		return err
	}

	if err := m.bpfTelemetry.RegisterEBPFTelemetry(m.Manager); err != nil {
		return err
	}

	return nil
}
