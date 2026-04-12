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
	bugs "github.com/DataDog/datadog-agent/pkg/ebpf/kernelbugs"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
)

const probeUID = "net"

// ErrorDisabled is the error that occurs when enable_fentry is false
var ErrorDisabled = errors.New("fentry tracer is disabled")

// LoadTracer loads a new tracer
func LoadTracer(config *config.Config, mgrOpts manager.Options, connCloseEventHandler *perf.EventHandler) (*ddebpf.Manager, func(), error) {
	if !config.EnableFentry {
		return nil, nil, ErrorDisabled
	}

	hasPotentialFentryDeadlock, err := bugs.HasTasksRCUExitLockSymbol()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check HasTasksRCUExitLockSymbol: %w", err)
	}
	if hasPotentialFentryDeadlock {
		return nil, nil, errors.New("unable to load fentry because this kernel version has a potential deadlock (fixed in kernel v6.9+)")
	}

	m := ddebpf.NewManagerWithDefault(&manager.Manager{}, "network", &ebpftelemetry.ErrorsTelemetryModifier{}, connCloseEventHandler)
	var closeProtocolClassifierSocketFilterFn func()
	err = ddebpf.LoadCOREAsset(netebpf.ModuleFileName("tracer-fentry", config.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.RemoveRlimit = mgrOpts.RemoveRlimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors
		var initErr error
		closeProtocolClassifierSocketFilterFn, initErr = initFentryTracer(ar, o, config, m)
		return initErr
	})

	if err != nil {
		return nil, nil, err
	}

	return m, closeProtocolClassifierSocketFilterFn, nil
}

// Use a function so someone doesn't accidentally use mgrOpts from the outer scope in LoadTracer
func initFentryTracer(ar bytecode.AssetReader, o manager.Options, config *config.Config, m *ddebpf.Manager) (func(), error) {
	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledPrograms(config)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	initManager(m)

	file, err := os.Stat("/proc/self/ns/pid")
	if err != nil {
		return nil, fmt.Errorf("could not load sysprobe pid: %w", err)
	}
	pidStat := file.Sys().(*syscall.Stat_t)
	o.ConstantEditors = append(o.ConstantEditors, manager.ConstantEditor{
		Name:  "systemprobe_device",
		Value: pidStat.Dev,
	}, manager.ConstantEditor{
		Name:  "systemprobe_ino",
		Value: pidStat.Ino,
	})

	// Protocol classification setup
	var closeProtocolClassifierSocketFilterFn func()
	classificationSupported := util.ClassificationSupported(config)
	util.AddBoolConst(&o, "protocol_classification_enabled", classificationSupported)

	var tailCallsIdentifiersSet map[manager.ProbeIdentificationPair]struct{}
	if classificationSupported {
		pcTailCalls := protocolClassificationTailCalls()
		tailCallsIdentifiersSet = make(map[manager.ProbeIdentificationPair]struct{}, len(pcTailCalls))
		for _, tailCall := range pcTailCalls {
			tailCallsIdentifiersSet[tailCall.ProbeIdentificationPair] = struct{}{}
		}
		socketFilterProbe, _ := m.GetProbe(manager.ProbeIdentificationPair{
			EBPFFuncName: probes.ProtocolClassifierEntrySocketFilter,
			UID:          probeUID,
		})
		if socketFilterProbe == nil {
			return nil, errors.New("error retrieving protocol classifier socket filter")
		}

		closeProtocolClassifierSocketFilterFn, err = filter.HeadlessSocketFilter(config, socketFilterProbe)
		if err != nil {
			return nil, fmt.Errorf("error enabling protocol classifier: %w", err)
		}

		o.TailCallRouter = append(o.TailCallRouter, pcTailCalls...)
	}

	if config.FailedConnectionsSupported() {
		util.AddBoolConst(&o, "tcp_failed_connections_enabled", true)
	}

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			o.ExcludedFunctions = append(o.ExcludedFunctions, p.EBPFFuncName)
		}
	}
	for funcName := range enabledProbes {
		if _, ok := tailCallsIdentifiersSet[manager.ProbeIdentificationPair{EBPFFuncName: funcName, UID: probeUID}]; ok {
			// tail calls should be enabled (a.k.a. not excluded) but not activated.
			continue
		}
		o.ActivatedProbes = append(
			o.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: funcName,
					UID:          probeUID,
				},
			})
	}

	if err := m.InitWithOptions(ar, &o); err != nil {
		return nil, err
	}
	return closeProtocolClassifierSocketFilterFn, nil
}

func protocolClassificationTailCalls() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationTLSClient,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierTLSClientSocketFilter,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationTLSServer,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierTLSServerSocketFilter,
				UID:          probeUID,
			},
		},
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
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationGRPC,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: probes.ProtocolClassifierGRPCSocketFilter,
				UID:          probeUID,
			},
		},
	}
}
