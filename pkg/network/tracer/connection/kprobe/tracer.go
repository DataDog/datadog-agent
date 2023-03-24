// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"fmt"

	"github.com/cilium/ebpf"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const probeUID = "net"

var (
	// The kernel has to be newer than 4.7.0 since we are using bpf_skb_load_bytes (4.5.0+) method to read from the
	// socket filter, and a tracepoint (4.7.0+).
	classificationMinimumKernel = kernel.VersionCode(4, 7, 0)

	tailCalls = []manager.TailCallRoute{
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationQueues,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierQueuesSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationDBs,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierDBsSocketFilter,
				UID:          probeUID,
			},
		},
	}
)

// ClassificationSupported returns true if the current kernel version supports the classification feature.
// The kernel has to be newer than 4.7.0 since we are using bpf_skb_load_bytes (4.5.0+) method to read from the socket
// filter, and a tracepoint (4.7.0+)
func ClassificationSupported(config *config.Config) bool {
	if !config.ProtocolClassificationEnabled {
		return false
	}
	if !config.CollectTCPv4Conns && !config.CollectTCPv6Conns {
		return false
	}
	currentKernelVersion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. classification monitoring disabled.")
		return false
	}

	return currentKernelVersion >= classificationMinimumKernel
}

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
			// do not use offset guessing constants with runtime compilation
			mgrOpts.ConstantEditors = nil
			runtimeTracer = true
			defer buf.Close()
		}
	}

	if buf == nil {
		kv, err := kernel.HostVersion()
		if err != nil {
			return nil, fmt.Errorf("kernel version: %s", err)
		}
		// prebuilt on 5.18+ cannot support UDPv6
		if kv >= kernel.VersionCode(5, 18, 0) {
			config.CollectUDPv6Conns = false
		}

		buf, err = netebpf.ReadBPFModule(config.BPFDir, config.BPFDebug)
		if err != nil {
			return nil, fmt.Errorf("could not read bpf module: %s", err)
		}

		defer buf.Close()
	}

	if err = initManager(m, config, perfHandlerTCP, runtimeTracer); err != nil {
		return nil, fmt.Errorf("could not initialize manager: %w", err)
	}

	telemetryMapKeys := errtelemetry.BuildTelemetryKeys(m)
	mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, telemetryMapKeys...)

	var undefinedProbes []manager.ProbeIdentificationPair

	var closeProtocolClassifierSocketFilterFn func()
	if ClassificationSupported(config) {
		socketFilterProbe, _ := m.GetProbe(manager.ProbeIdentificationPair{
			EBPFFuncName: probes.ProtocolClassifierEntrySocketFilter,
			UID:          probeUID,
		})
		if socketFilterProbe == nil {
			return nil, fmt.Errorf("error retrieving protocol classifier socket filter")
		}

		closeProtocolClassifierSocketFilterFn, err = filter.HeadlessSocketFilter(config, socketFilterProbe)
		if err != nil {
			return nil, fmt.Errorf("error enabling protocol classifier: %s", err)
		}

		undefinedProbes = append(undefinedProbes, tailCalls[0].ProbeIdentificationPair)
		mgrOpts.TailCallRouter = append(mgrOpts.TailCallRouter, tailCalls...)
	} else {
		// Kernels < 4.7.0 do not know about the per-cpu array map used
		// in classification, preventing the program to load even though
		// we won't use it. We change the type to a simple array map to
		// circumvent that.
		for _, mapName := range []string{probes.ProtocolClassificationBufMap, probes.KafkaClientIDBufMap, probes.KafkaTopicNameBufMap} {
			mgrOpts.MapSpecEditors[mapName] = manager.MapSpecEditor{
				Type:       ebpf.Array,
				EditorFlag: manager.EditType,
			}
		}
	}

	if err := errtelemetry.ActivateBPFTelemetry(m, undefinedProbes); err != nil {
		return nil, fmt.Errorf("could not activate ebpf telemetry: %w", err)
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledProbes(config, runtimeTracer)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			mgrOpts.ExcludedFunctions = append(mgrOpts.ExcludedFunctions, p.EBPFFuncName)
		}
	}

	tailCallsIdentifiersSet := make(map[manager.ProbeIdentificationPair]struct{}, len(tailCalls))
	for _, tailCall := range tailCalls {
		tailCallsIdentifiersSet[tailCall.ProbeIdentificationPair] = struct{}{}
	}

	for funcName := range enabledProbes {
		probeIdentifier := manager.ProbeIdentificationPair{
			EBPFFuncName: funcName,
			UID:          probeUID,
		}
		if _, ok := tailCallsIdentifiersSet[probeIdentifier]; ok {
			// tail calls should be enabled (a.k.a. not excluded) but not activated.
			continue
		}
		mgrOpts.ActivatedProbes = append(
			mgrOpts.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: probeIdentifier,
			})
	}

	if err := m.InitWithOptions(buf, mgrOpts); err != nil {
		return nil, fmt.Errorf("failed to init ebpf manager: %v", err)
	}

	return closeProtocolClassifierSocketFilterFn, nil
}
