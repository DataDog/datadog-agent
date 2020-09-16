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
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

func discardInode(probe *Probe, mountID uint32, inode uint64, tableName string) (bool, error) {
	key := pathKey{mountID: mountID, inode: inode}

	table := probe.Table(tableName)
	if err := table.Set(&key, ebpf.ZeroUint8TableItem); err != nil {
		return false, err
	}

	return true, nil
}

func discardParentInode(probe *Probe, rs *rules.RuleSet, eventType eval.EventType, filename string, mountID uint32, inode uint64, tableName string) (bool, error) {
	isDiscarder, err := isParentPathDiscarder(rs, eventType, filename)
	if !isDiscarder {
		return false, err
	}

	parentMountID, parentInode, err := probe.resolvers.DentryResolver.GetParent(mountID, inode)
	if err != nil {
		return false, err
	}

	return discardInode(probe, parentMountID, parentInode, tableName)
}

func approveBasename(probe *Probe, tableName string, basename string) error {
	key := ebpf.NewStringTableItem(basename, BasenameFilterSize)

	table := probe.Table(tableName)
	if err := table.Set(key, ebpf.ZeroUint8TableItem); err != nil {
		return err
	}

	return nil
}

func approveBasenames(probe *Probe, tableName string, basenames ...string) error {
	for _, basename := range basenames {
		if err := approveBasename(probe, tableName, basename); err != nil {
			return err
		}
	}
	return nil
}

func setFlagsFilter(probe *Probe, tableName string, flags ...int) error {
	var flagsItem ebpf.Uint32TableItem

	for _, flag := range flags {
		flagsItem |= ebpf.Uint32TableItem(flag)
	}

	if flagsItem != 0 {
		table := probe.Table(tableName)
		if err := table.Set(ebpf.ZeroUint32TableItem, flagsItem); err != nil {
			return err
		}
	}

	return nil
}

func approveFlags(probe *Probe, tableName string, flags ...int) error {
	return setFlagsFilter(probe, tableName, flags...)
}

func discardFlags(probe *Probe, tableName string, flags ...int) error {
	return setFlagsFilter(probe, tableName, flags...)
}

func approveProcessFilename(probe *Probe, tableName string, filename string) error {
	fileinfo, err := os.Stat(filename)
	if err != nil {
		return err
	}
	stat, _ := fileinfo.Sys().(*syscall.Stat_t)
	key := ebpf.Uint64TableItem(uint64(stat.Ino))

	table := probe.Table(tableName)
	if err := table.Set(key, ebpf.ZeroUint8TableItem); err != nil {
		return err
	}
	return nil
}

func approveProcessFilenames(probe *Probe, tableName string, filenames ...string) error {
	for _, filename := range filenames {
		if err := approveProcessFilename(probe, tableName, filename); err != nil {
			return err
		}
	}

	return nil
}
