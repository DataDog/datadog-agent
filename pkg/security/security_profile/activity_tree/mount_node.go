// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package activitytree

import (
	"slices"
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

	baseNamespaceTagIDs []uint64
}

func (mn *MountNode) size() int64 {
	s := int64(unsafe.Sizeof(*mn))
	s += seenBytes(mn.NodeBase)
	s += int64(cap(mn.baseNamespaceTagIDs)) * int64(unsafe.Sizeof(uint64(0)))
	s += int64(len(mn.MountPoint) + len(mn.MountRoot) + len(mn.Filesystem))
	return s
}

func (mn *MountNode) SetBaseNamespace(imageTagID uint64) {
	if imageTagID == 0 || slices.Contains(mn.baseNamespaceTagIDs, imageTagID) {
		return
	}
	mn.baseNamespaceTagIDs = append(mn.baseNamespaceTagIDs[:len(mn.baseNamespaceTagIDs):len(mn.baseNamespaceTagIDs)], imageTagID)
}

func (mn *MountNode) GetBaseNamespace(imageTagID uint64) bool {
	return imageTagID != 0 && slices.Contains(mn.baseNamespaceTagIDs, imageTagID)
}

func (mn *MountNode) EvictImageTag(imageTagID uint64) bool {
	mn.baseNamespaceTagIDs = slices.DeleteFunc(mn.baseNamespaceTagIDs, func(id uint64) bool {
		return id == imageTagID
	})
	return mn.NodeBase.EvictImageTag(imageTagID)
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

// AddBaseMountNamespaceID records nsID as a base mount namespace for imageTagID. Base-namespace
// membership is tracked per image tag because a mount can belong to the base namespace in one
// workload version and not in another.
func (at *ActivityTree) AddBaseMountNamespaceID(imageTagID uint64, nsID uint32) {
	if nsID == 0 || imageTagID == 0 {
		return
	}
	if at.baseMountNamespaceIDs == nil {
		at.baseMountNamespaceIDs = make(map[uint64]map[uint32]int)
	}
	ids := at.baseMountNamespaceIDs[imageTagID]
	if ids == nil {
		ids = make(map[uint32]int)
		at.baseMountNamespaceIDs[imageTagID] = ids
	}
	ids[nsID]++
}

// RemoveBaseMountNamespaceID drops a reference to nsID as a base mount namespace for imageTagID.
func (at *ActivityTree) RemoveBaseMountNamespaceID(imageTagID uint64, nsID uint32) {
	if nsID == 0 || imageTagID == 0 || at.baseMountNamespaceIDs == nil {
		return
	}
	ids := at.baseMountNamespaceIDs[imageTagID]
	if ids == nil {
		return
	}
	if ids[nsID] <= 1 {
		delete(ids, nsID)
		if len(ids) == 0 {
			delete(at.baseMountNamespaceIDs, imageTagID)
		}
		return
	}
	ids[nsID]--
}

func (at *ActivityTree) isBaseMountNamespace(imageTagID uint64, nsID uint32) bool {
	if nsID == 0 || imageTagID == 0 {
		return false
	}
	return at.baseMountNamespaceIDs[imageTagID][nsID] > 0
}

func (at *ActivityTree) InsertMount(nsID uint32, mountPoint, mountRoot, filesystem string, mountFlags uint32, imageTagID uint64, generationType NodeGenerationType, timestamp time.Time, dryRun bool) bool {
	isBase := at.isBaseMountNamespace(imageTagID, nsID)

	key := mountNodeKey{mountPoint: mountPoint, filesystem: filesystem, mountFlags: mountFlags}
	if mn, ok := at.mountIndex[key]; ok {
		if !dryRun {
			mn.MountRoot = mountRoot
			mn.AppendImageTagID(imageTagID, timestamp)
			if isBase {
				mn.SetBaseNamespace(imageTagID)
			}
		}
		return false
	}

	if dryRun {
		return true
	}

	node := NewMountNode(mountPoint, mountRoot, filesystem, mountFlags, generationType, imageTagID, timestamp)
	if isBase {
		node.SetBaseNamespace(imageTagID)
	}
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
