// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"github.com/moby/sys/mountinfo"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TmpMountAt(dir string) error {
	openfd, err := unix.Fsopen("tmpfs", unix.FSOPEN_CLOEXEC)
	if err != nil {
		return err
	}

	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "size", "1M", 0)
	_ = fsconfig(openfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)
	mountfd, err := unix.Fsmount(openfd, unix.FSMOUNT_CLOEXEC, 0)
	if err != nil {
		return err
	}

	err = unix.MoveMount(mountfd, "", unix.AT_FDCWD, dir, unix.MOVE_MOUNT_F_EMPTY_PATH)
	if err != nil {
		return err
	}

	return nil
}

func getMountID(path string) (uint32, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return 0, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	mounts, err := mountinfo.GetMounts(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get mounts: %w", err)
	}

	for _, mnt := range mounts {
		mountDevice := unix.Mkdev(uint32(mnt.Major), uint32(mnt.Minor))
		if mountDevice == stat.Dev {
			return uint32(mnt.ID), nil
		}
	}

	return 0, fmt.Errorf("mount not found for path %s", path)
}

func TestOpenTree(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `mount.syscall.fs_type != "xxxxx"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// Mount the following directory struct in /tmp:
	// + /tmp/<somedir>/001        (tmpfs, 1MB)
	// |-- /tmp/<somedir>/001/tmp1 (tmpfs, 1MB)
	// |-- /tmp/<somedir>/001/tmp2 (tmpfs, 1MB)
	// In which `tmp1` and `tmp2` are have 001 as the parent mount
	// This is using the new mount api, but could have been accomplished with the mount() syscall too
	// because this isn't the part that we're testing
	var tounmount []string
	mountIDsToPath := make(map[uint32]string)

	dir := t.TempDir()
	tounmount = append(tounmount, dir)

	err = TmpMountAt(dir)
	if err != nil {
		// Syscall not available in this kernel
		t.Skip(err)
	}

	if id, err := getMountID(dir); err != nil {
		t.Fatal(err)
	} else {
		mountIDsToPath[id] = "/"
	}

	mountSubDir := func(subdir string) {
		fullpath := dir + "/" + subdir
		err = os.Mkdir(fullpath, 0755)

		tounmount = append(tounmount, fullpath)

		if err != nil {
			t.Fatal(err)
		}
		err = TmpMountAt(fullpath)
		if err != nil {
			t.Fatal(err)
		}

		if id, err := getMountID(fullpath); err != nil {
			t.Fatal(err)
		} else {
			mountIDsToPath[id] = "/" + subdir
		}
	}

	mountSubDir("tmp1")
	mountSubDir("tmp2")

	defer func() {
		for i := len(tounmount) - 1; i >= 0; i-- {
			err = unix.Unmount(tounmount[i], syscall.MNT_DETACH)
			if err != nil {
				t.Fatal(err)
			}
		}
	}()

	t.Run("copy-tree-test-detached-recursive", func(t *testing.T) {
		seen := 0

		err = test.GetProbeEvent(func() error {
			_, err = unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE|unix.AT_RECURSIVE)
			if err != nil {
				fmt.Printf("Err: %+v\n", err)
				t.Skip(err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "mount" || event.Mount.Source != 3 {
				return false
			}

			assert.Equal(t, model.MountEventSourceOpenTreeSyscall, event.Mount.Source, "Incorrect mount source")
			assert.NotEqual(t, uint32(0), event.Mount.BindSrcMountID, "mount id is zero")
			assert.NotEmpty(t, event.GetMountMountpointPath(), "path is empty")
			assert.Equal(t, mountIDsToPath[event.Mount.BindSrcMountID], event.GetMountMountpointPath(), "Wrong Path")

			seen++
			if seen == 1 {
				assert.Equal(t, true, event.Mount.Detached, "First mount should be detached")
				assert.Equal(t, false, event.Mount.Visible, "First mount shouldn't be visible")
			} else {
				assert.Equal(t, false, event.Mount.Detached, "Second and third mounts shouldn't be detached")
				assert.Equal(t, false, event.Mount.Visible, "Second and third mounts shouldn't be visible")
			}

			return seen == 3
		}, 10*time.Second, model.FileMountEventType)
		assert.Equal(t, 3, seen)
	})

	t.Run("copy-tree-test-detached-non-recursive", func(t *testing.T) {
		seen := 0
		err = test.GetProbeEvent(func() error {
			unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE)
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "mount" && event.Mount.Source != 3 {
				return false
			}
			seen++
			
			assert.Equal(t, model.MountEventSourceOpenTreeSyscall, event.Mount.Source, "Incorrect mount source")
			assert.NotEqual(t, uint32(0), event.Mount.BindSrcMountID, "mount id is zero")
			assert.NotEmpty(t, event.GetMountMountpointPath(), "path is empty")
			assert.Equal(t, "/", event.GetMountMountpointPath(), "Wrong Path")
			assert.Equal(t, true, event.Mount.Detached, "Mount should be detached")
			assert.Equal(t, false, event.Mount.Visible, "Mount shouldn't be visible")

			return seen == 1
		}, 10*time.Second, model.FileMountEventType)
		assert.Equal(t, 1, seen)
	})
}
