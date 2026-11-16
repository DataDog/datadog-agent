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
	mountPoint      string
	filesystem      string
	mountFlags      uint32
	inBaseNamespace bool
}

type MountNode struct {
	NodeBase
	GenerationType NodeGenerationType

	MountPoint string
	MountRoot  string
	Filesystem string
	MountFlags uint32

	InBaseNamespace  bool
	baseNamespaceIDs []uint32
}

func (mn *MountNode) addBaseNamespaceID(nsID uint32) {
	if slices.Contains(mn.baseNamespaceIDs, nsID) {
		return
	}
	mn.baseNamespaceIDs = append(mn.baseNamespaceIDs[:len(mn.baseNamespaceIDs):len(mn.baseNamespaceIDs)], nsID)
}

func (mn *MountNode) removeBaseNamespaceID(nsID uint32) bool {
	for i, id := range mn.baseNamespaceIDs {
		if id == nsID {
			mn.baseNamespaceIDs[i] = mn.baseNamespaceIDs[len(mn.baseNamespaceIDs)-1]
			mn.baseNamespaceIDs = mn.baseNamespaceIDs[:len(mn.baseNamespaceIDs)-1]
			return true
		}
	}
	return false
}

func (mn *MountNode) size() int64 {
	s := int64(unsafe.Sizeof(*mn))
	s += seenBytes(mn.NodeBase)
	s += int64(cap(mn.baseNamespaceIDs)) * int64(unsafe.Sizeof(uint32(0)))
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

func (at *ActivityTree) SetBaseMountNamespace(cgroupInode uint64, nsID uint32) {
	if cgroupInode == 0 || nsID == 0 {
		return
	}
	if at.baseMountNSByCgroup == nil {
		at.baseMountNSByCgroup = make(map[uint64]uint32)
	}
	oldNsID := at.baseMountNSByCgroup[cgroupInode]
	at.baseMountNSByCgroup[cgroupInode] = nsID
	if oldNsID != 0 && oldNsID != nsID && !at.hasBaseMountNamespace(oldNsID) {
		at.demoteBaseMountNamespace(oldNsID)
	}
}

func (at *ActivityTree) hasBaseMountNamespace(nsID uint32) bool {
	for _, pinnedNsID := range at.baseMountNSByCgroup {
		if pinnedNsID == nsID {
			return true
		}
	}
	return false
}

func (at *ActivityTree) demoteBaseMountNamespace(nsID uint32) {
	newMounts := at.Mounts[:0]
	for _, mn := range at.Mounts {
		if !mn.InBaseNamespace || !mn.removeBaseNamespaceID(nsID) {
			newMounts = append(newMounts, mn)
			continue
		}

		stillBase := false
		for _, otherNs := range mn.baseNamespaceIDs {
			if otherNs == 0 || at.hasBaseMountNamespace(otherNs) {
				stillBase = true
				break
			}
		}
		if stillBase {
			newMounts = append(newMounts, mn)
			continue
		}

		baseKey := mountNodeKey{mountPoint: mn.MountPoint, filesystem: mn.Filesystem, mountFlags: mn.MountFlags, inBaseNamespace: true}
		delete(at.mountIndex, baseKey)

		nonBaseKey := mountNodeKey{mountPoint: mn.MountPoint, filesystem: mn.Filesystem, mountFlags: mn.MountFlags, inBaseNamespace: false}
		if existing, ok := at.mountIndex[nonBaseKey]; ok {
			before := existing.size()
			existing.mergeSeen(&mn.NodeBase)
			at.Stats.SizeBytes += (existing.size() - before) - mn.size()
			continue
		}

		mn.InBaseNamespace = false
		at.mountIndex[nonBaseKey] = mn
		newMounts = append(newMounts, mn)
	}
	at.Mounts = newMounts
}

func (at *ActivityTree) RemoveBaseMountNamespace(cgroupInode uint64) {
	if cgroupInode == 0 || at.baseMountNSByCgroup == nil {
		return
	}
	delete(at.baseMountNSByCgroup, cgroupInode)
}

const (
	maxSeededMountNamespacesPerTag = 64
	maxSeedAttemptsPerNamespace    = 3
)

type mountNamespaceSeedState struct {
	seeded   bool
	failures uint8
}

func (at *ActivityTree) ClaimMountNamespaceSeed(imageTagID uint64, nsID uint32) bool {
	if nsID == 0 || imageTagID == 0 {
		return false
	}
	if at.seededMountNamespaceIDs == nil {
		at.seededMountNamespaceIDs = make(map[uint64]map[uint32]mountNamespaceSeedState)
	}
	ids := at.seededMountNamespaceIDs[imageTagID]
	if ids == nil {
		ids = make(map[uint32]mountNamespaceSeedState)
		at.seededMountNamespaceIDs[imageTagID] = ids
	}
	state, exists := ids[nsID]
	if state.seeded || state.failures >= maxSeedAttemptsPerNamespace {
		return false
	}
	if !exists && len(ids) >= maxSeededMountNamespacesPerTag {
		return false
	}
	state.seeded = true
	ids[nsID] = state
	return true
}

func (at *ActivityTree) ReleaseMountNamespaceSeed(imageTagID uint64, nsID uint32) {
	if nsID == 0 || imageTagID == 0 || at.seededMountNamespaceIDs == nil {
		return
	}
	ids := at.seededMountNamespaceIDs[imageTagID]
	state, ok := ids[nsID]
	if !ok {
		return
	}
	state.seeded = false
	state.failures++
	ids[nsID] = state
}

func (at *ActivityTree) isBaseMountNamespace(cgroupInode uint64, nsID uint32) bool {
	if cgroupInode == 0 || nsID == 0 {
		return false
	}
	return at.baseMountNSByCgroup[cgroupInode] == nsID
}

func (at *ActivityTree) InsertMount(cgroupInode uint64, nsID uint32, mountPoint, mountRoot, filesystem string, mountFlags uint32, imageTagID uint64, generationType NodeGenerationType, timestamp time.Time, dryRun bool) bool {
	isBase := at.isBaseMountNamespace(cgroupInode, nsID)

	key := mountNodeKey{mountPoint: mountPoint, filesystem: filesystem, mountFlags: mountFlags, inBaseNamespace: isBase}
	if mn, ok := at.mountIndex[key]; ok {
		if !dryRun {
			before := mn.size()
			mn.MountRoot = mountRoot
			if isBase && nsID != 0 {
				mn.addBaseNamespaceID(nsID)
			}
			mn.AppendImageTagID(imageTagID, timestamp)
			at.Stats.SizeBytes += mn.size() - before
		}
		return false
	}

	if dryRun {
		return true
	}

	node := NewMountNode(mountPoint, mountRoot, filesystem, mountFlags, generationType, imageTagID, timestamp)
	node.InBaseNamespace = isBase
	if isBase && nsID != 0 {
		node.addBaseNamespaceID(nsID)
	}
	at.indexMount(key, node)
	at.Stats.SizeBytes += node.size()
	return true
}

func (at *ActivityTree) insertMountEvent(event *model.Event, imageTagID uint64, generationType NodeGenerationType, res *resolvers.EBPFResolvers, dryRun bool) bool {
	m := &event.Mount.Mount

	var cgroupInode uint64
	if event.ProcessContext != nil {
		cgroupInode = event.ProcessContext.Process.CGroup.CGroupPathKey.Inode
		if res != nil && res.CGroupResolver != nil && !event.ProcessContext.Process.ContainerContext.IsNull() {
			if cgce := res.CGroupResolver.GetCacheEntryContainerID(event.ProcessContext.Process.ContainerContext.ContainerID); cgce != nil && cgce.GetCGroupInode() != 0 {
				cgroupInode = cgce.GetCGroupInode()
			}
		}
	}

	mountPoint := m.Path
	if res != nil {
		if resolved, _, _, err := res.MountResolver.ResolveMountPath(m.MountID, event.ProcessContext.Process.Pid); err == nil && resolved != "" {
			mountPoint = resolved
		}
	}

	return at.InsertMount(cgroupInode, m.NamespaceInode, mountPoint, m.RootStr, m.FSType, m.MountFlags, imageTagID, generationType, event.ResolveEventTime(), dryRun)
}
