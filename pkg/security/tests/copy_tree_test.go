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
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"os"
	"syscall"
	"testing"
	"time"

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

	t.Run("copy-tree-test", func(t *testing.T) {
		// Mount the following directory struct in /tmp:
		// + /tmp/<somedir>/001        (tmpfs, 1MB)
		// |-- /tmp/<somedir>/001/tmp1 (tmpfs, 1MB)
		// |-- /tmp/<somedir>/001/tmp2 (tmpfs, 1MB)
		// In which `tmp1` and `tmp2` are have 001 as the parent mount
		// This is using the new mount api, but could have been accomplished with the mount() syscall too
		// because this isn't the part that we're testing
		var tounmount []string

		dir := t.TempDir()
		tounmount = append(tounmount, dir)

		err := TmpMountAt(dir)
		if err != nil {
			t.Fatal(err)
		}

		mountSubDir := func(subdir string) {
			subdir = dir + "/" + subdir
			err = os.Mkdir(subdir, 0755)
			tounmount = append(tounmount, subdir)

			if err != nil {
				t.Fatal(err)
			}
			err = TmpMountAt(subdir)
			if err != nil {
				t.Fatal(err)
			}
		}

		mountSubDir("tmp1")
		mountSubDir("tmp2")

		defer func() {
			for i := len(tounmount) - 1; i >= 0; i-- {
				fmt.Println(tounmount[i])
				err = unix.Unmount(tounmount[i], syscall.MNT_DETACH)
				if err != nil {
					t.Fatal(err)
				}
			}
		}()

		var detachedMounts, mounts int
		err = test.GetProbeEvent(func() error {
			// Now we attempt to make a recursive copy of the entire tree that was created previously created
			unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE)
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() == "detached_mount" {
				detachedMounts++
			}

			if event.GetType() == "mount" {
				mounts++
			}

			fmt.Println("EVENT: ", event.Mount)
			//assert.NotEqual(t, event.Fsmount.MountID, uint32(0), "Mount id is zero")
			//assert.NotEqual(t, event.Fsmount.Flags, unix.FSOPEN_CLOEXEC, "Mount flags")
			//assert.NotEqual(t, event.Fsmount.MountAttrs, unix.FSMOUNT_CLOEXEC, "Wrong mount attributes")
			return detachedMounts == 2 && mounts == 1
		}, 3*time.Second, model.FileFsmountEventType)
	})

}
