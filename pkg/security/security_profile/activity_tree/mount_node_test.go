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
	testNsA = uint32(4026531840)
	testNsB = uint32(4026531841)
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

func TestInsertMountDedup(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	t0 := time.Unix(0, 1000)
	t1 := time.Unix(0, 2000)

	// first insert creates a node
	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", model.MountAttrReadOnly, tagID, Runtime, t0, false))
	require.Len(t, at.Mounts, 1)

	// identical mount dedups and only bumps last-seen
	require.False(t, at.InsertMount(testNsA, "/data", "/", "ext4", model.MountAttrReadOnly, tagID, Runtime, t1, false))
	require.Len(t, at.Mounts, 1)

	times, ok := at.Mounts[0].GetSeenTimes(tagID)
	require.True(t, ok)
	assert.Equal(t, t0, times.FirstSeen)
	assert.Equal(t, t1, times.LastSeen)
}

func TestInsertMountDifferentRootDedups(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testNsA, "/merged", "/run/a", "overlay", 0, tagID, Runtime, now, false))
	// same point/fs/flags but a different (per-run) root dedups and refreshes the root
	require.False(t, at.InsertMount(testNsA, "/merged", "/run/b", "overlay", 0, tagID, Runtime, now, false))
	require.Len(t, at.Mounts, 1)
	assert.Equal(t, "/run/b", at.Mounts[0].MountRoot)
}

func TestInsertMountFlagDifferenceKeepsBoth(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", model.MountAttrReadOnly, tagID, Runtime, now, false))
	// same point/root/fs but writable+executable => new entry
	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.Len(t, at.Mounts, 2)
}

func TestInsertMountAppendOnly(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	// there is no removal on unmount: re-observing the mount keeps a single entry
	require.False(t, at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.Len(t, at.Mounts, 1)
}

func TestInsertMountFlatUnionAcrossNamespaces(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// the same mount observed in two different namespaces collapses into a single
	// deduplicated entry (flat union)
	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	require.False(t, at.InsertMount(testNsB, "/data", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.Len(t, at.Mounts, 1)
}

func TestInsertMountBaseNamespaceFlag(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	at.AddBaseMountNamespaceID(testNsA)

	at.InsertMount(testNsA, "/base", "/", "ext4", 0, tagID, Runtime, now, false)
	// a mount seen only in a later namespace is not part of the base
	at.InsertMount(testNsB, "/late", "/", "ext4", 0, tagID, Runtime, now, false)

	base := findMount(at, "/base", 0)
	require.NotNil(t, base)
	assert.True(t, base.IsBaseNamespace(tagID))

	late := findMount(at, "/late", 0)
	require.NotNil(t, late)
	assert.False(t, late.IsBaseNamespace(tagID))

	// re-observing the late mount in the base namespace upgrades its flag
	require.False(t, at.InsertMount(testNsA, "/late", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.True(t, late.IsBaseNamespace(tagID))
}

func TestInsertMountBaseNamespacePerImageTag(t *testing.T) {
	at := newMountTestTree()
	v1 := at.GetOrInsertImageTag("img:v1")
	v2 := at.GetOrInsertImageTag("img:v2")
	now := time.Unix(0, 1000)

	at.AddBaseMountNamespaceID(testNsA)

	// v1 observes /foo in the base namespace
	at.InsertMount(testNsA, "/foo", "/", "ext4", 0, v1, Runtime, now, false)
	// v2 observes the same mount, but outside the base namespace
	at.InsertMount(testNsB, "/foo", "/", "ext4", 0, v2, Runtime, now, false)

	foo := findMount(at, "/foo", 0)
	require.NotNil(t, foo)
	assert.True(t, foo.IsBaseNamespace(v1), "expected /foo to be base for v1")
	assert.False(t, foo.IsBaseNamespace(v2), "expected /foo not to be base for v2")
	assert.True(t, foo.IsBaseNamespaceAny())
}

func TestInsertMountNoBaseNamespacePinned(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// with no base namespace pinned, mounts are never flagged as base regardless
	// of insertion order (the base is no longer inferred from the first mount)
	at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false)
	require.Len(t, at.Mounts, 1)
	assert.False(t, at.Mounts[0].IsBaseNamespace(tagID))
}

func TestMultipleBaseMountNamespaces(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// two containers of the same image, each with its own base namespace
	at.AddBaseMountNamespaceID(testNsA)
	at.AddBaseMountNamespaceID(testNsB)

	at.InsertMount(testNsA, "/a", "/", "ext4", 0, tagID, Runtime, now, false)
	at.InsertMount(testNsB, "/b", "/", "ext4", 0, tagID, Runtime, now, false)

	a := findMount(at, "/a", 0)
	require.NotNil(t, a)
	assert.True(t, a.IsBaseNamespace(tagID))

	b := findMount(at, "/b", 0)
	require.NotNil(t, b)
	assert.True(t, b.IsBaseNamespace(tagID))
}

func TestRemoveBaseMountNamespaceIsRefcounted(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	// two containers share the same base namespace inode
	at.AddBaseMountNamespaceID(testNsA)
	at.AddBaseMountNamespaceID(testNsA)

	// one unlinks: the inode is still a base namespace for the other
	at.RemoveBaseMountNamespaceID(testNsA)
	at.InsertMount(testNsA, "/shared", "/", "ext4", 0, tagID, Runtime, now, false)
	shared := findMount(at, "/shared", 0)
	require.NotNil(t, shared)
	assert.True(t, shared.IsBaseNamespace(tagID))

	// the second unlinks: the inode is no longer a base namespace
	at.RemoveBaseMountNamespaceID(testNsA)
	assert.Zero(t, at.baseMountNamespaceIDs[testNsA])

	at.InsertMount(testNsA, "/after", "/", "ext4", 0, tagID, Runtime, now, false)
	after := findMount(at, "/after", 0)
	require.NotNil(t, after)
	assert.False(t, after.IsBaseNamespace(tagID))
}

func TestInsertMountDryRun(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, true))
	assert.Empty(t, at.Mounts)
}

func TestEvictImageTagRemovesOrphanMountsAndClearsSharedTag(t *testing.T) {
	at := newMountTestTree()
	v1 := at.GetOrInsertImageTag("img:v1")
	v2 := at.GetOrInsertImageTag("img:v2")
	now := time.Unix(0, 1000)

	at.InsertMount(testNsA, "/only-v1", "/", "ext4", 0, v1, Runtime, now, false)
	at.InsertMount(testNsA, "/shared", "/", "ext4", 0, v1, Runtime, now, false)
	at.InsertMount(testNsA, "/shared", "/", "ext4", 0, v2, Runtime, now, false)

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

	src.AddBaseMountNamespaceID(testNsA)
	src.InsertMount(testNsA, "/", "/", "overlay", 0, tagID, Snapshot, first, false)
	src.Mounts[0].RecordWithTimestamps(tagID, first, last)
	src.InsertMount(testNsA, "/proc", "/", "proc", model.MountAttrReadOnly|model.MountAttrNoExec, tagID, Runtime, first, false)
	src.InsertMount(testNsB, "/sys", "/", "sysfs", model.MountAttrNoSUID, tagID, Runtime, first, false)

	proto := MountsToProto(src)

	dst := newMountTestTree()
	ProtoDecodeMounts(dst, proto)

	require.Len(t, dst.Mounts, 3)

	dstTagID := dst.GetImageTagID("img:v1")
	require.NotZero(t, dstTagID)

	root := findMount(dst, "/", 0)
	require.NotNil(t, root)
	assert.Equal(t, "overlay", root.Filesystem)
	assert.True(t, root.IsBaseNamespace(dstTagID))

	sys := findMount(dst, "/sys", model.MountAttrNoSUID)
	require.NotNil(t, sys)
	assert.False(t, sys.IsBaseNamespace(dstTagID))

	times, ok := root.GetSeenTimes(dstTagID)
	require.True(t, ok)
	assert.Equal(t, first, times.FirstSeen)
	assert.Equal(t, last, times.LastSeen)
}
