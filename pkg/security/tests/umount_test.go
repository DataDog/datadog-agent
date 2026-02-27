// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
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

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mountDir := t.TempDir()
	err = TmpMountAtLegacyAPI(mountDir)
	if err != nil {
		t.Fatal(err)
	}
	mountID, err := getMountID(mountDir)
	if err != nil {
		t.Fatal(err)
	}

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)

	if !ok {
		t.Skip("not supported")
	}

	time.Sleep(1 * time.Second)

	mnt, _, _, err := p.Resolvers.MountResolver.ResolveMount(mountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if mnt == nil {
		t.Fatal("nil mount")
	}

	assert.Equal(t, "tmpfs", mnt.FSType)

	err = unix.Unmount(mountDir, syscall.MNT_DETACH)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	// Resolve the mount after detaching, without using redemption or reloading. Should return nil
	mnt, _, _, err = p.Resolvers.MountResolver.ResolveMount(mountID, 0)
	if err == nil {
		t.Fatal("No error")
	}
	if mnt != nil {
		t.Fatal("mount not nil")
	}
}
