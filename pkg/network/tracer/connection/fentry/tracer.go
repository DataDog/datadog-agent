// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package fentry

import (
	"errors"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

const probeUID = "net"

var ErrorNotSupported = errors.New("fentry tracer is only supported on Fargate")

// LoadTracer loads a new tracer
func LoadTracer(config *config.Config, m *manager.Manager, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (func(), error) {
	if !fargate.IsFargateInstance() {
		return nil, ErrorNotSupported
	}

	filename := "tracer-fentry.o"
	if config.BPFDebug {
		filename = "tracer-fentry-debug.o"
	}

	err := ddebpf.LoadCOREAsset(&config.Config, filename, func(ar bytecode.AssetReader, o manager.Options) error {
		o.RLimit = mgrOpts.RLimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors

		// Use the config to determine what kernel probes should be enabled
		enabledProbes, err := enabledPrograms(config)
		if err != nil {
			return fmt.Errorf("invalid probe configuration: %v", err)
		}

		initManager(m, config, perfHandlerTCP)

		if err := errtelemetry.ActivateBPFTelemetry(m, nil); err != nil {
			return fmt.Errorf("could not activate ebpf telemetry: %w", err)
		}

		telemetryMapKeys := errtelemetry.BuildTelemetryKeys(m)
		o.ConstantEditors = append(o.ConstantEditors, telemetryMapKeys...)

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
		return nil, err
	}

	return nil, nil
}
