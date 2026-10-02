// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package activitytree

import (
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type mountNodeKey struct {
	mountPoint string
	filesystem string
	mountFlags uint32
}

type MountNode struct {
	NodeBase
	GenerationType NodeGenerationType

	MountPoint string
	MountRoot  string
	Filesystem string
	MountFlags uint32

	InBaseNamespace bool
}

func (mn *MountNode) size() int64 {
	s := int64(unsafe.Sizeof(*mn))
	s += seenBytes(mn.NodeBase)
	s += int64(len(mn.MountPoint) + len(mn.MountRoot) + len(mn.Filesystem))
	return s
}

func NewMountNode(mountPoint, mountRoot, filesystem string, mountFlags uint32, generationType NodeGenerationType, imageTagID uint64, timestamp time.Time) *MountNode {
	node := &MountNode{
		GenerationType: generationType,
		MountPoint:     mountPoint,
		MountRoot:      mountRoot,
		Filesystem:     filesystem,
		MountFlags:     mountFlags,
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTagID(imageTagID, timestamp)
	return node
}

func (at *ActivityTree) indexMount(key mountNodeKey, node *MountNode) {
	if at.mountIndex == nil {
		at.mountIndex = make(map[mountNodeKey]*MountNode)
	}
	at.Mounts = append(at.Mounts, node)
	at.mountIndex[key] = node
}

func (at *ActivityTree) AddBaseMountNamespaceID(nsID uint32) {
	if nsID == 0 {
		return
	}
	if at.baseMountNamespaceIDs == nil {
		at.baseMountNamespaceIDs = make(map[uint32]int)
	}
	at.baseMountNamespaceIDs[nsID]++
}

func (at *ActivityTree) RemoveBaseMountNamespaceID(nsID uint32) {
	if nsID == 0 || at.baseMountNamespaceIDs == nil {
		return
	}
	if at.baseMountNamespaceIDs[nsID] <= 1 {
		delete(at.baseMountNamespaceIDs, nsID)
		return
	}
	at.baseMountNamespaceIDs[nsID]--
}

func (at *ActivityTree) InsertMount(nsID uint32, mountPoint, mountRoot, filesystem string, mountFlags uint32, imageTagID uint64, generationType NodeGenerationType, timestamp time.Time, dryRun bool) bool {
	isBase := nsID != 0 && at.baseMountNamespaceIDs[nsID] > 0

	key := mountNodeKey{mountPoint: mountPoint, filesystem: filesystem, mountFlags: mountFlags}
	if mn, ok := at.mountIndex[key]; ok {
		if !dryRun {
			mn.MountRoot = mountRoot
			mn.AppendImageTagID(imageTagID, timestamp)
			if isBase {
				mn.InBaseNamespace = true
			}
		}
		return false
	}

	if dryRun {
		return true
	}

	node := NewMountNode(mountPoint, mountRoot, filesystem, mountFlags, generationType, imageTagID, timestamp)
	node.InBaseNamespace = isBase
	at.indexMount(key, node)
	at.Stats.SizeBytes += node.size()
	return true
}

func (at *ActivityTree) insertMountEvent(event *model.Event, imageTagID uint64, generationType NodeGenerationType, res *resolvers.EBPFResolvers, dryRun bool) bool {
	m := &event.Mount.Mount

	mountPoint := m.Path
	if res != nil {
		if resolved, _, _, err := res.MountResolver.ResolveMountPath(m.MountID, event.ProcessContext.Process.Pid); err == nil && resolved != "" {
			mountPoint = resolved
		}
	}

	return at.InsertMount(m.NamespaceInode, mountPoint, m.RootStr, m.FSType, m.MountFlags, imageTagID, generationType, event.ResolveEventTime(), dryRun)
}
