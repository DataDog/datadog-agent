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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/sys/unix"
)

func TestMoveMount(t *testing.T) {
	SkipIfNotAvailable(t)

	if !testutils.SyscallExists(unix.SYS_MOVE_MOUNT) {
		t.Skip("move_mount syscall is not supported on this platform")
	}

	mountDir := t.TempDir()
	fsmountfd := 0
	// Create a temporary mount
	fdTmp, err := TmpMountAt(mountDir)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fdTmp)

	// Make this mount private, so that the second mount doesn't propagate to all namespaces
	err = unix.Mount("", mountDir, "", unix.MS_REC|unix.MS_PRIVATE, "")
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
	defer unix.Close(openfd)

	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "source", "tmpfs", 0)
	_ = fsconfigStr(openfd, unix.FSCONFIG_SET_STRING, "size", "1M", 0)
	_ = fsconfig(openfd, unix.FSCONFIG_CMD_CREATE, nil, nil, 0)
	fsmountfd, err = unix.Fsmount(openfd, unix.FSMOUNT_CLOEXEC, 0)
	if err != nil {
		t.Fatal(fmt.Errorf("fsmount error %w", err))
	}
	defer unix.Close(fsmountfd)

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("move-detached-no-propagation", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			err = unix.MoveMount(fsmountfd, "", unix.AT_FDCWD, submountDir, unix.MOVE_MOUNT_F_EMPTY_PATH)
			if err != nil {
				t.Fatal("Could not move mount: ", err)
				return err
			}

			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "move_mount" || event.ProcessContext.Pid != testSuitePid {
				return false
			}
			p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
			mountPtr, _, _, err := p.Resolvers.MountResolver.ResolveMount(event.Mount.MountID, 0)
			assert.Equal(t, err, nil)
			assert.Equal(t, submountDir, mountPtr.Path, "Wrong mountpoint path")
			assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Namespace inode not captured")
			return true
		}, 10*time.Second, model.FileMoveMountEventType)

		if err != nil {
			t.Fatal("Test timeout without any event")
		}
	})

	if err := unix.Unmount(submountDir, unix.MNT_DETACH); err != nil {
		t.Logf("Failed to unmount %s: %v", submountDir, err)
	}
	if err := unix.Unmount(mountDir, unix.MNT_DETACH); err != nil {
		t.Logf("Failed to unmount %s: %v", mountDir, err)
	}
}

type MountEnvironment struct {
	fsmountfd      int
	tounmount      []string
	mountDir       string
	submountDirSrc string
	submountDirDst string

	// [do not merge] TestMoveMountRecursivePropagation flakiness debug: mount id -> path of every
	// mount the environment created, so stray setup events can be told apart from the moved tree.
	dbgSetupMountIDs map[uint32]string
}

func newTestEnvironment(private bool, mountDir string) (*MountEnvironment, error) {
	r := MountEnvironment{}

	// Prepare the source directory:
	r.mountDir = mountDir
	r.tounmount = []string{mountDir}

	fd, err := TmpMountAt(mountDir)
	if err != nil {
		return nil, err
	}
	unix.Close(fd)

	r.dbgSetupMountIDs = map[uint32]string{}
	if id, err := getMountID(mountDir); err == nil {
		r.dbgSetupMountIDs[id] = mountDir
	}

	if private {
		// Make this mount private, so that the second mount doesn't propagate to all namespaces
		err := unix.Mount("", mountDir, "", unix.MS_REC|unix.MS_PRIVATE, "")
		if err != nil {
			return nil, fmt.Errorf("could not mount: %w", err)
		}
	}

	r.submountDirSrc = mountDir + "/src"
	err = os.Mkdir(r.submountDirSrc, 0o777)
	if err != nil {
		return nil, fmt.Errorf("error making directory: %w", err)
	}

	r.submountDirDst = mountDir + "/dst"
	err = os.Mkdir(r.submountDirDst, 0o777)
	if err != nil {
		return nil, fmt.Errorf("error making directory: %w", err)
	}

	// Mount the following directory struct in /tmp:
	// + /tmp/<tmpdir>        (tmpfs, 1MB)
	// |-- /tmp/<tmpdir>/tmp1 (tmpfs, 1MB)
	// |-- /tmp/<tmpdir>/tmp2 (tmpfs, 1MB)
	// In which `tmp1` and `tmp2` are have 001 as the parent mount
	// This is using the new mount api, but could have been accomplished with the mount() syscall too
	// because this isn't the part that we're testing
	mountIDsToPath := make(map[uint32]string)

	r.tounmount = append(r.tounmount, r.submountDirSrc)

	r.fsmountfd, err = TmpMountAt(r.submountDirSrc)
	if err != nil {
		// Syscall not available in this kernel
		return nil, err
	}

	id, err := getMountID(r.submountDirSrc)
	if err != nil {
		return nil, err
	}

	mountIDsToPath[id] = "/"
	r.dbgSetupMountIDs[id] = r.submountDirSrc

	mountSubDir := func(subdir string) error {
		fullpath := r.submountDirSrc + "/" + subdir
		err = os.Mkdir(fullpath, 0755)

		r.tounmount = append(r.tounmount, fullpath)

		if err != nil {
			return err
		}
		fd2, err := TmpMountAt(fullpath)
		if err != nil {
			return err
		}
		unix.Close(fd2)

		id, err := getMountID(fullpath)
		if err != nil {
			return err
		}

		mountIDsToPath[id] = "/" + subdir
		r.dbgSetupMountIDs[id] = fullpath
		return nil
	}

	if err := mountSubDir("tmp1"); err != nil {
		return nil, err
	}
	if err := mountSubDir("tmp2"); err != nil {
		return nil, err
	}

	return &r, nil
}

func (r *MountEnvironment) UnmountAll() {
	for i := len(r.tounmount) - 1; i >= 0; i-- {
		unix.Unmount(r.tounmount[i], syscall.MNT_DETACH)
	}
	if r.fsmountfd > 0 {
		unix.Close(r.fsmountfd)
	}
}

func TestMoveMountRecursiveNoPropagation(t *testing.T) {
	SkipIfNotAvailable(t)

	if !testutils.SyscallExists(unix.SYS_MOVE_MOUNT) {
		t.Skip("move_mount syscall is not supported on this platform")
	}

	te, err := newTestEnvironment(true, t.TempDir())
	if err != nil {
		t.Fatal("Error creating new test environment", err)
	}
	defer te.UnmountAll()

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("moved-attached-recursive-no-propagation", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			err = unix.MoveMount(te.fsmountfd, "", unix.AT_FDCWD, te.submountDirDst, unix.MOVE_MOUNT_F_EMPTY_PATH)
			if err == nil {
				for i := 0; i != len(te.tounmount); i++ {
					te.tounmount[i] = strings.Replace(te.tounmount[i], te.submountDirSrc, te.submountDirDst, 1)
				}
			}
			return nil
		}, func(event *model.Event) bool {
			if event.GetType() != "move_mount" {
				return false
			}
			p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
			mount, _, _, err := p.Resolvers.MountResolver.ResolveMount(event.Mount.MountID, 0)
			assert.Equal(t, err, nil, "Error resolving mount")
			assert.Equal(t, 2, len(mount.Children), "Wrong number of child mounts")
			assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Namespace inode not captured")

			for _, childMountID := range mount.Children {
				child, _, _, err := p.Resolvers.MountResolver.ResolveMount(childMountID, 0)
				assert.Equal(t, err, nil, "Error resolving child mount")
				assert.True(t, strings.HasPrefix(child.Path, te.submountDirDst), "Path wasn't updated")
			}

			return true
		}, 10*time.Second, model.FileMoveMountEventType)

		if err != nil {
			t.Fatal("Test timeout without any event")
		}
	})
}

func TestMoveMountRecursivePropagation(t *testing.T) {
	SkipIfNotAvailable(t)

	// Docker messes up with the propagation
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test in Docker environment")
	}

	if !testutils.SyscallExists(unix.SYS_MOVE_MOUNT) {
		t.Skip("move_mount syscall is not supported on this platform")
	}

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	t.Run("moved-recursive-with-propagation", func(t *testing.T) {
		dbgMoveMountRecursiveOnce(t, test, "test", true)
	})
}

// [do not merge] moveMountDbgResult is what one run of the moved-recursive-with-propagation
// sequence saw.
type moveMountDbgResult struct {
	failed        bool // the original assertion would have failed
	accepted      int  // move_mount tmpfs events the callback accepted
	straySetup    int  // accepted events whose mount id is one of the environment's setup mounts
	strayBefore   int  // accepted events timestamped before the MoveMount call
	drained       int  // events consumed by the drain
	drainedMove   int  // move_mount events consumed by the drain
	drainDuration time.Duration
}

func dbgMonotonicNs() uint64 {
	var ts unix.Timespec
	_ = unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts)
	return uint64(ts.Nano())
}

// dbgDrainProbeEvents is DrainProbeEvents (same 250ms quiet period / 1s cap) that also reports
// what it consumed.
func dbgDrainProbeEvents(test *testModule) (total, moveMounts int, took time.Duration) {
	const (
		drainQuietPeriod = 250 * time.Millisecond
		drainMaxWait     = time.Second
	)
	var mu sync.Mutex
	seen := make(chan struct{}, 1)
	test.RegisterProbeEventHandler(func(e *model.Event) {
		mu.Lock()
		total++
		if e.GetEventType() == model.FileMoveMountEventType {
			moveMounts++
		}
		mu.Unlock()
		select {
		case seen <- struct{}{}:
		default:
		}
	})
	defer test.RegisterProbeEventHandler(nil)

	begin := time.Now()
	deadline := time.After(drainMaxWait)
loop:
	for {
		select {
		case <-seen:
		case <-time.After(drainQuietPeriod):
			break loop
		case <-deadline:
			break loop
		}
	}
	mu.Lock()
	defer mu.Unlock()
	return total, moveMounts, time.Since(begin)
}

// dbgMoveMountRecursiveOnce runs the moved-recursive-with-propagation sequence. With assertions,
// it behaves like the original subtest; without, it only reports whether it would have failed.
func dbgMoveMountRecursiveOnce(t *testing.T, test *testModule, label string, assertions bool) moveMountDbgResult {
	var res moveMountDbgResult

	// allMounts is written from the probe event handler goroutine and read
	// from the test goroutine after GetProbeEvent returns. GetProbeEvent
	// does not wait for in-flight callbacks on timeout, so we need our own
	// synchronization to avoid a data race.
	var allMountsMu sync.Mutex
	allMounts := map[uint32]uint32{}
	var accepted []string

	te, err := newTestEnvironment(false, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer te.UnmountAll()

	fd, err := unix.OpenTree(0, te.submountDirSrc, unix.OPEN_TREE_CLONE|unix.AT_RECURSIVE)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)

	res.drained, res.drainedMove, res.drainDuration = dbgDrainProbeEvents(test)

	var moveStart, moveEnd uint64
	err = test.GetProbeEvent(func() error {
		moveStart = dbgMonotonicNs()
		err = unix.MoveMount(fd, "", unix.AT_FDCWD, te.submountDirDst, unix.MOVE_MOUNT_F_EMPTY_PATH)
		moveEnd = dbgMonotonicNs()

		if err != nil {
			t.Fatal("Err moving mount:", err)
		}
		if err == nil {
			for i := 0; i != len(te.tounmount); i++ {
				te.tounmount[i] = strings.Replace(te.tounmount[i], te.submountDirSrc, te.submountDirDst, 1)
			}
		}
		return nil
	}, func(event *model.Event) bool {
		if event.GetType() != "move_mount" || event.Mount.FSType != "tmpfs" {
			return false
		}
		if assertions {
			assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Namespace inode not captured")
		}
		setupPath, isSetup := te.dbgSetupMountIDs[event.Mount.MountID]
		before := event.TimestampRaw < moveStart
		allMountsMu.Lock()
		allMounts[event.Mount.MountID]++
		res.accepted++
		if isSetup {
			res.straySetup++
		}
		if before {
			res.strayBefore++
		}
		accepted = append(accepted, fmt.Sprintf("{mount_id=%d pid=%d ts=%d ts-move_start=%dns before_move=%v setup_mount=%v setup_path=%q mountpoint=%q}",
			event.Mount.MountID, event.ProcessContext.Pid, event.TimestampRaw, int64(event.TimestampRaw)-int64(moveStart), before, isSetup, setupPath, event.Mount.MountPointStr))
		allMountsMu.Unlock()
		return false
	}, 5*time.Second, model.FileMoveMountEventType)

	allMountsMu.Lock()
	if assertions {
		assert.GreaterOrEqual(t, len(allMounts), 3, "Not all mount events were obtained")
	}
	if len(allMounts) < 3 {
		res.failed = true
	}
	mountIDs := make([]uint32, 0, len(allMounts))
	for id := range allMounts {
		mountIDs = append(mountIDs, id)
	}
	allMountsMu.Unlock()

	var failing []string
	p, _ := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	for _, i := range mountIDs {
		path, _, _, _ := p.Resolvers.MountResolver.ResolveMountPath(i, 0)

		if len(path) == 0 || !strings.Contains(path, "tmp1") && !strings.Contains(path, "tmp2") {
			// Some paths aren't being fully resolved due to missing mounts in the chain
			// Need to figure out what are these mount points and why they aren't to be found anywhere
			continue
		}

		if !strings.Contains(path, te.submountDirDst) {
			res.failed = true
			_, isSetup := te.dbgSetupMountIDs[i]
			failing = append(failing, fmt.Sprintf("{mount_id=%d path=%q setup_mount=%v}", i, path, isSetup))
		}
		if assertions {
			assert.True(t, strings.Contains(path, te.submountDirDst), fmt.Sprintf("Path %s wasn't moved. Destination=%s", path, te.submountDirDst))
		}
	}

	if res.failed || assertions {
		t.Logf("MOVEMOUNTDBG %s: failed=%v accepted=%d stray_setup=%d stray_before_move=%d drained=%d drained_move_mount=%d drain_took=%s move_start=%d move_took=%dns setup_ids=%v failing=%v accepted_events=%v",
			label, res.failed, res.accepted, res.straySetup, res.strayBefore, res.drained, res.drainedMove, res.drainDuration,
			moveStart, moveEnd-moveStart, te.dbgSetupMountIDs, failing, accepted)
	}
	return res
}

// [do not merge] TestMoveMountRecursivePropagationStress loops the sequence to measure how often it
// would fail and whether every failure is explained by stray setup events.
func TestMoveMountRecursivePropagationStress(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test in Docker environment")
	}
	if !testutils.SyscallExists(unix.SYS_MOVE_MOUNT) {
		t.Skip("move_mount syscall is not supported on this platform")
	}

	test, err := newTestModule(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	const iterations = 300
	var failed, failedExplained, withStraySetup, withStrayBefore int
	for i := 0; i < iterations; i++ {
		res := dbgMoveMountRecursiveOnce(t, test, fmt.Sprintf("stress iter=%d", i), false)
		if res.straySetup > 0 {
			withStraySetup++
		}
		if res.strayBefore > 0 {
			withStrayBefore++
		}
		if res.failed {
			failed++
			if res.straySetup > 0 {
				failedExplained++
			}
		}
	}
	t.Logf("MOVEMOUNTDBG stress summary: iterations=%d failed=%d failed_with_stray_setup=%d iterations_with_stray_setup=%d iterations_with_stray_before_move=%d",
		iterations, failed, failedExplained, withStraySetup, withStrayBefore)
}
