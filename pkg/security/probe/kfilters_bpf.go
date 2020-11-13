// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"github.com/pkg/errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type pidDiscarder struct {
	eventType EventType
	pid       uint32
	padding   uint32
}

type pidDiscarderParameters struct {
	timestamp uint64
}

func discardPID(probe *Probe, eventType EventType, pid uint32) (bool, error) {
	key := pidDiscarder{
		eventType: eventType,
		pid:       pid,
	}

	table := probe.Map("pid_discarders")
	if err := table.Put(&key, &pidDiscarderParameters{}); err != nil {
		return false, err
	}

	return true, nil
}

func discardPIDWithTimeout(probe *Probe, eventType EventType, pid uint32, timeout time.Duration) (bool, error) {
	key := pidDiscarder{
		eventType: eventType,
		pid:       pid,
	}
	params := pidDiscarderParameters{
		timestamp: uint64(probe.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now().Add(timeout))),
	}

	table := probe.Map("pid_discarders")
	if err := table.Put(&key, &params); err != nil {
		return false, err
	}

	return true, nil
}

type inodeDiscarder struct {
	eventType EventType
	pathKey   PathKey
}

func removeDiscarderInode(probe *Probe, mountID uint32, inode uint64) {
	key := inodeDiscarder{
		pathKey: PathKey{
			MountID: mountID,
			Inode:   inode,
		},
	}

	table := probe.Map("inode_discarders")
	for eventType := UnknownEventType + 1; eventType != maxEventType; eventType++ {
		key.eventType = eventType
		table.Delete(&key)
	}
}

func discardInode(probe *Probe, eventType EventType, mountID uint32, inode uint64) (bool, error) {
	key := inodeDiscarder{
		eventType: eventType,
		pathKey: PathKey{
			MountID: mountID,
			Inode:   inode,
		},
	}

	table := probe.Map("inode_discarders")
	if err := table.Put(&key, ebpf.ZeroUint8MapItem); err != nil {
		return false, err
	}

	return true, nil
}

func discardParentInode(probe *Probe, rs *rules.RuleSet, eventType EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32) (bool, error) {
	isDiscarder, err := isParentPathDiscarder(rs, eventType, field, filename)
	if !isDiscarder {
		return false, err
	}

	parentMountID, parentInode, err := probe.resolvers.DentryResolver.GetParent(mountID, inode, pathID)
	if err != nil {
		return false, err
	}

	return discardInode(probe, eventType, parentMountID, parentInode)
}

func approveBasename(probe *Probe, tableName string, basename string) error {
	key := ebpf.NewStringMapItem(basename, BasenameFilterSize)

	table := probe.Map(tableName)
	if table == nil {
		return errors.Errorf("map %s not found", tableName)
	}
	if err := table.Put(key, ebpf.ZeroUint8MapItem); err != nil {
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
	var flagsItem ebpf.Uint32MapItem

	for _, flag := range flags {
		flagsItem |= ebpf.Uint32MapItem(flag)
	}

	if flagsItem != 0 {
		table := probe.Map(tableName)
		if table == nil {
			return errors.Errorf("map %s not found", tableName)
		}
		if err := table.Put(ebpf.ZeroUint32MapItem, flagsItem); err != nil {
			return err
		}
	}

	return nil
}

func approveFlags(probe *Probe, tableName string, flags ...int) error {
	return setFlagsFilter(probe, tableName, flags...)
}
