// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestUmount(t *testing.T) {
	pause := func() {
		fmt.Println("Press Enter to continue...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	SkipIfNotAvailable(t)

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mountDir := t.TempDir()
	_, err = TmpMountAt(mountDir)
	if err != nil {
		t.Fatal(err)
	}
	mountID, err := getMountID(mountDir)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Created new mount at", mountDir, mountID)
	fmt.Println("Will umount")
	pause()
	err = unix.Unmount(mountDir, syscall.MNT_DETACH)
	time.Sleep(10 * time.Second)
	//unix.Close(fd)
}
