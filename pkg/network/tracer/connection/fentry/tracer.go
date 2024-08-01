// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package fentry

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

const probeUID = "net"

//nolint:revive // TODO(NET) Fix revive linter
var ErrorNotSupported = errors.New("fentry tracer is only supported on Fargate")

// LoadTracer loads a new tracer
func LoadTracer(config *config.Config, mgrOpts manager.Options, connCloseEventHandler *perf.EventHandler) (*manager.Manager, func(), error) {
	if !fargate.IsFargateInstance() {
		return nil, nil, ErrorNotSupported
	}

	m := ddebpf.NewManagerWithDefault(&manager.Manager{}, &ebpftelemetry.ErrorsTelemetryModifier{})
	err := ddebpf.LoadCOREAsset(netebpf.ModuleFileName("tracer-fentry", config.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.RLimit = mgrOpts.RLimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors
		return initFentryTracer(ar, o, config, m, connCloseEventHandler)
	})

	if err != nil {
		return nil, nil, err
	}

	return m.Manager, nil, nil
}

// Use a function so someone doesn't accidentally use mgrOpts from the outer scope in LoadTracer
func initFentryTracer(ar bytecode.AssetReader, o manager.Options, config *config.Config, m *ddebpf.Manager, connCloseEventHandler *perf.EventHandler) error {
	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledPrograms(config)
	if err != nil {
		return fmt.Errorf("invalid probe configuration: %v", err)
	}

	initManager(m)

	file, err := os.Stat("/proc/self/ns/pid")
	if err != nil {
		return fmt.Errorf("could not load sysprobe pid: %w", err)
	}
	pidStat := file.Sys().(*syscall.Stat_t)
	o.ConstantEditors = append(o.ConstantEditors, manager.ConstantEditor{
		Name:  "systemprobe_device",
		Value: pidStat.Dev,
	}, manager.ConstantEditor{
		Name:  "systemprobe_ino",
		Value: pidStat.Ino,
	})

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			o.ExcludedFunctions = append(o.ExcludedFunctions, p.EBPFFuncName)
		}
	}
	for funcName := range enabledProbes {
		o.ActivatedProbes = append(
			o.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: funcName,
					UID:          probeUID,
				},
			})
	}

	if err := m.LoadELF(ar); err != nil {
		return fmt.Errorf("failed to load ELF with ebpf manager: %w", err)
	}
	if err := connCloseEventHandler.Init(m.Manager, &o); err != nil {
		return fmt.Errorf("error initializing closed connections event handler: %w", err)
	}
	util.AddBoolConst(&o, "ringbuffers_enabled", connCloseEventHandler.MapType() == ebpf.RingBuf)
	return m.InitWithOptions(nil, &o)
}
