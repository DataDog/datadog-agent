// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
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
		return &arrayEntry{
			tableName: tableName,
			index:     uint32(0),
			value:     flagsItem,
			zeroValue: ebpf.ZeroUint32MapItem,
		}, nil
	}

	return nil, nil
}

func approveFlags(probe *Probe, tableName string, flags ...int) (activeApprover, error) {
	return setFlagsFilter(probe, tableName, flags...)
}
