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
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestPivotRoot(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("pivot_root test cannot run in Docker environment")
	}

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	tmpDir := t.TempDir()
	newRoot := filepath.Join(tmpDir, "newroot")
	putOld := filepath.Join(newRoot, "old")

	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := unix.Mount("tmpfs", newRoot, "tmpfs", 0, "size=1M"); err != nil {
		t.Fatalf("failed to mount tmpfs at new root: %v", err)
	}
	defer unix.Unmount(newRoot, unix.MNT_DETACH)

	if err := os.MkdirAll(putOld, 0o755); err != nil {
		t.Fatal(err)
	}

	// Drain pending events
	if err := test.GetProbeEvent(nil, func(_ *model.Event) bool { return false }, 1000*time.Millisecond); err != nil {
		if _, ok := err.(ErrTimeout); !ok {
			t.Fatal(err)
		}
	}

	t.Run("pivot-root-generates-pivot-root-events", func(t *testing.T) {
		var eventCount atomic.Int32

		err = test.GetProbeEvent(func() error {
			done := make(chan error, 1)
			go func() {
				runtime.LockOSThread()
				// pivot_root leaves the thread in a dirty state (different root fs),
				// so we intentionally skip UnlockOSThread on the success path.
				// Go runtime will terminate the OS thread when this goroutine exits.

				if err := unix.Unshare(unix.CLONE_NEWNS); err != nil {
					done <- fmt.Errorf("unshare: %w", err)
					runtime.UnlockOSThread()
					return
				}

				if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
					done <- fmt.Errorf("make mounts private: %w", err)
					runtime.UnlockOSThread()
					return
				}

				if err := unix.PivotRoot(newRoot, putOld); err != nil {
					done <- fmt.Errorf("pivot_root: %w", err)
					runtime.UnlockOSThread()
					return
				}

				unix.Unmount("/old", unix.MNT_DETACH)
				done <- nil
			}()
			return <-done
		}, func(event *model.Event) bool {
			if event.GetType() != "pivot_root" {
				return false
			}

			if event.ProcessContext.Pid != testSuitePid {
				return false
			}

			p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
			mount, _, _, err := p.Resolvers.MountResolver.ResolveMount(event.Mount.MountID, 0)
			if err == nil && mount != nil {
				t.Logf("pivot_root event: mount_id=%d path=%q fstype=%s",
					event.Mount.MountID, mount.Path, event.Mount.FSType)
			}

			assert.NotEqual(t, uint32(0), event.Mount.MountID, "mount ID should be non-zero")

			count := eventCount.Add(1)
			// pivot_root internally calls attach_mnt twice, producing 2 events
			return count >= 2
		}, 10*time.Second, model.PivotRootEventType)

		if err != nil {
			t.Error("timeout waiting for pivot_root events")
		}
		assert.GreaterOrEqual(t, eventCount.Load(), int32(2),
			"pivot_root should produce at least 2 pivot_root events")
	})
}
