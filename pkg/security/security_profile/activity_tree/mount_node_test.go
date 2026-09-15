// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	// first namespace seen becomes the base namespace
	at.InsertMount(testNsA, "/base", "/", "ext4", 0, tagID, Runtime, now, false)
	// a mount seen only in a later namespace is not part of the base
	at.InsertMount(testNsB, "/late", "/", "ext4", 0, tagID, Runtime, now, false)

	base := findMount(at, "/base", 0)
	require.NotNil(t, base)
	assert.True(t, base.InBaseNamespace)

	late := findMount(at, "/late", 0)
	require.NotNil(t, late)
	assert.False(t, late.InBaseNamespace)

	// re-observing the late mount in the base namespace upgrades its flag
	require.False(t, at.InsertMount(testNsA, "/late", "/", "ext4", 0, tagID, Runtime, now, false))
	assert.True(t, late.InBaseNamespace)
}

func TestInsertMountDryRun(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	require.True(t, at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, true))
	assert.Empty(t, at.Mounts)
}

func TestEvictImageTagKeepsMountsButClearsTag(t *testing.T) {
	at := newMountTestTree()
	tagID := at.GetOrInsertImageTag("img:v1")
	now := time.Unix(0, 1000)

	at.InsertMount(testNsA, "/data", "/", "ext4", 0, tagID, Runtime, now, false)

	at.EvictImageTag("img:v1")

	// the mount node survives eviction (append-only)
	require.Len(t, at.Mounts, 1)

	// the freed slot is reused by a new tag; the surviving mount must not be
	// misattributed to it
	reusedID := at.GetOrInsertImageTag("img:v2")
	assert.Equal(t, tagID, reusedID)
	assert.False(t, at.Mounts[0].HasImageTag(reusedID))
}

func TestMountsProtoRoundTrip(t *testing.T) {
	src := newMountTestTree()
	tagID := src.GetOrInsertImageTag("img:v1")
	first := time.Unix(0, 1000)
	last := time.Unix(0, 5000)

	src.InsertMount(testNsA, "/", "/", "overlay", 0, tagID, Snapshot, first, false)
	src.Mounts[0].RecordWithTimestamps(tagID, first, last)
	src.InsertMount(testNsA, "/proc", "/", "proc", model.MountAttrReadOnly|model.MountAttrNoExec, tagID, Runtime, first, false)
	src.InsertMount(testNsB, "/sys", "/", "sysfs", model.MountAttrNoSUID, tagID, Runtime, first, false)

	proto := MountsToProto(src)

	dst := newMountTestTree()
	ProtoDecodeMounts(dst, proto)

	require.Len(t, dst.Mounts, 3)

	root := findMount(dst, "/", 0)
	require.NotNil(t, root)
	assert.Equal(t, "overlay", root.Filesystem)
	assert.True(t, root.InBaseNamespace)

	sys := findMount(dst, "/sys", model.MountAttrNoSUID)
	require.NotNil(t, sys)
	assert.False(t, sys.InBaseNamespace)

	dstTagID := dst.GetImageTagID("img:v1")
	require.NotZero(t, dstTagID)
	times, ok := root.GetSeenTimes(dstTagID)
	require.True(t, ok)
	assert.Equal(t, first, times.FirstSeen)
	assert.Equal(t, last, times.LastSeen)
}
