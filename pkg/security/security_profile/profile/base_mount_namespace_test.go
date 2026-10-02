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
func insertMountIn(t *testing.T, p *Profile, nsID uint32, mountPoint string) bool {
	t.Helper()

	mnt := &model.Mount{
		NamespaceInode: nsID,
		Path:           mountPoint,
		RootStr:        "/",
		FSType:         "ext4",
	}
	require.True(t, p.InsertMount(mnt, "img:v1", activity_tree.Runtime, time.Unix(0, 1000)))

	for _, mn := range p.ActivityTree.Mounts {
		if mn.MountPoint == mountPoint {
			return mn.IsBaseNamespaceAny()
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

		assert.True(t, insertMountIn(t, p, testNsA, "/in-base"))
		assert.False(t, insertMountIn(t, p, testNsB, "/elsewhere"))
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

		assert.True(t, insertMountIn(t, p, testNsB, "/real-base"))
		assert.False(t, insertMountIn(t, p, testNsA, "/was-wrongly-base"),
			"the superseded namespace must stop counting as base")
	})

	t.Run("a confirming authoritative pin is promoted without re-seeding", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, false))
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true),
			"the namespace did not change, so there is nothing to seed")

		// the pin is now authoritative, so a later differing value must not displace it
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsB, true))
		assert.True(t, insertMountIn(t, p, testNsA, "/still-base"))
	})

	t.Run("a provisional pin never displaces an authoritative one", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))
		assert.False(t, p.PinBaseMountNamespace(testCgroupA, testNsB, false))

		assert.True(t, insertMountIn(t, p, testNsA, "/base"))
		assert.False(t, insertMountIn(t, p, testNsB, "/guess"))
	})

	t.Run("pins are independent per cgroup", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))
		require.True(t, p.PinBaseMountNamespace(testCgroupB, testNsB, true),
			"a second container of the same image has its own base namespace")

		assert.True(t, insertMountIn(t, p, testNsA, "/base-a"))
		assert.True(t, insertMountIn(t, p, testNsB, "/base-b"))
	})

	t.Run("unlinking a workload releases its pin", func(t *testing.T) {
		p := newPinTestProfile()
		require.True(t, p.PinBaseMountNamespace(testCgroupA, testNsA, true))

		p.RemoveBaseMountNamespace(testCgroupA)
		assert.False(t, insertMountIn(t, p, testNsA, "/after-unlink"))

		assert.True(t, p.PinBaseMountNamespace(testCgroupA, testNsB, false),
			"a released cgroup must be pinnable again")
	})
}
