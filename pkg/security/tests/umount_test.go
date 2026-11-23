// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
	"syscall"
	"testing"
	"time"
)

func TmpMountAtLegacyAPI(dir string) error {
	return unix.Mount("", dir, "tmpfs", 0, "size=1M")
}

func TestUmount(t *testing.T) {
	SkipIfNotAvailable(t)

	if ebpfLessEnabled {
		t.Skip("not supported in ebpfless mode")
	}

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	mountDir := t.TempDir()
	err = TmpMountAtLegacyAPI(mountDir)
	if err != nil {
		t.Fatal(err)
	}
	unmounted := false
	defer func() {
		if !unmounted {
			_ = unix.Unmount(mountDir, syscall.MNT_DETACH)
		}
	}()

	mountID, err := getMountID(mountDir)
	if err != nil {
		t.Fatal(err)
	}

	var mnt *model.Mount
	if !assert.Eventually(t, func() bool {
		var resolveErr error
		mnt, _, _, resolveErr = p.Resolvers.MountResolver.ResolveMount(mountID, 0)
		return resolveErr == nil && mnt != nil
	}, 2*time.Second, 50*time.Millisecond) {
		t.Fatal("failed to resolve mounted tmpfs")
	}

	assert.Equal(t, "tmpfs", mnt.FSType)

	err = unix.Unmount(mountDir, syscall.MNT_DETACH)
	if err != nil {
		t.Fatal(err)
	}
	unmounted = true

	// Resolve the mount after detaching, without using redemption or reloading. Should return an error
	assert.Eventually(t, func() bool {
		mnt, _, _, err = p.Resolvers.MountResolver.ResolveMount(mountID, 0)
		return err != nil && mnt == nil
	}, 2*time.Second, 50*time.Millisecond)
}
