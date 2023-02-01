// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"fmt"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/protocol"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
)

// LoadTracer loads the prebuilt or runtime compiled tracer, depending on config
func LoadTracer(config *config.Config, m *manager.Manager, mgrOpts manager.Options, perfHandlerTCP *ddebpf.PerfHandler) (func(), error) {
	kprobeAttachMethod := manager.AttachKprobeWithPerfEventOpen
	if config.AttachKprobesWithKprobeEventsABI {
		kprobeAttachMethod = manager.AttachKprobeWithKprobeEvents
	}

	mgrOpts.DefaultKprobeAttachMethod = kprobeAttachMethod

	runtimeTracer := false
	var buf bytecode.AssetReader
	var err error
	if config.EnableRuntimeCompiler {
		buf, err = getRuntimeCompiledTracer(config)
		if err != nil {
			if !config.AllowPrecompiledFallback {
				return nil, fmt.Errorf("error compiling network tracer: %s", err)
			}
			log.Warnf("error compiling network tracer, falling back to pre-compiled: %s", err)
		} else {
			runtimeTracer = true
			defer buf.Close()
		}
	}

	if buf == nil {
		buf, err = netebpf.ReadBPFModule(config.BPFDir, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}

		defer buf.Close()
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledProbes(config, runtimeTracer)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	initManager(m, config, perfHandlerTCP, runtimeTracer)

	closeFn, undefinedProbes, err := protocol.EnableProtocolClassification(config, m, &mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to enable protocol classification: %w", err)
	}

	telemetryMapKeys := errtelemetry.BuildTelemetryKeys(m)
	mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, telemetryMapKeys...)
	if err := errtelemetry.ActivateBPFTelemetry(m, undefinedProbes); err != nil {
		return nil, fmt.Errorf("could not activate ebpf telemetry: %w", err)
	}

	probes.ExcludeAndActivateProbes(m, &mgrOpts, enabledProbes)

	if err := m.InitWithOptions(buf, mgrOpts); err != nil {
		return nil, fmt.Errorf("failed to init ebpf manager: %v", err)
	}

	return closeFn, nil
}
