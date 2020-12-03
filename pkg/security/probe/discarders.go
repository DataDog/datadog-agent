// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"time"

	libebpf "github.com/DataDog/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type pidDiscarderParameters struct {
	EventType  EventType
	Timestamps [maxEventRoundedUp]uint64
}

func (p *Probe) discardPID(eventType EventType, pid uint32) error {
	var params pidDiscarderParameters

	updateFlags := libebpf.UpdateExist
	if err := p.pidDiscarders.Lookup(pid, &params); err != nil {
		updateFlags = libebpf.UpdateAny
	}

	params.EventType |= 1 << (eventType - 1)
	return p.pidDiscarders.Update(pid, &params, updateFlags)
}

func (p *Probe) discardPIDWithTimeout(eventType EventType, pid uint32, timeout time.Duration) error {
	var params pidDiscarderParameters

	updateFlags := libebpf.UpdateExist
	if err := p.pidDiscarders.Lookup(pid, &params); err != nil {
		updateFlags = libebpf.UpdateAny
	}

	params.EventType |= 1 << (eventType - 1)
	params.Timestamps[eventType] = uint64(p.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now().Add(timeout)))

	return p.pidDiscarders.Update(pid, &params, updateFlags)
}

type inodeDiscarder struct {
	PathKey PathKey
}

type inodeDiscarderParameters struct {
	EventType EventType
}

func (p *Probe) removeDiscarderInode(mountID uint32, inode uint64) {
	key := inodeDiscarder{
		PathKey: PathKey{
			MountID: mountID,
			Inode:   inode,
		},
	}
	_ = p.inodeDiscarders.Delete(&key)
}

func (p *Probe) discardInode(eventType EventType, mountID uint32, inode uint64) error {
	var params inodeDiscarderParameters
	key := inodeDiscarder{
		PathKey: PathKey{
			MountID: mountID,
			Inode:   inode,
		},
	}

	updateFlags := libebpf.UpdateExist
	if err := p.inodeDiscarders.Lookup(key, &params); err != nil {
		updateFlags = libebpf.UpdateAny
	}

	params.EventType |= 1 << (eventType - 1)
	return p.inodeDiscarders.Update(&key, &params, updateFlags)
}

func (p *Probe) discardParentInode(rs *rules.RuleSet, eventType EventType, field eval.Field, filename string, mountID uint32, inode uint64, pathID uint32) (bool, uint32, uint64, error) {
	isDiscarder, err := isParentPathDiscarder(rs, p.regexCache, eventType, field, filename)
	if !isDiscarder {
		return false, 0, 0, err
	}

	parentMountID, parentInode, err := p.resolvers.DentryResolver.GetParent(mountID, inode, pathID)
	if err != nil {
		return false, 0, 0, err
	}

	if err := p.discardInode(eventType, parentMountID, parentInode); err != nil {
		return false, 0, 0, err
	}

	return true, parentMountID, parentInode, nil
}
