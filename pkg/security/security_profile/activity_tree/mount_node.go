// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// MountNode is used to store a mount observed for a workload. Mount nodes live
// in a single deduplicated table on the ActivityTree and are append-only: they
// are never removed when the underlying mount is unmounted.
type MountNode struct {
	NodeBase
	GenerationType NodeGenerationType

	MountPoint string
	MountRoot  string
	Filesystem string
	MountFlags uint32

	// InBaseNamespace is true when this mount was observed in the workload's base
	// (first-seen) mount namespace. Mounts seen only in later namespaces (e.g.
	// after a container restart) have it false.
	InBaseNamespace bool
}

// size approximates this node's heap footprint
func (mn *MountNode) size() int64 {
	s := int64(unsafe.Sizeof(*mn))
	s += seenBytes(mn.NodeBase)
	s += int64(len(mn.MountPoint) + len(mn.MountRoot) + len(mn.Filesystem))
	return s
}

// NewMountNode returns a new MountNode instance
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

// InsertMount inserts a mount into the workload's deduplicated mount table.
// Mounts observed across different mount namespaces are collapsed into a single
// union, deduplicating on (mount point, mount root, filesystem, mount flags);
// the mount-namespace inode is only used to flag whether the mount belongs to
// the base (first-seen) namespace. A matching entry bumps its last-seen
// timestamp (and gains the base flag if seen in the base namespace); a
// difference in flags is recorded as a new entry. Returns true if a new node
// was created.
func (at *ActivityTree) InsertMount(nsID uint32, mountPoint, mountRoot, filesystem string, mountFlags uint32, imageTagID uint64, generationType NodeGenerationType, timestamp time.Time, dryRun bool) bool {
	if !dryRun && at.baseMountNamespaceID == 0 && nsID != 0 {
		at.baseMountNamespaceID = nsID
	}
	isBase := nsID != 0 && nsID == at.baseMountNamespaceID

	for _, mn := range at.Mounts {
		if mn.MountPoint == mountPoint && mn.MountRoot == mountRoot && mn.Filesystem == filesystem && mn.MountFlags == mountFlags {
			if !dryRun {
				mn.AppendImageTagID(imageTagID, timestamp)
				if isBase {
					mn.InBaseNamespace = true
				}
			}
			return false
		}
	}

	if dryRun {
		return true
	}

	node := NewMountNode(mountPoint, mountRoot, filesystem, mountFlags, generationType, imageTagID, timestamp)
	node.InBaseNamespace = isBase
	at.Mounts = append(at.Mounts, node)
	at.Stats.SizeBytes += node.size()
	return true
}

// insertMountEvent extracts the mount from a mount event and inserts it into the
// workload's deduplicated mount table.
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
