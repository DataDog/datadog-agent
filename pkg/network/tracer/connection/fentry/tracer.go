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
	"slices"
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
	ssluprobes "github.com/DataDog/datadog-agent/pkg/network/tracer/connection/ssl-uprobes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	var closeFn func()
	err = ddebpf.LoadCOREAsset(netebpf.ModuleFileName("tracer-fentry", config.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.RemoveRlimit = mgrOpts.RemoveRlimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors
		o.DefaultKProbeMaxActive = mgrOpts.DefaultKProbeMaxActive
		o.BypassEnabled = mgrOpts.BypassEnabled
		var initErr error
		closeFn, initErr = initFentryTracer(ar, o, config, m)
		return initErr
	})

	if err != nil {
		return nil, nil, err
	}

	return m, closeFn, nil
}

// protocolClassificationTailCalls returns the tail call routes for protocol classification
func protocolClassificationTailCalls(cfg *config.Config) []manager.TailCallRoute {
	tcs := []manager.TailCallRoute{
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationTLSClient,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolClassifierTLSClient,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationTLSServer,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolClassifierTLSServer,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationQueues,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolClassifierQueues,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationDBs,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolClassifierDBs,
				UID:          probeUID,
			},
		},
		{
			ProgArrayName: probes.ClassificationProgsMap,
			Key:           netebpf.ClassificationGRPC,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: protocolClassifierGRPC,
				UID:          probeUID,
			},
		},
	}
	// Note: unlike kprobe, fentry does NOT use bpf_tail_call into tcp_close_progs.
	// The fexit/tcp_close handler directly calls flush_conn_close_if_full(),
	// so tcpCloseReturn should NOT be added as a tail call route here.
	return tcs
}

// initFentryTracer sets up and initializes the fentry tracer
func initFentryTracer(ar bytecode.AssetReader, o manager.Options, config *config.Config, m *ddebpf.Manager) (func(), error) {
	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledPrograms(config)
	if err != nil {
		return nil, fmt.Errorf("invalid probe configuration: %v", err)
	}

	initManager(m)

	// Add SSL uprobe declarations
	if config.EnableCertCollection {
		funcNameToSSLProbe := func(funcName probes.ProbeFuncName) *manager.Probe {
			return &manager.Probe{
				ProbeIdentificationPair: ssluprobes.IDPairFromFuncName(funcName),
			}
		}
		for _, funcName := range ssluprobes.OpenSSLUProbes {
			m.Probes = append(m.Probes, funcNameToSSLProbe(funcName))
		}
		m.Probes = append(m.Probes, ssluprobes.GetSchedExitProbeSSL())
	}

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
	isClassificationSupported := classificationSupported(config)
	util.AddBoolConst(&o, "protocol_classification_enabled", isClassificationSupported)
	var tailCallsIdentifiersSet map[manager.ProbeIdentificationPair]struct{}

	if isClassificationSupported {
		pcTailCalls := protocolClassificationTailCalls(config)
		tailCallsIdentifiersSet = make(map[manager.ProbeIdentificationPair]struct{}, len(pcTailCalls))
		for _, tailCall := range pcTailCalls {
			tailCallsIdentifiersSet[tailCall.ProbeIdentificationPair] = struct{}{}
		}
		socketFilterProbe, _ := m.GetProbe(manager.ProbeIdentificationPair{
			EBPFFuncName: protocolClassifierEntry,
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

	// Failed connections support
	if config.FailedConnectionsSupported() {
		util.AddBoolConst(&o, "tcp_failed_connections_enabled", true)
	}

	// SSL cert collection: enable the sched_process_exit cleanup probe (uses separate UID)
	if config.EnableCertCollection {
		enabledProbes[probes.RawTracepointSchedProcessExit] = struct{}{}
	}

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			// OpenSSLProbes and sched_process_exit will get used later by the uprobe attacher
			if config.EnableCertCollection && (slices.Contains(ssluprobes.OpenSSLUProbes, p.EBPFFuncName) ||
				p.EBPFFuncName == probes.RawTracepointSchedProcessExit) {
				continue
			}
			o.ExcludedFunctions = append(o.ExcludedFunctions, p.EBPFFuncName)
		}
	}

	for funcName := range enabledProbes {
		pid := manager.ProbeIdentificationPair{
			EBPFFuncName: funcName,
			UID:          probeUID,
		}
		if _, ok := tailCallsIdentifiersSet[pid]; ok {
			// tail calls should be enabled (a.k.a. not excluded) but not activated.
			continue
		}
		o.ActivatedProbes = append(
			o.ActivatedProbes,
			&manager.ProbeSelector{
				ProbeIdentificationPair: pid,
			})
	}

	if err := m.InitWithOptions(ar, &o); err != nil {
		return nil, fmt.Errorf("failed to init ebpf manager: %w", err)
	}

	return closeProtocolClassifierSocketFilterFn, nil
}
