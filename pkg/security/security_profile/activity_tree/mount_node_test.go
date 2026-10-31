// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package activitytree

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	testCgroupA = uint64(1001)
	testCgroupB = uint64(1002)
	testNsA     = uint32(4026531840)
	testNsB     = uint32(4026531841)
)

func newMountTestTree() *ActivityTree {
	return NewActivityTree(activityTreeInsertTestValidator{}, nil, "security_profile")
}

func findMount(at *ActivityTree, mountPoint string, mountFlags uint32) *MountNode {
	for _, mn := range at.Mounts {
		if mn.MountPoint == mountPoint && mn.MountFlags == mountFlags {
			return mn
		}
	}
	return nil
}

func findMountWithBase(at *ActivityTree, mountPoint string, mountFlags uint32, inBase bool) *MountNode {
	for _, mn := range at.Mounts {
		if mn.MountPoint == mountPoint && mn.MountFlags == mountFlags && mn.InBaseNamespace == inBase {
			return mn
		}
	}
	return nil
}

func TestInsertMountDedup(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	t0 := time.Unix(0, 1000)
	t1 := time.Unix(0, 2000)

	// first insert creates a node
	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", model.MountAttrReadOnly, tagID, Runtime, t0, false))
	require.Len(t, at.Mounts, 1)

	// identical mount dedups and only bumps last-seen
	require.False(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", model.MountAttrReadOnly, tagID, Runtime, t1, false))
	require.Len(t, at.Mounts, 1)

	firstSeen, lastSeen, ok := at.Mounts[0].GetSeenTimes(tagID)
	require.True(t, ok)
	assert.Equal(t, t0.UnixNano(), firstSeen)
	assert.Equal(t, t1.UnixNano(), lastSeen)
}

func TestInsertMountDifferentRootDedups(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testCgroupA, testNsA, "/merged", "/run/a", "overlay", 0, tagID, Runtime, now, false))
	// same point/fs/flags but a different (per-run) root dedups and refreshes the root
	require.False(t, at.InsertMount(testCgroupA, testNsA, "/merged", "/run/b", "overlay", 0, tagID, Runtime, now, false))
	require.Len(t, at.Mounts, 1)
	assert.Equal(t, "/run/b", at.Mounts[0].MountRoot)
}

func TestInsertMountFlagDifferenceKeepsBoth(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", model.MountAttrReadOnly, tagID, Runtime, now, false))
	// same point/root/fs but writable+executable => new entry
	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.Len(t, at.Mounts, 2)
}

func TestInsertMountAppendOnly(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	// there is no removal on unmount: re-observing the mount keeps a single entry
	require.False(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.Len(t, at.Mounts, 1)
}

func TestInsertMountFlatUnionAcrossNamespaces(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// the same mount observed in two different namespaces collapses into a single
	// deduplicated entry (flat union)
	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	require.False(t, at.InsertMount(testCgroupA, testNsB, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.Len(t, at.Mounts, 1)
}

func TestInsertMountBaseNamespaceFlag(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	at.SetBaseMountNamespace(testCgroupA, testNsA)

	at.InsertMount(testCgroupA, testNsA, "/base", "/", "ext4", 0, tagID, Runtime, now, false)
	at.InsertMount(testCgroupA, testNsB, "/late", "/", "ext4", 0, tagID, Runtime, now, false)

	base := findMount(at, "/base", 0)
	require.NotNil(t, base)
	assert.True(t, base.InBaseNamespace)

	late := findMountWithBase(at, "/late", 0, false)
	require.NotNil(t, late)
	assert.Nil(t, findMountWithBase(at, "/late", 0, true))

	require.True(t, at.InsertMount(testCgroupA, testNsA, "/late", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.NotNil(t, findMountWithBase(at, "/late", 0, false))
	assert.NotNil(t, findMountWithBase(at, "/late", 0, true))
}

func TestInsertMountBaseNamespacePerImageTag(t *testing.T) {
	at := newMountTestTree()
	v1 := at.GetOrInsertImageTag("img:v1")
	v2 := at.GetOrInsertImageTag("img:v2")
	now := time.Unix(0, 1000)

	at.SetBaseMountNamespace(testCgroupA, testNsA)
	at.SetBaseMountNamespace(testCgroupB, testNsA)

	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, v1, Runtime, now, false))
	require.True(t, at.InsertMount(testCgroupB, testNsB, "/data", "/", "ext4", 0, v2, Runtime, now, false))

	baseNode := findMountWithBase(at, "/data", 0, true)
	require.NotNil(t, baseNode)
	assert.True(t, baseNode.HasImageTag(v1))
	assert.False(t, baseNode.HasImageTag(v2))

	nonBaseNode := findMountWithBase(at, "/data", 0, false)
	require.NotNil(t, nonBaseNode)
	assert.False(t, nonBaseNode.HasImageTag(v1))
	assert.True(t, nonBaseNode.HasImageTag(v2))

	at.EvictImageTag("img:v1")
	assert.Nil(t, findMountWithBase(at, "/data", 0, true))
	assert.NotNil(t, findMountWithBase(at, "/data", 0, false))
}

func TestInsertMountNoBaseNamespacePinned(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// with no base namespace pinned, mounts are never flagged as base regardless
	// of insertion order (the base is no longer inferred from the first mount)
	at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false)
	require.Len(t, at.Mounts, 1)
	assert.False(t, at.Mounts[0].InBaseNamespace)
}

func TestMultipleBaseMountNamespaces(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// two containers of the same image, each with its own base namespace
	at.SetBaseMountNamespace(testCgroupA, testNsA)
	at.SetBaseMountNamespace(testCgroupB, testNsB)

	at.InsertMount(testCgroupA, testNsA, "/a", "/", "ext4", 0, tagID, Runtime, now, false)
	at.InsertMount(testCgroupB, testNsB, "/b", "/", "ext4", 0, tagID, Runtime, now, false)
	at.InsertMount(testCgroupA, testNsB, "/cross", "/", "ext4", 0, tagID, Runtime, now, false)

	a := findMount(at, "/a", 0)
	require.NotNil(t, a)
	assert.True(t, a.InBaseNamespace)

	b := findMount(at, "/b", 0)
	require.NotNil(t, b)
	assert.True(t, b.InBaseNamespace)

	cross := findMount(at, "/cross", 0)
	require.NotNil(t, cross)
	assert.False(t, cross.InBaseNamespace)
}

func TestRemoveBaseMountNamespaceIsIndependentPerCgroup(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// two containers share the same base namespace inode
	at.SetBaseMountNamespace(testCgroupA, testNsA)
	at.SetBaseMountNamespace(testCgroupB, testNsA)

	// one unlinks: the inode is still a base namespace for the other
	at.RemoveBaseMountNamespace(testCgroupA)
	at.InsertMount(testCgroupB, testNsA, "/shared", "/", "ext4", 0, tagID, Runtime, now, false)
	shared := findMount(at, "/shared", 0)
	require.NotNil(t, shared)
	assert.True(t, shared.InBaseNamespace)

	// the second unlinks: neither cgroup has it pinned anymore
	at.RemoveBaseMountNamespace(testCgroupB)
	assert.Zero(t, at.baseMountNSByCgroup[testCgroupB])

	at.InsertMount(testCgroupB, testNsA, "/after", "/", "ext4", 0, tagID, Runtime, now, false)
	after := findMount(at, "/after", 0)
	require.NotNil(t, after)
	assert.False(t, after.InBaseNamespace)
}

func TestInsertMountDryRun(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testCgroupA, testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, true))
	assert.Empty(t, at.Mounts)
}

func TestEvictImageTagRemovesOrphanMountsAndClearsSharedTag(t *testing.T) {
	at := newMountTestTree()
	v1 := at.GetOrInsertImageTag("img:v1")
	v2 := at.GetOrInsertImageTag("img:v2")
	now := time.Unix(0, 1000)

	at.InsertMount(testCgroupA, testNsA, "/only-v1", "/", "ext4", 0, v1, Runtime, now, false)
	at.InsertMount(testCgroupA, testNsA, "/shared", "/", "ext4", 0, v1, Runtime, now, false)
	at.InsertMount(testCgroupA, testNsA, "/shared", "/", "ext4", 0, v2, Runtime, now, false)

	at.EvictImageTag("img:v1")

	// a mount that only belonged to the evicted tag is removed
	assert.Nil(t, findMount(at, "/only-v1", 0))

	// a mount still referenced by another tag survives with the evicted tag cleared
	shared := findMount(at, "/shared", 0)
	require.NotNil(t, shared)
	assert.False(t, shared.HasImageTag(v1))
	assert.True(t, shared.HasImageTag(v2))

	require.Len(t, at.Mounts, 1)
}

func TestMountsProtoRoundTrip(t *testing.T) {
	src := newMountTestTree()
	tagID := src.GetOrInsertImageTag("img:v1")
	first := time.Unix(0, 1000)
	last := time.Unix(0, 5000)

	src.SetBaseMountNamespace(testCgroupA, testNsA)
	src.InsertMount(testCgroupA, testNsA, "/", "/", "overlay", 0, tagID, Snapshot, first, false)
	src.Mounts[0].RecordWithTimestamps(tagID, first, last)
	src.InsertMount(testCgroupA, testNsA, "/proc", "/", "proc", model.MountAttrReadOnly|model.MountAttrNoExec, tagID, Runtime, first, false)
	src.InsertMount(testCgroupA, testNsB, "/sys", "/", "sysfs", model.MountAttrNoSUID, tagID, Runtime, first, false)
	src.InsertMount(testCgroupA, testNsA, "/both", "/", "tmpfs", 0, tagID, Runtime, first, false)
	src.InsertMount(testCgroupA, testNsB, "/both", "/", "tmpfs", 0, tagID, Runtime, first, false)

	proto := MountsToProto(src)

	dst := newMountTestTree()
	ProtoDecodeMounts(dst, proto)

	require.Len(t, dst.Mounts, 5)

	root := findMount(dst, "/", 0)
	require.NotNil(t, root)
	assert.Equal(t, "overlay", root.Filesystem)
	assert.True(t, root.InBaseNamespace)

	sys := findMount(dst, "/sys", model.MountAttrNoSUID)
	require.NotNil(t, sys)
	assert.False(t, sys.InBaseNamespace)

	assert.NotNil(t, findMountWithBase(dst, "/both", 0, true))
	assert.NotNil(t, findMountWithBase(dst, "/both", 0, false))

	dstTagID := dst.GetImageTagID("img:v1")
	require.NotZero(t, dstTagID)
	firstSeen, lastSeen, ok := root.GetSeenTimes(dstTagID)
	require.True(t, ok)
	assert.Equal(t, first.UnixNano(), firstSeen)
	assert.Equal(t, last.UnixNano(), lastSeen)
}
