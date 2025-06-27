// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
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

// getMountID returns the mount ID reported by the kernel for the mount which
// contains the provided path. It relies on the STATX_MNT_ID extension which is
// available on Linux ≥ 5.8.
func getMountID(path string) (uint32, error) {
	var stx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &stx); err != nil {
		return 0, err
	}
	return uint32(stx.Mnt_id), nil
}

func TestCopyTree(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `open.file.name == "test-open"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("copy-tree-test-recursive", func(t *testing.T) {
		// Mount the following directory struct in /tmp:
		// + /tmp/<somedir>/001        (tmpfs, 1MB)
		// |-- /tmp/<somedir>/001/tmp1 (tmpfs, 1MB)
		// |-- /tmp/<somedir>/001/tmp2 (tmpfs, 1MB)
		// In which `tmp1` and `tmp2` are have 001 as the parent mount
		// This is using the new mount api, but could have been accomplished with the mount() syscall too
		// because this isn't the part that we're testing
		var tounmount []string
		mountIdsToPath := make(map[uint32]string)

		dir := t.TempDir()
		tounmount = append(tounmount, dir)

		err := TmpMountAt(dir)
		if err != nil {
			t.Fatal(err)
		}

		if id, err := getMountID(dir); err != nil {
			t.Fatal(err)
		} else {
			mountIdsToPath[id] = "/"
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
				mountIdsToPath[id] = "/" + subdir
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

		seen := 0
		err = test.GetProbeEvent(func() error {
			unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE|unix.AT_RECURSIVE)
			return nil
		}, func(event *model.Event) bool {
			typeStr := event.GetType()
			if typeStr != "mount" {
				return false
			}

			assert.NotEqual(t, uint32(0), event.Mount.BindSrcMountID, "mount id is zero")
			assert.NotEmpty(t, event.GetMountMountpointPath(), "path is empty")
			assert.Equal(t, mountIdsToPath[event.Mount.BindSrcMountID], event.GetMountMountpointPath(), "Wrong Path")
			seen++
			return seen == 3
		}, 5*time.Second, model.FileMountEventType)

	})

	//TOOD: Create copy-tree-test-not-recursive

}
