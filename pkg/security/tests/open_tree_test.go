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
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/moby/sys/mountinfo"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func openTreeIsSupported() bool {
	_, _, errno := unix.Syscall6(unix.SYS_OPEN_TREE, 0, 0, 0, 0, 0, 0)
	return !errors.Is(errno, syscall.ENOSYS)
}

func TmpMountAt(dir string) (int, error) {
	openfd, err := unix.Fsopen("tmpfs", unix.FSOPEN_CLOEXEC)
	if err != nil {
		return 0, err
	}
	defer unix.Close(openfd)

	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "size", "1M", 0)
	_ = fsconfig(openfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)
	mountfd, err := unix.Fsmount(openfd, unix.FSMOUNT_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}

	if dir != "" {
		err = unix.MoveMount(mountfd, "", unix.AT_FDCWD, dir, unix.MOVE_MOUNT_F_EMPTY_PATH)
		if err != nil {
			unix.Close(mountfd)
			return 0, err
		}
	}

	return mountfd, nil
}

func TmpMountFdAt(fd int, at string) (int, error) {
	openfd, err := unix.Fsopen("tmpfs", unix.FSOPEN_CLOEXEC)
	if err != nil {
		return 0, err
	}

	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "size", "1M", 0)
	_ = fsconfig(openfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)
	mountfd, err := unix.Fsmount(openfd, unix.FSMOUNT_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}

	err = unix.MoveMount(mountfd, "", fd, at, unix.MOVE_MOUNT_F_EMPTY_PATH)
	if err != nil {
		return 0, err
	}

	return mountfd, nil
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

	if !openTreeIsSupported() {
		t.Skip("open_tree syscall is not supported on this platform")
	}

	execRules := []*rules.RuleDefinition{
		{
			ID:         "test_rule1",
			Expression: `exec.file.name == "true" && exec.file.mount_visible == false && exec.file.mount_detached == true`,
		},
		{
			ID:         "test_rule2",
			Expression: `exec.file.name == "false" && exec.file.mount_visible == true && exec.file.mount_detached == false`,
		},
	}

	mountRules := []*rules.RuleDefinition{
		{
			ID:         "test_rule3",
			Expression: `mount.detached == true && mount.visible == false`,
		},
	}
	test, err := newTestModule(t, nil, mountRules)
	if err != nil {
		t.Fatal(err)
	}

	// Mount the following directory struct in /tmp:
	// + /tmp/<tmpdir>        (tmpfs, 1MB)
	// |-- /tmp/<tmpdir>/tmp1 (tmpfs, 1MB)
	// |-- /tmp/<tmpdir>/tmp2 (tmpfs, 1MB)
	// In which `tmp1` and `tmp2` are have 001 as the parent mount
	// This is using the new mount api, but could have been accomplished with the mount() syscall too
	// because this isn't the part that we're testing
	var tounmount []string
	mountIDsToPath := make(map[uint32]string)

	dir := t.TempDir()
	tounmount = append(tounmount, dir)

	fdRoot, err := TmpMountAt(dir)
	if err != nil {
		// Syscall not available in this kernel
		t.Skip(err)
	}
	defer unix.Close(fdRoot)

	rootMountID, err := getMountID(dir)
	if err != nil {
		t.Fatal(err)
	}
	mountIDsToPath[rootMountID] = "/"

	mountSubDir := func(subdir string) {
		fullpath := dir + "/" + subdir
		err = os.Mkdir(fullpath, 0755)

		tounmount = append(tounmount, fullpath)

		if err != nil {
			t.Fatal(err)
		}
		fd, err := TmpMountAt(fullpath)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)

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

	type mountEvent struct {
		srcMountID uint32
		path       string
		detached   bool
		visible    bool
	}

	// GetProbeEvent sees every mount on the host, so scope the stream to the open_tree calls made below
	isOwnOpenTreeMount := func(event *model.Event) bool {
		return event.GetType() == "mount" &&
			event.Mount.Origin == model.MountOriginOpenTree &&
			event.ProcessContext.Pid == testSuitePid
	}

	t.Run("copy-tree-test-detached-recursive", func(t *testing.T) {
		var mu sync.Mutex
		seen := make(map[uint32]mountEvent)

		test.DrainProbeEvents()

		err = test.GetProbeEvent(func() error {
			fd, err := unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE|unix.AT_RECURSIVE)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			return nil
		}, func(event *model.Event) bool {
			if !isOwnOpenTreeMount(event) {
				return false
			}

			mu.Lock()
			defer mu.Unlock()
			seen[event.Mount.BindSrcMountID] = mountEvent{
				srcMountID: event.Mount.BindSrcMountID,
				path:       event.GetMountMountpointPath(),
				detached:   event.Mount.Detached,
				visible:    event.Mount.Visible,
			}

			return len(seen) == len(mountIDsToPath)
		}, 10*time.Second, model.FileMountEventType)

		mu.Lock()
		defer mu.Unlock()
		assert.Len(t, seen, len(mountIDsToPath), "wrong number of open_tree mounts (%v)", err)
		for srcMountID, srcPath := range mountIDsToPath {
			ev, ok := seen[srcMountID]
			if !assert.True(t, ok, "no open_tree mount event for %s", srcPath) {
				continue
			}
			assert.Equal(t, srcPath, ev.path, "Wrong Path")
			// only the root of the copy is detached from the VFS, its children stay attached to it
			assert.Equal(t, srcPath == "/", ev.detached, "Wrong detached state for %s", srcPath)
			assert.False(t, ev.visible, "%s shouldn't be visible", srcPath)
		}
	})

	t.Run("copy-tree-test-detached-non-recursive", func(t *testing.T) {
		var mu sync.Mutex
		var seen []mountEvent

		test.DrainProbeEvents()

		err = test.GetProbeEvent(func() error {
			fd, err := unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			return nil
		}, func(event *model.Event) bool {
			if !isOwnOpenTreeMount(event) {
				return false
			}

			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, mountEvent{
				srcMountID: event.Mount.BindSrcMountID,
				path:       event.GetMountMountpointPath(),
				detached:   event.Mount.Detached,
				visible:    event.Mount.Visible,
			})

			return true
		}, 10*time.Second, model.FileMountEventType)

		mu.Lock()
		defer mu.Unlock()
		if !assert.Len(t, seen, 1, "wrong number of open_tree mounts (%v)", err) {
			return
		}
		assert.Equal(t, rootMountID, seen[0].srcMountID, "wrong source mount")
		assert.Equal(t, "/", seen[0].path, "Wrong Path")
		assert.True(t, seen[0].detached, "Mount should be detached")
		assert.False(t, seen[0].visible, "Mount shouldn't be visible")
	})

	t.Run("detached-event-captured", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			fd, err := unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(fd)
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, true, event.Mount.Detached, "Mount should be detached")
			assert.Equal(t, false, event.Mount.Visible, "Mount shouldn't be visible")
		}, "test_rule3")
	})

	test.Close()
	test, err = newTestModule(t, nil, execRules)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("execution-from-detached-mount", func(t *testing.T) {
		srcPath := which(t, "true")
		fd, err := unix.OpenTree(0, dir, unix.OPEN_TREE_CLONE)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)

		destPath := fmt.Sprintf("/proc/%d/fd/%d/true", os.Getpid(), fd)
		if out, err := exec.Command("cp", srcPath, destPath).CombinedOutput(); err != nil {
			t.Fatalf("failed to copy %s to %s: %v (%s)", srcPath, destPath, err, out)
		}

		test.WaitSignalFromRule(t, func() error {
			if out, err := exec.Command(destPath).CombinedOutput(); err != nil {
				return fmt.Errorf("failed to run %s: %w (%s)", destPath, err, out)
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, true, event.Exec.FileEvent.MountDetached, "Mount should be detached")
			assert.Equal(t, false, event.Exec.FileEvent.MountVisible, "Mount shouldn't be visible")
		}, "test_rule1")
	})

	t.Run("execution-from-visible-mount", func(t *testing.T) {
		// not which: it resolves symlinks, and "false" points at the multi-call coreutils binary on uutils distros
		exePath, err := exec.LookPath("false")
		if err != nil {
			t.Fatal(err)
		}

		test.WaitSignalFromRule(t, func() error {
			// "false" is expected to exit non-zero, only a failure to start it is an error
			var exitErr *exec.ExitError
			if err := exec.Command(exePath).Run(); err != nil && !errors.As(err, &exitErr) {
				return fmt.Errorf("failed to run %s: %w", exePath, err)
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, false, event.Exec.FileEvent.MountDetached, "Mount should be detached")
			assert.Equal(t, true, event.Exec.FileEvent.MountVisible, "Mount shouldn't be visible")
		}, "test_rule2")
	})

}
