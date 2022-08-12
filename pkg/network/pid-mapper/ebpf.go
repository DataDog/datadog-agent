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
	"os"
	"unsafe"

	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

const (
	savePid        = "save_pid"
	symbolTableMap = "symbol_table"
	inodePidMap    = "inode_pid_map"
)

var kernelSyms = []string{"sockfs_inode_ops", "tcp_prot", "inet_stream_ops"}
var kernelSymIds = map[string]int{"sockfs_inode_ops": 1, "tcp_prot": 2, "inet_stream_ops": 3}

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
			{Name: savePid},
			{Name: symbolTableMap},
			{Name: inodePidMap},
		},
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.UserPathAtEmpty), EBPFFuncName: "kprobe__user_path_at_empty"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.DPath), EBPFFuncName: "kprobe__d_path"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SecuritySkAlloc), EBPFFuncName: "kprobe__security_sk_alloc"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SecuritySkFree), EBPFFuncName: "kprobe__security_sk_free"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SecuritySkClone), EBPFFuncName: "kprobe__security_sk_clone"}},
		},
	}

	return &ebpfProgram{
		Manager:           mgr,
		bytecode:          bc,
		cfg:               c,
		initializorMaps:   map[string]struct{}{savePid: struct{}{}, symbolTableMap: struct{}{}},
		initializorProbes: map[string]struct{}{string(probes.UserPathAtEmpty): struct{}{}, string(probes.DPath): struct{}{}},
	}, nil
}

func (e *ebpfProgram) Init(cfg *config.Config, sockToPidMap *ebpf.Map) error {
	defer e.bytecode.Close()
	return e.InitWithOptions(e.bytecode, manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			inodePidMap: {Type: ebpf.Hash, MaxEntries: uint32(cfg.MaxTrackedConnections), EditorFlag: manager.EditMaxEntries},
		},
		MapEditors: map[string]*ebpf.Map{
			string(probes.SockToPidMap): sockToPidMap,
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.UserPathAtEmpty),
					EBPFFuncName: "kprobe__user_path_at_empty",
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  string(probes.DPath),
					EBPFFuncName: "kprobe__d_path",
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

func (e *ebpfProgram) Start() (func() error, func() error, error) {
	symTabMap, _, _ := e.GetMap(symbolTableMap)
	err := initializeSymbolTableMap(symTabMap)
	if err != nil {
		return nil, nil, fmt.Errorf("error starting pid mapper: %w", err)
	}

	err = e.Manager.Start()
	if err != nil {
		return nil, nil, err
	}

	initializor := func() error {
		err := WalkProcFds(func(path string) error {
			_, err := os.Readlink(path)
			return err
		})
		if err != nil {
			return fmt.Errorf("error triggering sock->pid mapping: %w", err)
		}

		return nil
	}

	initializorDone := func() error {
		for _, probe := range e.Probes {
			if _, ok := e.initializorProbes[probe.EBPFSection]; ok {
				if stopErr := probe.Stop(); stopErr != nil {
					return fmt.Errorf("could not stop initializor probe: %w", err)
				}
			}
		}

		for _, m := range e.Maps {
			if _, ok := e.initializorMaps[m.Name]; ok {
				if stopErr := m.Close(manager.CleanInternal); stopErr != nil {
					return fmt.Errorf("could not close initializor map: %w", err)
				}
			}
		}

		return nil
	}

	return initializor, initializorDone, nil
}
