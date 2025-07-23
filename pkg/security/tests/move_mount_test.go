// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/sys/unix"
)

func moveMountIsSupported() bool {
	_, _, errno := unix.Syscall6(unix.SYS_MOVE_MOUNT, 0, 0, 0, 0, 0, 0)
	return !errors.Is(errno, syscall.ENOSYS)
}

func GetMountID(fd int) (uint64, error) {
	var stx unix.Statx_t

	flags := unix.AT_EMPTY_PATH | unix.AT_STATX_DONT_SYNC

	if err := unix.Statx(fd, "", flags, unix.STATX_MNT_ID, &stx); err != nil {
		return 0, fmt.Errorf("statx: %w", err)
	}

	if stx.Mask&unix.STATX_MNT_ID == 0 {
		return 0, fmt.Errorf("statx: kernel não preencheu STATX_MNT_ID — kernel < 5.8?")
	}

	return stx.Mnt_id, nil
}

func TestMoveMount(t *testing.T) {
	SkipIfNotAvailable(t)

	if !moveMountIsSupported() {
		t.Skip("move_mount syscall is not supported on this platform")
	}

	mountDir := t.TempDir()
	fsmountfd := 0
	var mountid uint64
	// Create a temporary mount
	TmpMountAt(mountDir)

	// Make this mount private, so that the second mount doesn't propagate to all namespaces
	err := unix.Mount("", mountDir, "", unix.MS_REC|unix.MS_PRIVATE, "")
	if err != nil {
		t.Fatal(err)
	}

	submountDir := mountDir + "/subdir"
	err = os.Mkdir(submountDir, 0o777)
	if err != nil {
		t.Fatal(fmt.Errorf("error making directory: %w", err))
	}

	openfd, err := unix.Fsopen("tmpfs", unix.FSOPEN_CLOEXEC)
	if err != nil {
		t.Fatal("fsopen error %w", err)
	}

	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "size", "1M", 0)
	_ = fsconfig(openfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)
	fsmountfd, err = unix.Fsmount(openfd, unix.FSMOUNT_CLOEXEC, 0)
	if err != nil {
		t.Fatal(fmt.Errorf("fsmount error %w", err))
	}

	mountid, _ = GetMountID(fsmountfd)

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("move-detached-no-propagation", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			err = unix.MoveMount(fsmountfd, "", unix.AT_FDCWD, submountDir, unix.MOVE_MOUNT_F_EMPTY_PATH)
			if err != nil {
				t.Fatal("Could not move mount: ", err)
				return err
			}

			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "move_mount" && event.Mount.MountID != uint32(mountid) {
				return false
			}
			p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
			mountPtr, _, _, err := p.Resolvers.MountResolver.ResolveMount(event.Mount.MountID, 0, 0, "")
			assert.Equal(t, err, nil)
			assert.Equal(t, submountDir, mountPtr.MountPointStr, "Wrong mountpoint path")
			return true
		}, 10*time.Second, model.FileMoveMountEventType)

		if err != nil {
			t.Fatal("Test timeout without any event")
		}
	})

	_ = unix.Unmount(submountDir, unix.MNT_FORCE|unix.MNT_DETACH)
	_ = unix.Unmount(mountDir, unix.MNT_FORCE|unix.MNT_DETACH)
}

func TestMoveMountRecursive(t *testing.T) {
	// Prepare the source directory:
	mountDir := t.TempDir()

	TmpMountAt(mountDir)
	// Make this mount private, so that the second mount doesn't propagate to all namespaces
	err := unix.Mount("", mountDir, "", unix.MS_REC|unix.MS_PRIVATE, "")
	tounmount := []string{mountDir}
	if err != nil {
		t.Fatal("Could not mount: ", err)
		return
	}

	submountDirSrc := mountDir + "/src"
	err = os.Mkdir(submountDirSrc, 0o777)
	if err != nil {
		t.Fatal("Error making directory:", err)
	}

	submountDirDst := mountDir + "/dst"
	err = os.Mkdir(submountDirDst, 0o777)
	if err != nil {
		t.Fatal("Error making directory:", err)
	}

	// Mount the following directory struct in /tmp:
	// + /tmp/<tmpdir>        (tmpfs, 1MB)
	// |-- /tmp/<tmpdir>/tmp1 (tmpfs, 1MB)
	// |-- /tmp/<tmpdir>/tmp2 (tmpfs, 1MB)
	// In which `tmp1` and `tmp2` are have 001 as the parent mount
	// This is using the new mount api, but could have been accomplished with the mount() syscall too
	// because this isn't the part that we're testing
	mountIDsToPath := make(map[uint32]string)

	tounmount = append(tounmount, submountDirSrc)

	fsmountfd, err := TmpMountAt(submountDirSrc)
	if err != nil {
		// Syscall not available in this kernel
		t.Skip(err)
	}

	if id, err := getMountID(submountDirSrc); err != nil {
		t.Fatal(err)
	} else {
		mountIDsToPath[id] = "/"
	}

	mountSubDir := func(subdir string) {
		fullpath := submountDirSrc + "/" + subdir
		err = os.Mkdir(fullpath, 0755)

		tounmount = append(tounmount, fullpath)

		if err != nil {
			t.Fatal(err)
		}
		_, err = TmpMountAt(fullpath)
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

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		for i := len(tounmount) - 1; i >= 0; i-- {
			err = unix.Unmount(tounmount[i], syscall.MNT_DETACH)
			if err != nil {
				t.Fatal(err)
			}
		}
	}()

	t.Run("moved-attached-recursive-no-propagation", func(_ *testing.T) {
		err = test.GetProbeEvent(func() error {
			err = unix.MoveMount(fsmountfd, "", unix.AT_FDCWD, submountDirDst, unix.MOVE_MOUNT_F_EMPTY_PATH)
			if err == nil {
				for i := 0; i != len(tounmount); i++ {
					tounmount[i] = strings.Replace(tounmount[i], submountDirSrc, submountDirDst, 1)
				}
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "move_mount" {
				return false
			}
			p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
			mount, _, _, err := p.Resolvers.MountResolver.ResolveMount(event.Mount.MountID, 0, 0, "")
			assert.Equal(t, err, nil, "Error resolving mount")
			assert.Equal(t, len(mount.Children), 2, "Wrong number of child mounts")

			for _, childMountID := range mount.Children {
				child, _, _, err := p.Resolvers.MountResolver.ResolveMount(childMountID, 0, 0, "")
				assert.Equal(t, err, nil, "Error resolving child mount")
				assert.True(t, strings.HasPrefix(child.MountPointStr, submountDirDst), "Path wasn't updated")
			}

			return true
		}, 10*time.Second, model.FileMoveMountEventType)

		if err != nil {
			t.Fatal("Test timeout without any event")
		}
	})

}

// Create a test that will create a mount point with several submounts
// Then use open_tree to create a recursive clone to get a detached mount with several submounts
// Then use move_mount on that

//t.Run("move-attached-fd", func(t *testing.T) {
//})
//
//t.Run("move-attached-filename", func(t *testing.T) {
//})
//
//t.Run("move-attached-fd-recursive", func(t *testing.T) {
//})
