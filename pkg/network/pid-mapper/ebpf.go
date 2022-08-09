// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package mapper

import (
	"fmt"
	"math"
	"unsafe"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

const (
	tgidPidToFd    = "tgidpid_to_fd"
	symbolTableMap = "symbol_table"
)

var kernelSyms = []string{"socket_file_ops", "tcp_prot", "inet_stream_ops"}
var kernelSymIds = map[string]int{"socket_file_ops": 1, "tcp_prot": 2, "inet_stream_ops": 3}

type ebpfProgram struct {
	*manager.Manager
	cfg               *config.Config
	bytecode          bytecode.AssetReader
	initializorProbes map[string]struct{}
	initializorMaps   map[string]struct{}
}

func newEBPFProgram(c *config.Config) (*ebpfProgram, error) {
	var bc bytecode.AssetReader
	var err error

	if !c.EnableRuntimeCompiler {
		return nil, fmt.Errorf("error compiling pid mapper: %s\n", err)
	}

	bc, err = getRuntimeCompiledPidMapper(c)
	if err != nil {
		return nil, fmt.Errorf("error compiling pid mapper: %s\n", err)
	}

	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: tgidPidToFd},
			{Name: symbolTableMap},
		},
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.DoSysOpen), EBPFFuncName: "kprobe__do_sys_open"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.GetPidTaskReturn), EBPFFuncName: "kretprobe__get_pid_task"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SecuritySkAlloc), EBPFFuncName: "kprobe__security_sk_alloc"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SecuritySkFree), EBPFFuncName: "kprobe__security_sk_free"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SecuritySkClone), EBPFFuncName: "kprobe__security_sk_clone"}},
		},
	}

	return &ebpfProgram{
		Manager:           mgr,
		bytecode:          bc,
		cfg:               c,
		initializorMaps:   map[string]struct{}{tgidPidToFd: struct{}{}, symbolTableMap: struct{}{}},
		initializorProbes: map[string]struct{}{string(probes.DoSysOpen): struct{}{}, string(probes.GetPidTaskReturn): struct{}{}},
	}, nil
}

func (e *ebpfProgram) Init(sockToPidMap *ebpf.Map) error {
	defer e.bytecode.Close()
	return e.InitWithOptions(e.bytecode, manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapEditors: map[string]*ebpf.Map{
			string(probes.SockToPidMap): sockToPidMap,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.DoSysOpen),
					EBPFFuncName: "kprobe__do_sys_open",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.GetPidTaskReturn),
					EBPFFuncName: "kretprobe__get_pid_task",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.SecuritySkAlloc),
					EBPFFuncName: "kprobe__security_sk_alloc",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.SecuritySkFree),
					EBPFFuncName: "kprobe__security_sk_free",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.SecuritySkClone),
					EBPFFuncName: "kprobe__security_sk_clone",
				},
			},
		},
	})
}

func initializeSymbolTableMap(symbolTableMap *ebpf.Map) error {
	addrs, err := ddebpf.GetSymbolsAddresses("/proc/kallsyms", kernelSyms)
	if err != nil {
		return err
	}

	for sym, addr := range addrs {
		id := kernelSymIds[sym]
		_ = symbolTableMap.Put(unsafe.Pointer(&addr), unsafe.Pointer(&id))
	}

	return nil
}

func (e *ebpfProgram) Start() error {
	symTabMap, _, _ := e.GetMap(symbolTableMap)
	err := initializeSymbolTableMap(symTabMap)
	if err != nil {
		return fmt.Errorf("Error starting pid mapper: %w", err)
	}

	err = e.Manager.Start()
	if err != nil {
		return err
	}

	err = WalkProcFds()
	if err != nil {
		return fmt.Errorf("Error triggering sock->pid mapping: %w", err)
	}

	for _, probe := range e.Probes {
		if _, ok := e.initializorProbes[probe.EBPFSection]; ok {
			if stopErr := probe.Stop(); stopErr != nil {
				return fmt.Errorf("Could not stop initializor probe: %w", err)
			}
		}
	}

	for _, m := range e.Maps {
		if _, ok := e.initializorMaps[m.Name]; ok {
			if stopErr := m.Close(manager.CleanInternal); stopErr != nil {
				return fmt.Errorf("Could not close initializor map: %w", err)
			}
		}
	}

	return nil
}
