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

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

const probeUID = "net"

//nolint:revive // TODO(NET) Fix revive linter
var ErrorNotSupported = errors.New("fentry tracer is only supported on Fargate")

// LoadTracer loads a new tracer
func LoadTracer(config *config.Config, mgrOpts manager.Options, connCloseEventHandler ddebpf.EventHandler, bpfTelemetry *ebpftelemetry.EBPFTelemetry) (*manager.Manager, func(), error) {
	if !fargate.IsFargateInstance() {
		return nil, nil, ErrorNotSupported
	}

	m := ebpftelemetry.NewManager(&manager.Manager{}, bpfTelemetry)
	err := ddebpf.LoadCOREAsset(netebpf.ModuleFileName("tracer-fentry", config.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.RLimit = mgrOpts.RLimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors

		// Use the config to determine what kernel probes should be enabled
		enabledProbes, err := enabledPrograms(config)
		if err != nil {
			return fmt.Errorf("invalid probe configuration: %v", err)
		}

		initManager(m, connCloseEventHandler, config)

		file, err := os.Stat("/proc/self/ns/pid")

		if err != nil {
			return fmt.Errorf("could not load sysprobe pid: %w", err)
		}

		device := file.Sys().(*syscall.Stat_t).Dev
		inode := file.Sys().(*syscall.Stat_t).Ino

		o.ConstantEditors = append(o.ConstantEditors, manager.ConstantEditor{
			Name:  "systemprobe_device",
			Value: device,
		})
		o.ConstantEditors = append(o.ConstantEditors, manager.ConstantEditor{
			Name:  "systemprobe_ino",
			Value: inode,
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

		return m.InitWithOptions(ar, o)
	})

	if err != nil {
		return nil, nil, err
	}

	return m.Manager, nil, nil
}
