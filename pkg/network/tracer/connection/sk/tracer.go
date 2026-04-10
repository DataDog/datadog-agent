// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package sk

import (
	"errors"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ddfeatures "github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/ebpf/perf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

const probeUID = "net"

// ErrorDisabled is the error that occurs when enable_fentry is false
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
	if features.HaveProgramHelper(ebpf.Tracing, asm.FnSkStorageGet) != nil {
		return false
	}
	if ok, err := ddfeatures.HasIteratorType("task_file"); err != nil || !ok {
		return false
	}
	if !ddfeatures.SupportsFentry("tcp_connect") {
		return false
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
