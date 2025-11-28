// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
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

	fmt.Println("Created new mount at", mountDir, mountID)

	found := false
	fmt.Println("Start")
	err = test.GetProbeEvent(func() error {
		err = unix.Unmount(mountDir, syscall.MNT_DETACH)
		if err != nil {
			fmt.Println("Error unmounting", err)
		}

		if err != nil {
			fmt.Println("Error closing", err)
		}
		return nil
	}, func(event *model.Event) bool {
		if event.GetType() != "mount_released" {
			return false
		}
		if event.MountReleased.MountID != mountID {
			return false
		}
		found = true
		return true
	}, 3*time.Second, model.MountReleasedEventType)
	fmt.Println("Finish")
	if err != nil {
		fmt.Println("Error getting probe event", err)
	}

	assert.True(t, found, "expected mount was never detached")
}
