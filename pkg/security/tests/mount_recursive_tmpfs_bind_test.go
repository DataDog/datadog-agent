// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"syscall"
	"testing"
)

// TestRecursiveTmpfsBindMount creates the following layout:
//
//	<tmpDir>/top-mount      <- tmpfs
//	    ├── sub1            <- tmpfs
//	    ├── sub2            <- tmpfs
//	    └── sub3            <- tmpfs
//
// It then bind-mounts <tmpDir>/top-mount on <tmpDir>/bind-target.
//
// No assertions are made – the purpose of this test is purely to exercise
// the kernel mount-tracking hooks with nested mounts and a subsequent bind.
func TestRecursiveTmpfsBindMount(t *testing.T) {
	SkipIfNotAvailable(t)

	// use the Go testing temporary directory as root for our mounts
	baseDir := t.TempDir()

	// 1. mount top-level tmpfs
	topPath := fmt.Sprintf("%s/top-mount", baseDir)
	if err := os.Mkdir(topPath, 0o755); err != nil {
		t.Fatalf("failed to mkdir %s: %v", topPath, err)
	}
	topMount := newTestMount(
		topPath,
		withSource("tmpfs"),
		withFSType("tmpfs"),
	)
	if err := topMount.mount(); err != nil {
		t.Fatalf("failed to mount top tmpfs: %v", err)
	}
	defer topMount.unmount(syscall.MNT_FORCE)

	// 2. inside the top tmpfs, create and mount three nested tmpfs mounts
	var subMounts []*testMount
	for i := 1; i <= 3; i++ {
		subDir := fmt.Sprintf("%s/sub%d", topPath, i)
		if err := os.Mkdir(subDir, 0o755); err != nil {
			t.Fatalf("failed to mkdir %s: %v", subDir, err)
		}
		m := newTestMount(
			subDir,
			withSource("tmpfs"),
			withFSType("tmpfs"),
		)
		if err := m.mount(); err != nil {
			t.Fatalf("failed to mount nested tmpfs %d: %v", i, err)
		}
		subMounts = append(subMounts, m)
	}
	// ensure nested tmpfs are unmounted when the test ends
	defer func() {
		for _, m := range subMounts {
			_ = m.unmount(syscall.MNT_FORCE)
		}
	}()

	// 3. bind-mount the whole subtree elsewhere
	bindTarget := fmt.Sprintf("%s/bind-target", baseDir)
	if err := os.Mkdir(bindTarget, 0o755); err != nil {
		t.Fatalf("failed to mkdir %s: %v", bindTarget, err)
	}
	bindMount := newTestMount(
		bindTarget,
		withSource(topPath),
		withFlags(syscall.MS_BIND),
	)
	if err := bindMount.mount(); err != nil {
		t.Fatalf("failed to bind-mount: %v", err)
	}
	defer bindMount.unmount(syscall.MNT_FORCE)
}
