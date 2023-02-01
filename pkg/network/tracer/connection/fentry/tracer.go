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

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/protocol"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	manager "github.com/DataDog/ebpf-manager"
)

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

	var closeFn func()

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

		closeFn, err = protocol.EnableProtocolClassification(config, m, &o)
		if err != nil {
			return fmt.Errorf("failed to enable protocol classification: %w", err)
		}

		if err := errtelemetry.ActivateBPFTelemetry(m, nil); err != nil {
			return fmt.Errorf("could not activate ebpf telemetry: %w", err)
		}

		telemetryMapKeys := errtelemetry.BuildTelemetryKeys(m)
		mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, telemetryMapKeys...)

		// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
		for _, p := range m.Probes {
			if _, enabled := enabledProbes[p.EBPFSection]; !enabled {
				o.ExcludedFunctions = append(o.ExcludedFunctions, p.EBPFFuncName)
			}
		}
		for probeName, funcName := range enabledProbes {
			o.ActivatedProbes = append(
				o.ActivatedProbes,
				&manager.ProbeSelector{
					ProbeIdentificationPair: manager.ProbeIdentificationPair{
						EBPFSection:  probeName,
						EBPFFuncName: funcName,
						UID:          probes.UID,
					},
				})
		}

		return m.InitWithOptions(ar, o)
	})

	return closeFn, err
}
