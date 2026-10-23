// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

const (
	testCgroupA = uint64(1001)
	testCgroupB = uint64(1002)
	testNsA     = uint32(4026531840)
	testNsB     = uint32(4026531841)
)

func newPinTestProfile() *Profile {
	return New(WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}))
}

// insertMountIn inserts a mount observed in nsID at mountPoint and reports whether the
// profile considered it part of a base mount namespace. The mount point doubles as the
// dedup key, so each call must use a distinct one to get an independent answer.
func insertMountIn(t *testing.T, p *Profile, cgroupInode uint64, nsID uint32, mountPoint string) bool {
	t.Helper()

	mnt := &model.Mount{
		NamespaceInode: nsID,
		Path:           mountPoint,
		RootStr:        "/",
		FSType:         "ext4",
	}
	require.True(t, p.InsertMount(mnt, cgroupInode, "img:v1", activity_tree.Runtime, time.Unix(0, 1000)))

	for _, mn := range p.ActivityTree.Mounts {
		if mn.MountPoint == mountPoint {
			return mn.InBaseNamespace
		}
	}
	t.Fatalf("mount %q was not inserted", mountPoint)
	return false
}

func TestProfile_PinBaseMountNamespace(t *testing.T) {
	t.Run("zero namespace is never pinned", func(t *testing.T) {
		p := newPinTestProfile()
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, 0, true))
	})

	t.Run("first pin seeds and flags its namespace", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, false))

		assert.True(t, insertMountIn(t, p, testCgroupA, testNsA, "/in-base"))
		assert.False(t, insertMountIn(t, p, testCgroupA, testNsB, "/elsewhere"))
	})

	t.Run("repeating a provisional pin does not re-seed", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, false))
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsA, false),
			"an unchanged pin must not ask the caller to seed again")
	})

	t.Run("an authoritative pin replaces a differing provisional one", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, false))
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsB, true),
			"correcting the namespace must ask the caller to seed the real base")

		assert.True(t, insertMountIn(t, p, testCgroupA, testNsB, "/real-base"))
		assert.False(t, insertMountIn(t, p, testCgroupA, testNsA, "/was-wrongly-base"),
			"the superseded namespace must stop counting as base")
	})

	t.Run("a confirming authoritative pin is promoted without re-seeding", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, false))
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true),
			"the namespace did not change, so there is nothing to seed")

		// the pin is now authoritative, so a later differing value must not displace it
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsB, true))
		assert.True(t, insertMountIn(t, p, testCgroupA, testNsA, "/still-base"))
	})

	t.Run("a provisional pin never displaces an authoritative one", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsB, false))

		assert.True(t, insertMountIn(t, p, testCgroupA, testNsA, "/base"))
		assert.False(t, insertMountIn(t, p, testCgroupA, testNsB, "/guess"))
	})

	t.Run("pins are independent per cgroup", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))
		require.True(t, p.PinBaseMountNamespace(testCgroupB, testNsB, true),
			"a second container of the same image has its own base namespace")

		assert.True(t, insertMountIn(t, p, testCgroupA, testNsA, "/base-a"))
		assert.True(t, insertMountIn(t, p, testCgroupB, testNsB, "/base-b"))
		assert.False(t, insertMountIn(t, p, testCgroupA, testNsB, "/cross-a-in-b"))
	})

	t.Run("unlinking a workload releases its pin", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))

		p.RemoveBaseMountNamespace(testCgroupA)
		assert.False(t, insertMountIn(t, p, testCgroupA, testNsA, "/after-unlink"))

		assert.True(t, p.PinBaseMountNamespace(testCgroupA, testNsB, false),
			"a released cgroup must be pinnable again")
	})
}

func TestProfile_ClaimMountNamespaceSeed(t *testing.T) {
	t.Run("zero namespace is never claimed", func(t *testing.T) {
		p := newPinTestProfile()
		assert.False(t, p.ClaimMountNamespaceSeed(testCgroupA, 0, "img:v1"))
	})

	t.Run("claims a foreign namespace once per image tag without marking it as base", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))

		assert.False(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsA, "img:v1"),
			"a namespace already pinned as this cgroup's base must not be seeded as foreign")
		assert.True(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsB, "img:v1"),
			"a foreign namespace entered via setns should be seeded on first observation")
		assert.False(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsB, "img:v1"),
			"the same foreign namespace must not be seeded twice for the same tag")
		assert.True(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsB, "img:v2"),
			"another image tag should still seed the namespace")

		assert.True(t, insertMountIn(t, p, testCgroupA, testNsA, "/in-base"))
		assert.False(t, insertMountIn(t, p, testCgroupA, testNsB, "/in-foreign"),
			"claiming a foreign namespace for seeding must not mark it as a base namespace")
	})

	t.Run("claims and records base_namespace=false when a cgroup enters another same-tag cgroup's base namespace", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))
		require.True(t, p.PinBaseMountNamespace(testCgroupB, testNsB, true))

		assert.True(t, insertMountIn(t, p, testCgroupB, testNsB, "/replica-mount"),
			"mounts for testCgroupB in its own base namespace must have base_namespace=true")

		assert.True(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsB, "img:v1"),
			"testCgroupA entering testCgroupB's namespace must still claim it for non-base seeding")

		mnt := &model.Mount{
			NamespaceInode: testNsB,
			Path:           "/replica-mount",
			RootStr:        "/",
			FSType:         "ext4",
		}
		require.True(t, p.InsertMount(mnt, testCgroupA, "img:v1", activity_tree.Snapshot, time.Unix(0, 2000)),
			"seeding testNsB for testCgroupA must create a separate MountNode with base_namespace=false")

		var foundBase, foundNonBase bool
		for _, mn := range p.ActivityTree.Mounts {
			if mn.MountPoint == "/replica-mount" {
				if mn.InBaseNamespace {
					foundBase = true
				} else {
					foundNonBase = true
				}
			}
		}
		assert.True(t, foundBase)
		assert.True(t, foundNonBase)
	})

	t.Run("evicting an image tag clears its claimed namespaces", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsB, "img:v1"))
		p.ActivityTree.EvictImageTag("img:v1")

		assert.True(t, p.ClaimMountNamespaceSeed(testCgroupA, testNsB, "img:v2"),
			"reusing the freed tag slot must not inherit claimed namespaces from the evicted tag")
	})
}
