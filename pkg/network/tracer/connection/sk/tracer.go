// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package sk

import (
	"errors"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ddfeatures "github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const probeUID = "net"

// ErrorDisabled is the error that occurs when enable_sk_tracer is false
var ErrorDisabled = errors.New("SK tracer is disabled")

// LoadTracer loads a new tracer
func LoadTracer(config *config.Config, mgrOpts manager.Options, connCloseEventHandler *perf.EventHandler) (*ddebpf.Manager, func(), error) {
	if !config.EnableSKTracer {
		return nil, nil, ErrorDisabled
	}
	if !KernelSupported() {
		return nil, nil, errors.New("sk tracer unsupported on this platform")
	}

	m := ddebpf.NewManagerWithDefault(&manager.Manager{}, "network", &ebpftelemetry.ErrorsTelemetryModifier{}, connCloseEventHandler)
	err := ddebpf.LoadCOREAsset(netebpf.ModuleFileName("sk_tracer", config.BPFDebug), func(ar bytecode.AssetReader, o manager.Options) error {
		o.RemoveRlimit = mgrOpts.RemoveRlimit
		o.MapSpecEditors = mgrOpts.MapSpecEditors
		o.ConstantEditors = mgrOpts.ConstantEditors
		return initSKTracer(ar, o, config, m)
	})

	if err != nil {
		return nil, nil, err
	}
	return m, nil, nil
}

// KernelSupported returns whether the kernel supports all the eBPF features needed for the SK tracer
// bpf_sk_storage_get in BPF_PROG_TYPE_TRACING - 5.11
// bpf_iter__task_file - 5.8
// fentry - 5.5
// BPF_MAP_TYPE_SK_STORAGE - 5.2
// BPF_PROG_TYPE_CGROUP_SOCK - ctx fields - src_ip4/6, dst_ip4/6, src/dst_port - 5.1
// BPF_PROG_TYPE_SOCK_OPS - BPF_SOCK_OPS_STATE_CB - 4.16
var KernelSupported = funcs.MemoizeNoError(func() bool {
	if ddfeatures.HaveHelperInFentry(asm.FnSkStorageGet) != nil {
		return false
	}
	if ddfeatures.HaveIteratorType("task_file") != nil {
		return false
	}
	for _, sym := range fentrySymbols {
		if ddfeatures.SupportsFentry(sym) != nil {
			return false
		}
	}

	kv, err := kernel.HostVersion()
	if err != nil {
		return false
	}
	if util.HasTCPSendPage(kv) {
		fexitSymbols = append(fexitSymbols, "udp_sendpage")
	}
	for _, sym := range fexitSymbols {
		if ddfeatures.SupportsFexit(sym) != nil {
			return false
		}
	}
	if features.HaveMapType(ebpf.SkStorage) != nil {
		return false
	}
	if features.HaveProgramType(ebpf.CGroupSock) != nil {
		return false
	}
	if features.HaveProgramType(ebpf.SockOps) != nil {
		return false
	}
	if features.HaveMapType(ebpf.RingBuf) != nil {
		return false
	}
	return true
})

func initSKTracer(ar bytecode.AssetReader, o manager.Options, config *config.Config, m *ddebpf.Manager) error {
	if config.FailedConnectionsSupported() {
		util.AddBoolConst(&o, "tcp_failed_connections_enabled", true)
	}

	// Use the config to determine what kernel probes should be enabled
	enabledProbes, err := enabledPrograms(config)
	if err != nil {
		return fmt.Errorf("invalid probe configuration: %v", err)
	}

	initManager(m)

	// exclude all non-enabled probes to ensure we don't run into problems with unsupported probe types
	for _, p := range m.Probes {
		if _, enabled := enabledProbes[p.EBPFFuncName]; !enabled {
			o.ExcludedFunctions = append(o.ExcludedFunctions, p.EBPFFuncName)
		}
	}
	// not a manager probe, see AttachOrphanSocketsIterator
	if !OrphanSocketsIteratorEnabled(config) {
		o.ExcludedFunctions = append(o.ExcludedFunctions, orphanSocketsIterator)
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

	return m.InitWithOptions(ar, &o)
}

// orphanSocketsIterator is the iterator program listing the TCP sockets that have been closed by
// the application, but not yet torn down by the kernel. It isn't a manager probe, because a map
// iterator must be attached to its map, which the manager can't do.
const orphanSocketsIterator = "bpf_iter__sk_storage_map_tcp_orphans"

// haveSkStorageMapIterator returns whether the kernel supports iterators over BPF_MAP_TYPE_SK_STORAGE
// maps (5.9). ddfeatures.HaveIteratorType can't check it: a map iterator can only be attached to a map.
var haveSkStorageMapIterator = funcs.MemoizeNoError(func() bool {
	prog, err := ebpf.NewProgramWithOptions(&ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: ebpf.AttachTraceIter,
		AttachTo:   "bpf_sk_storage_map",
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
		License: "GPL",
	}, ebpf.ProgramOptions{LogDisabled: true})
	if err != nil {
		log.Warnf("TCP sockets closed by the application won't be reported as active until the kernel destroys them: sk_storage map iterators aren't supported: %s", err)
		return false
	}
	_ = prog.Close()
	return true
})

// OrphanSocketsIteratorEnabled returns whether the iterator listing orphaned TCP sockets is used
func OrphanSocketsIteratorEnabled(config *config.Config) bool {
	return (config.CollectTCPv4Conns || config.CollectTCPv6Conns) && haveSkStorageMapIterator()
}

// AttachOrphanSocketsIterator attaches the iterator listing TCP sockets that have no file anymore,
// but whose close event hasn't been sent yet. They are still active connections.
func AttachOrphanSocketsIterator(m *manager.Manager) (*link.Iter, error) {
	progs, ok, err := m.GetProgram(manager.ProbeIdentificationPair{EBPFFuncName: orphanSocketsIterator})
	if err != nil {
		return nil, fmt.Errorf("error getting program %s: %w", orphanSocketsIterator, err)
	}
	if !ok || len(progs) == 0 || progs[0] == nil {
		return nil, fmt.Errorf("program %s not found", orphanSocketsIterator)
	}
	tcpStats, ok, err := m.GetMap("sk_tcp_stats")
	if err != nil {
		return nil, fmt.Errorf("error getting map sk_tcp_stats: %w", err)
	}
	if !ok {
		return nil, errors.New("map sk_tcp_stats not found")
	}
	it, err := link.AttachIter(link.IterOptions{Program: progs[0], Map: tcpStats})
	if err != nil {
		return nil, fmt.Errorf("error attaching %s: %w", orphanSocketsIterator, err)
	}
	return it, nil
}
