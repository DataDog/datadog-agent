// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

type Manager struct {
	*manager.Manager
	bpfTelemetry *EBPFTelemetry
}

func NewManager(mgr *manager.Manager, bt *EBPFTelemetry) *Manager {
	return &Manager{
		Manager:      mgr,
		bpfTelemetry: bt,
	}
}

func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts manager.Options) error {
	telemetryMapKeys := BuildTelemetryKeys(m.Manager)
	opts.ConstantEditors = append(opts.ConstantEditors, telemetryMapKeys...)

	initializeMaps(m.bpfTelemetry, &opts)

	if err := m.Manager.InitWithOptions(bytecode, opts); err != nil {
		return err
	}

	if err := m.bpfTelemetry.RegisterEBPFTelemetry(m.Manager); err != nil {
		return err
	}

	return nil
}

func initializeMaps(bpfTelemetry *EBPFTelemetry, opts *manager.Options) {
	if bpfTelemetry == nil {
		return
	}

	if (bpfTelemetry.MapErrMap != nil) || (bpfTelemetry.HelperErrMap != nil) {
		if opts.MapEditors == nil {
			opts.MapEditors = make(map[string]*ebpf.Map)
		}
	}
	if bpfTelemetry.MapErrMap != nil {
		opts.MapEditors[probes.MapErrTelemetryMap] = bpfTelemetry.MapErrMap
	}
	if bpfTelemetry.HelperErrMap != nil {
		opts.MapEditors[probes.HelperErrTelemetryMap] = bpfTelemetry.HelperErrMap
	}

}
