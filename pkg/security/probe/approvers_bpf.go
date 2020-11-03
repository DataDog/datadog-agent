// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

type activeApprover = activeKFilter
type activeApprovers = activeKFilters

func approveBasename(probe *Probe, tableName string, basename string) (activeApprover, error) {
	return &mapEntry{
		tableName: tableName,
		key:       basename,
		tableKey:  ebpf.NewStringMapItem(basename, BasenameFilterSize),
		value:     ebpf.ZeroUint8MapItem,
	}, nil
}

func approveBasenames(probe *Probe, tableName string, basenames ...string) (approvers []activeApprover, _ error) {
	for _, basename := range basenames {
		activeApprover, err := approveBasename(probe, tableName, basename)
		if err != nil {
			return nil, err
		}
		approvers = append(approvers, activeApprover)
	}
	return approvers, nil
}

func setFlagsFilter(probe *Probe, tableName string, flags ...int) (activeApprover, error) {
	var flagsItem ebpf.Uint32MapItem

	for _, flag := range flags {
		flagsItem |= ebpf.Uint32MapItem(flag)
	}

	if flagsItem != 0 {
		return &mapEntry{
			tableName: tableName,
			tableKey:  ebpf.ZeroUint32MapItem,
			value:     flagsItem,
		}, nil
	}

	return nil, nil
}

func approveFlags(probe *Probe, tableName string, flags ...int) (activeApprover, error) {
	return setFlagsFilter(probe, tableName, flags...)
}

func approveProcessFilename(probe *Probe, tableName string, filename string) (activeApprover, error) {
	fileinfo, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	stat, _ := fileinfo.Sys().(*syscall.Stat_t)
	key := ebpf.Uint64MapItem(uint64(stat.Ino))

	return &mapEntry{
		tableName: tableName,
		tableKey:  key,
		key:       stat.Ino,
		value:     ebpf.ZeroUint8MapItem,
	}, nil
}

func approveProcessFilenames(probe *Probe, tableName string, filenames ...string) (approvers []activeApprover, err error) {
	for _, filename := range filenames {
		approver, err := approveProcessFilename(probe, tableName, filename)
		if err != nil {
			return approvers, err
		}
		approvers = append(approvers, approver)
	}
	return
}
