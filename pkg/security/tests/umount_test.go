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

func TestUmount(t *testing.T) {
	//pause := func() {
	//	fmt.Println("Press Enter to continue...")
	//	bufio.NewReader(os.Stdin).ReadBytes('\n')
	//}

	SkipIfNotAvailable(t)

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mountDir := t.TempDir()
	fd, err := TmpMountAt(mountDir)
	if err != nil {
		t.Fatal(err)
	}
	mountID, err := getMountID(mountDir)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("Created new mount at", mountDir, mountID)

	found := false
	fmt.Println("START")
	err = test.GetProbeEvent(func() error {
		err = unix.Unmount(mountDir, syscall.MNT_DETACH)
		_ = unix.Close(fd)
		return nil
	}, func(event *model.Event) bool {
		// Check if mount id is correct
		fmt.Println("EVENT", event.GetType())
		if event.GetType() != "finalize_umount" {
			return false
		}
		fmt.Println("Found MountID", event.FinalizedUmount.MountID)
		if event.FinalizedUmount.MountID != mountID {
			found = true
			return false
		}

		return true
	}, 3*time.Second, model.FileFinalizedUmountEventType)
	fmt.Println("FINISH")
	assert.True(t, found, "expected file finalized umount event")
}
