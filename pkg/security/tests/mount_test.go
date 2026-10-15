// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/moby/sys/mountinfo"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestMount(t *testing.T) {
	SkipIfNotAvailable(t)

	dstMntBasename := "test-dest-mount"

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`chmod.file.path == "{{.Root}}/%s/test-mount"`, dstMntBasename),
	}, {
		ID:         "test_rule_pending",
		Expression: fmt.Sprintf(`chown.file.path == "{{.Root}}/%s/test-release"`, dstMntBasename),
	}}

	testDrive, err := newTestDrive(t, "xfs", []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	mntPath := testDrive.Path("test-mount")
	os.MkdirAll(mntPath, 0755)
	defer os.RemoveAll(mntPath)

	dstMntPath := testDrive.Path(dstMntBasename)
	os.MkdirAll(dstMntPath, 0755)
	defer os.RemoveAll(dstMntPath)

	dumpSyscallsCache(t, test, "TestMount start") // [do not merge]

	var mntID atomic.Uint32
	t.Run("mount", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			if err := syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, ""); err != nil {
				return fmt.Errorf("could not create bind mount: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.ProcessContext.Pid != testSuitePid {
				return false
			}

			mntID.Store(event.Mount.MountID)
			t.Logf("UMOUNTDBG test mount event: mount_id=%d mountpoint=%s fstype=%s ts=%d", event.Mount.MountID, event.Mount.MountPointStr, event.Mount.GetFSType(), event.TimestampRaw) // [do not merge]
			if !ebpfLessEnabled {
				assert.Equal(t, false, event.Mount.Detached, "Mount should not be detached")
				assert.Equal(t, true, event.Mount.Visible, "Mount should be visible")
				assert.Equal(t, model.MountOriginEvent, event.Mount.Origin, "Incorrect mount source")
				assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Mount namespace inode not captured")
			}

			return assert.Equal(t, "/"+dstMntBasename, event.Mount.MountPointStr, "wrong mount point") &&
				assert.Equal(t, "xfs", event.Mount.GetFSType(), "wrong mount fs type")
		}, 3*time.Second, model.FileMountEventType)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("mount_resolver", func(t *testing.T) {
		file := testDrive.Path(dstMntBasename, "test-mount")

		f, err := os.Create(file)
		if err != nil {
			t.Fatal(err)
		}

		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(file)

		test.WaitSignalFromRule(t, func() error {
			return os.Chmod(file, 0707)
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, file, event.Chmod.File.PathnameStr, "wrong path")
		}, "test_rule")
	})

	releaseFile, err := os.Create(path.Join(dstMntPath, "test-release"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFile.Close()

	t.Run("umount", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			// Test umount
			if err = syscall.Unmount(dstMntPath, syscall.MNT_DETACH); err != nil {
				return fmt.Errorf("could not unmount test-mount: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			if event.ProcessContext.Pid != testSuitePid {
				return false
			}

			// [do not merge] umount flakiness debug
			t.Logf("UMOUNTDBG test umount event: expected_mount_id=%d %s", mntID.Load(), umountDbgString(event))

			return ebpfLessEnabled || assert.Equal(t, mntID.Load(), event.Umount.MountID, "wrong mount id")
		}, 3*time.Second, model.FileUmountEventType)
		if err != nil {
			t.Error(err)
		}
		if t.Failed() {
			dumpSyscallsCache(t, test, "TestMount/umount failed") // [do not merge]
		}
	})

	t.Run("release-mount", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return syscall.Fchownat(int(releaseFile.Fd()), "", 123, 123, unix.AT_EMPTY_PATH)
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, rule, "test_rule_pending")
		}, "test_rule_pending")
	})
}

// [do not merge] umount flakiness debug
func umountDbgString(event *model.Event) string {
	u := &event.Umount
	return fmt.Sprintf("pid=%d tid=%d ts=%d retval=%d mount_id=%d entry_mount_id=%d entry_read_ret=%d exit_read_ret=%d prefix_mount_id=%d exit_mnt_flags=%#x vfs=%#x exit-entry=%dns prog_type=%d owner_after=%#x type_after=%d vfs_after=%#x",
		event.PIDContext.Pid, u.DbgTid, event.TimestampRaw, u.Retval, u.MountID, u.DbgEntryMountID, u.DbgEntryReadRet, u.DbgExitReadRet,
		u.DbgExitMountIDReread, u.DbgExitMntFlags, u.DbgVfs, int64(u.DbgExitTs)-int64(u.DbgEntryTs), u.DbgProgType,
		u.DbgOwnerAfter, u.DbgTypeAfter, u.DbgVfsAfter)
}

// [do not merge] dumpSyscallsCache logs the occupancy of the per-thread syscall cache map:
// total entries, entries per cached event type, and entries whose thread no longer exists
// (leaked: their exit hook never popped them).
func dumpSyscallsCache(t *testing.T, test *testModule, where string) {
	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		return
	}
	m, _, err := p.Manager.Get().GetMap("syscalls")
	if err != nil {
		t.Logf("UMOUNTDBG syscalls-cache %s: failed to get map: %v", where, err)
		return
	}
	info, _ := m.Info()
	if m.Type() != ebpf.LRUHash && m.Type() != ebpf.Hash {
		t.Logf("UMOUNTDBG syscalls-cache %s: map type=%s (task storage), not iterable by pid_tgid", where, m.Type())
		return
	}
	var (
		key   uint64
		value []byte
		total int
		dead  int
	)
	byType := map[uint64]int{}
	deadByType := map[uint64]int{}
	it := m.Iterate()
	for it.Next(&key, &value) {
		total++
		var typ uint64
		if len(value) >= 16 {
			typ = binary.NativeEndian.Uint64(value[8:16])
		}
		byType[typ]++
		tgid, tid := key>>32, key&0xffffffff
		if _, err := os.Stat(fmt.Sprintf("/proc/%d/task/%d", tgid, tid)); err != nil {
			dead++
			deadByType[typ]++
		}
	}
	if err := it.Err(); err != nil {
		t.Logf("UMOUNTDBG syscalls-cache %s: iterate error: %v", where, err)
	}
	name := ""
	if info != nil {
		name = info.Name
	}
	t.Logf("UMOUNTDBG syscalls-cache %s: map=%s type=%s max_entries=%d total=%d dead_threads=%d by_type=%v dead_by_type=%v", where, name, m.Type(), m.MaxEntries(), total, dead, byType, deadByType)
}

// [do not merge] TestMountUmountStress loops the TestMount mount/umount sequence to raise the
// hit rate of the flaky "umount event carries mount_id 0" failure, and logs every anomaly.
// Each variant adds one ingredient of TestMount that a plain mount/umount loop lacks, so
// the variant that reproduces names the trigger.
func TestMountUmountStress(t *testing.T) {
	SkipIfNotAvailable(t)
	if ebpfLessEnabled {
		t.Skip("eBPF only")
	}

	dstMntBasename := "test-dest-mount"

	// same rules as TestMount
	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`chmod.file.path == "{{.Root}}/%s/test-mount"`, dstMntBasename),
	}, {
		ID:         "test_rule_pending",
		Expression: fmt.Sprintf(`chown.file.path == "{{.Root}}/%s/test-release"`, dstMntBasename),
	}}

	testDrive, err := newTestDrive(t, "xfs", []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	mntPath := testDrive.Path("test-mount")
	os.MkdirAll(mntPath, 0755)
	defer os.RemoveAll(mntPath)

	dstMntPath := testDrive.Path(dstMntBasename)
	os.MkdirAll(dstMntPath, 0755)
	defer os.RemoveAll(dstMntPath)

	dumpSyscallsCache(t, test, "stress start")
	defer dumpSyscallsCache(t, test, "stress end")

	type variant struct {
		name        string
		chmodBefore bool // TestMount/mount_resolver: chmod through the bind mount, rule match
		holdOpen    bool // releaseFile: a file on the bind mount stays open across the umount
		chownAfter  bool // TestMount/release-mount: fchown the open file once detached, pending rule
		churn       bool // other threads keep entering cached syscalls while the umount runs
		iterations  int
	}
	// iterations 1-3 proved every variant clean with the fixed (peek, read, pop) order; the
	// churn variants check whether the pre-fix (pop, read) order, replayed in eBPF as
	// prefix_mount_id, reports a wrong mount id once other tasks recycle syscall cache entries.
	variants := []variant{
		{name: "plain", iterations: 1000},
		{name: "plain-churn", churn: true, iterations: 5000},
		{name: "full-churn", chmodBefore: true, holdOpen: true, chownAfter: true, churn: true, iterations: 3000},
	}

	churnFile := testDrive.Path("churn-file")
	if f, err := os.Create(churnFile); err == nil {
		f.Close()
	}
	defer os.Remove(churnFile)
	churnDir := testDrive.Path("churn-dir")

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			iterations := v.iterations
			if v.churn {
				stop := make(chan struct{})
				var wg sync.WaitGroup
				for w := 0; w < 2*runtime.NumCPU(); w++ {
					wg.Add(1)
					go func(w int) {
						defer wg.Done()
						runtime.LockOSThread()
						defer runtime.UnlockOSThread()
						dir := fmt.Sprintf("%s-%d", churnDir, w)
						for n := 0; ; n++ {
							select {
							case <-stop:
								return
							default:
							}
							// each of these caches a syscall_cache_t entry on entry and pops it on exit
							_ = os.Chmod(churnFile, os.FileMode(0600|(n&1)<<2))
							_ = os.Chown(churnFile, 0, 0)
							_ = unix.Utimes(churnFile, []unix.Timeval{{Sec: int64(n)}, {Sec: int64(n)}})
							_ = os.Mkdir(dir, 0755)
							_ = os.Remove(dir)
							if f, err := os.Open(churnFile); err == nil {
								f.Close()
							}
						}
					}(w)
				}
				defer func() {
					close(stop)
					wg.Wait()
				}()
			}

			dumped, prefixDumped := 0, 0
			var mountTimeouts, umountTimeouts, mismatches, ok int
			var prefixWrong, prefixZero, prefixReclaimed int
			for i := 0; i < iterations; i++ {
				var mntIDAtomic atomic.Uint32
				err := test.GetProbeEvent(func() error {
					return syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, "")
				}, func(event *model.Event) bool {
					if event.ProcessContext.Pid != testSuitePid || event.Mount.MountPointStr != "/"+dstMntBasename {
						return false
					}
					mntIDAtomic.Store(event.Mount.MountID)
					return true
				}, 3*time.Second, model.FileMountEventType)
				if err != nil {
					mountTimeouts++
					t.Logf("UMOUNTDBG variant=%s iter=%d mount event not received: %v", v.name, i, err)
				}
				mntID := mntIDAtomic.Load()

				if v.chmodBefore {
					file := testDrive.Path(dstMntBasename, "test-mount")
					if f, err := os.Create(file); err == nil {
						f.Close()
						test.WaitSignalFromRule(t, func() error {
							return os.Chmod(file, 0707)
						}, func(_ *model.Event, _ *rules.Rule) {}, "test_rule")
						os.Remove(file)
					}
				}

				var releaseFile *os.File
				if v.holdOpen {
					releaseFile, err = os.Create(path.Join(dstMntPath, "test-release"))
					if err != nil {
						t.Fatal(err)
					}
				}

				var (
					lock     sync.Mutex
					received []string
					matched  bool
				)
				err = test.GetProbeEvent(func() error {
					return syscall.Unmount(dstMntPath, syscall.MNT_DETACH)
				}, func(event *model.Event) bool {
					if event.ProcessContext.Pid != testSuitePid {
						return false
					}
					lock.Lock()
					defer lock.Unlock()
					received = append(received, umountDbgString(event))
					if mntID != 0 && event.Umount.MountID == mntID {
						u := &event.Umount
						reclaimed := u.DbgTypeAfter != uint64(model.FileUmountEventType) || u.DbgVfsAfter != u.DbgVfs || u.DbgOwnerAfter != uint64(testSuitePid)<<32|uint64(u.DbgTid)
						if reclaimed {
							prefixReclaimed++
						}
						if u.DbgExitMountIDReread != mntID {
							prefixWrong++
							if u.DbgExitMountIDReread == 0 {
								prefixZero++
							}
						}
						if (reclaimed || u.DbgExitMountIDReread != mntID) && prefixDumped < 20 {
							prefixDumped++
							t.Logf("UMOUNTDBG variant=%s iter=%d PREFIX expected_mount_id=%d reclaimed=%v %s", v.name, i, mntID, reclaimed, umountDbgString(event))
						}
						matched = true
						return true
					}
					return false
				}, 3*time.Second, model.FileUmountEventType)
				lock.Lock()
				switch {
				case matched:
					ok++
				case len(received) > 0:
					mismatches++
					t.Logf("UMOUNTDBG variant=%s iter=%d MISMATCH expected_mount_id=%d received=%v", v.name, i, mntID, received)
					if dumped < 5 {
						dumped++
						dumpSyscallsCache(t, test, fmt.Sprintf("variant %s iter %d mismatch", v.name, i))
					}
				default:
					umountTimeouts++
					t.Logf("UMOUNTDBG variant=%s iter=%d umount event not received (expected_mount_id=%d): %v", v.name, i, mntID, err)
					if dumped < 5 {
						dumped++
						dumpSyscallsCache(t, test, fmt.Sprintf("variant %s iter %d timeout", v.name, i))
					}
				}
				lock.Unlock()

				if releaseFile != nil {
					if v.chownAfter {
						test.WaitSignalFromRule(t, func() error {
							return syscall.Fchownat(int(releaseFile.Fd()), "", 123, 123, unix.AT_EMPTY_PATH)
						}, func(_ *model.Event, _ *rules.Rule) {}, "test_rule_pending")
					}
					releaseFile.Close()
					os.Remove(path.Join(mntPath, "test-release"))
				}
			}

			t.Logf("UMOUNTDBG summary: variant=%s iterations=%d ok=%d mismatches=%d umount_timeouts=%d mount_timeouts=%d", v.name, iterations, ok, mismatches, umountTimeouts, mountTimeouts)
			t.Logf("UMOUNTDBG prefix-summary: variant=%s ok=%d entry_reclaimed=%d prefix_wrong_mount_id=%d prefix_zero_mount_id=%d", v.name, ok, prefixReclaimed, prefixWrong, prefixZero)
			if ok != iterations {
				t.Errorf("UMOUNTDBG summary: variant=%s iterations=%d ok=%d mismatches=%d umount_timeouts=%d mount_timeouts=%d", v.name, iterations, ok, mismatches, umountTimeouts, mountTimeouts)
			}
		})
	}
}

// withForceReload() at the newTestModule call is load-bearing: this test and
// the two TestMountSnapshot* below share the inline-config run, so without it
// they would reuse each other's module and snapshot the wrong mounts.
var _ = declareInlineConfig(TestMountPropagated)

func TestMountPropagated(t *testing.T) {
	SkipIfNotAvailable(t)

	// - testroot
	// 		/ dir1
	// 			/ test-drive (xfs mount)
	// 		/ dir-bind-mounted (bind mount of testroot/dir1)
	// 			/ test-drive (propagated)
	//				/ test-file

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: `chmod.file.path == "{{.Root}}/dir1-bind-mounted/test-drive/test-file"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs, withForceReload())
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	dir1Path, _, err := test.Path("dir1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1Path)

	testDrivePath := path.Join(dir1Path, "test-drive")
	if err := os.MkdirAll(testDrivePath, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDrivePath)

	testDrive, err := newTestDrive(t, "xfs", []string{}, testDrivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if testEnvironment == DockerEnvironment {
			testDrive.Close()
			return
		}

		if err := testDrive.DetachDevice(); err != nil {
			fmt.Printf("failed to detach device: %v", err)
		}
	}()

	dir1BindMntPath, _, err := test.Path("dir1-bind-mounted")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir1BindMntPath, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir1BindMntPath)

	bindMnt := newTestMount(
		dir1BindMntPath,
		withSource(dir1Path),
		withFlags(syscall.MS_BIND|syscall.MS_REC),
	)

	if err := bindMnt.mount(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		testPropagatedDrivePath := path.Join(dir1BindMntPath, "test-drive")
		if err := syscall.Unmount(testPropagatedDrivePath, syscall.MNT_FORCE); err != nil {
			t.Logf("Failed to unmount %s", testPropagatedDrivePath)
		}

		if err := bindMnt.unmount(syscall.MNT_FORCE); err != nil {
			t.Logf("Failed to umount %s", bindMnt.target)
		}
	}()

	file, _, err := test.Path("dir1-bind-mounted/test-drive/test-file")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(file, []byte{}, 0700); err != nil {
		t.Fatal(err)
	}

	t.Run("bind-mounted-chmod", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			return os.Chmod(file, 0700)
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, file, event.Chmod.File.PathnameStr, "wrong path")
		}, "test_rule")
	})
}

func testMountSnapshot(t *testing.T) {
	SkipIfNotAvailable(t)

	//      / testDrive
	//        / rootA
	//          / tmpfs-mount (tmpfs)
	//                / test-bind-source
	//          / test-bind-target (bind mount of test-bind-source)
	//        / rootB
	//      ... (same hierarchy as rootA)
	//    / test-bind-testdrive

	testDrive, err := newTestDrive(t, "xfs", []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	rootA := testDrive.Path("rootA")
	rootB := testDrive.Path("rootB")

	createHierarchy := func(root string) (tmpfsMount, bindMount *testMount, err error) {
		defer func() {
			if err != nil {
				if bindMount != nil {
					bindMount.unmount(0)
				}
				if tmpfsMount != nil {
					tmpfsMount.unmount(0)
				}
			}
		}()

		tmpfsPath := path.Join(root, "tmpfs-mount")
		if err = os.MkdirAll(tmpfsPath, 0755); err != nil {
			return nil, nil, err
		}

		// tmpfs mount
		tmpfsMount = newTestMount(
			tmpfsPath,
			withFSType("tmpfs"),
		)

		if err := tmpfsMount.mount(); err != nil {
			return nil, nil, fmt.Errorf("could not create tmpfs mount: %s", err)
		}

		bindSourcePath := tmpfsMount.path("test-bind-source")
		if err = os.Mkdir(bindSourcePath, 0755); err != nil {
			return nil, nil, err
		}

		bindTargetPath := path.Join(root, "test-bind-target")
		if err = os.Mkdir(bindTargetPath, 0755); err != nil {
			return nil, nil, err
		}

		// bind mount
		bindMount = newTestMount(
			bindTargetPath,
			withSource(bindSourcePath),
			withFlags(syscall.MS_BIND),
		)

		if err = bindMount.mount(); err != nil {
			return nil, nil, fmt.Errorf("could not create bind mount: %s", err)
		}

		return
	}

	tmpfsMountA, bindMountA, err := createHierarchy(rootA)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfsMountA.unmount(0)
	defer bindMountA.unmount(0)

	test, err := newTestModule(t, nil, nil, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}), withForceReload())
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	tmpfsMountB, bindMountB, err := createHierarchy(rootB)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfsMountB.unmount(0)
	defer bindMountB.unmount(0)

	mountResolver := p.Resolvers.MountResolver
	pid := utils.Getpid()

	mounts, err := kernel.ParseMountInfoFile(int32(pid))

	if err != nil {
		t.Fatal(err)
	}

	// we need to wait for the mount events of the hierarchy B to be processed
	time.Sleep(5 * time.Second)

	checkSnapshotAndModelMatch := func(mntInfo *mountinfo.Info) {
		dev := utils.Mkdev(uint32(mntInfo.Major), uint32(mntInfo.Minor))

		mount, mountSource, mountOrigin, err := mountResolver.ResolveMount(uint32(mntInfo.ID), pid)
		if err != nil {
			t.Error(err)
			return
		}
		assert.Equal(t, model.MountSourceMountID, mountSource)
		assert.NotEqual(t, model.MountOriginUnknown, mountOrigin)
		assert.Equal(t, uint32(mntInfo.ID), mount.MountID, "snapshot and model mount ID mismatch")
		assert.Equal(t, uint32(mntInfo.Parent), mount.ParentPathKey.MountID, "snapshot and model parent mount ID mismatch")
		assert.Equal(t, dev, mount.Device, "snapshot and model device mismatch")
		assert.Equal(t, mntInfo.FSType, mount.FSType, "snapshot and model fstype mismatch")
		assert.Equal(t, mntInfo.Root, mount.RootStr, "snapshot and model root mismatch")

		mountPointPath, mountSource, mountOrigin, err := mountResolver.ResolveMountPath(mount.MountID, pid)
		if err != nil {
			t.Errorf("failed to resolve mountpoint path of mountpoint with id %d", mount.MountID)
		}
		assert.Equal(t, mntInfo.Mountpoint, mountPointPath, "snapshot and model mountpoint path mismatch")
		assert.Equal(t, model.MountSourceMountID, mountSource)
		assert.NotEqual(t, model.MountOriginUnknown, mountOrigin)
	}

	mntResolved := 0
	for _, mntInfo := range mounts {
		if strings.HasSuffix(mntInfo.Mountpoint, "rootA/tmpfs-mount") {
			mntResolved |= 1
			checkSnapshotAndModelMatch(mntInfo)
		} else if strings.HasSuffix(mntInfo.Mountpoint, "rootA/test-bind-target") {
			mntResolved |= 2
			checkSnapshotAndModelMatch(mntInfo)
		} else if strings.HasSuffix(mntInfo.Mountpoint, "rootB/tmpfs-mount") {
			mntResolved |= 4
			checkSnapshotAndModelMatch(mntInfo)
		} else if strings.HasSuffix(mntInfo.Mountpoint, "rootB/test-bind-target") {
			mntResolved |= 8
			checkSnapshotAndModelMatch(mntInfo)
		}
	}
	assert.Equal(t, 1|2|4|8, mntResolved)
}

var _ = declareInlineConfig(TestMountSnapshotListmount)

func TestMountSnapshotListmount(t *testing.T) {
	SkipIfNotAvailable(t)
	t.Setenv("DD_EVENT_MONITORING_CONFIG_SNAPSHOT_USING_LISTMOUNT", "true")
	testMountSnapshot(t)
}

var _ = declareInlineConfig(TestMountSnapshotProcfs)

func TestMountSnapshotProcfs(t *testing.T) {
	SkipIfNotAvailable(t)

	t.Setenv("DD_EVENT_MONITORING_CONFIG_SNAPSHOT_USING_LISTMOUNT", "false")
	testMountSnapshot(t)
}

func TestMountEvent(t *testing.T) {
	SkipIfNotAvailable(t)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	testDrive, err := newTestDrive(t, "xfs", []string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	tmpfsMountPointName := "tmpfs_mnt"
	bindMountPointName := "bind_mnt"
	bindMountSourceName := "bind_src"

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_mount_tmpfs",
			Expression: fmt.Sprintf(`mount.mountpoint.path == "{{.Root}}/%s" && mount.fs_type == "tmpfs" && process.file.path == "%s"`, tmpfsMountPointName, executable),
		},
		{
			ID:         "test_mount_bind",
			Expression: fmt.Sprintf(`mount.mountpoint.path == "{{.Root}}/%s" && mount.source.path == "{{.Root}}/%s" && mount.fs_type == "%s" && process.file.path == "%s"`, bindMountPointName, bindMountSourceName, testDrive.FSType(), executable),
		},
		{
			ID:         "test_mount_in_container_root",
			Expression: `mount.mountpoint.path == "/host_root" && mount.source.path == "/" && mount.fs_type != "overlay" && process.container.id != ""`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	tmpfsMountPointPath := testDrive.Path(tmpfsMountPointName)
	if err = os.Mkdir(tmpfsMountPointPath, 0755); err != nil {
		t.Fatal(err)
	}

	bindMountPointPath := testDrive.Path(bindMountPointName)
	if err = os.Mkdir(bindMountPointPath, 0755); err != nil {
		t.Fatal(err)
	}

	bindMountSourcePath := testDrive.Path(bindMountSourceName)
	if err = os.Mkdir(bindMountSourcePath, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("mount-tmpfs", func(t *testing.T) {
		tmpfsMount := newTestMount(
			tmpfsMountPointPath,
			withFSType("tmpfs"),
		)

		test.WaitSignalFromRule(t, func() error {
			if err := tmpfsMount.mount(); err != nil {
				return err
			}
			return tmpfsMount.unmount(syscall.MNT_FORCE)
		}, func(event *model.Event, rule *rules.Rule) {
			if !ebpfLessEnabled {
				assert.Equal(t, model.MountOriginEvent, event.Mount.Origin, "Incorrect mount source")
				assert.Equal(t, false, event.Mount.Detached, "Mount should not be detached")
				assert.Equal(t, true, event.Mount.Visible, "Mount should be visible")
				assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Mount namespace inode not captured")
			}
			assertTriggeredRule(t, rule, "test_mount_tmpfs")
			assertFieldEqual(t, event, "mount.mountpoint.path", tmpfsMountPointPath)
			assertFieldEqual(t, event, "mount.fs_type", "tmpfs")
			assertFieldEqual(t, event, "process.file.path", executable)

			test.validateMountSchema(t, event)
			validateSyscallContext(t, event, "$.syscall.mount.path")
			validateSyscallContext(t, event, "$.syscall.mount.destination_path")
			validateSyscallContext(t, event, "$.syscall.mount.fs_type")
		}, "test_mount_tmpfs")
	})

	t.Run("mount-bind", func(t *testing.T) {
		bindMount := newTestMount(
			bindMountPointPath,
			withSource(bindMountSourcePath),
			withFlags(syscall.MS_BIND),
		)

		test.WaitSignalFromRule(t, func() error {
			if err := bindMount.mount(); err != nil {
				return err
			}
			return bindMount.unmount(syscall.MNT_FORCE)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mount_bind")
			if !ebpfLessEnabled {
				assert.Equal(t, false, event.Mount.Detached, "Mount should not be detached")
				assert.Equal(t, true, event.Mount.Visible, "Mount should be visible")
				assert.Equal(t, model.MountOriginEvent, event.Mount.Origin, "Incorrect mount source")
				assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Mount namespace inode not captured")
			}
			assertFieldEqual(t, event, "mount.mountpoint.path", bindMountPointPath)
			assertFieldEqual(t, event, "mount.source.path", bindMountSourcePath)
			assertFieldEqual(t, event, "mount.fs_type", testDrive.FSType())
			assertFieldEqual(t, event, "process.file.path", executable)

			if !ebpfLessEnabled {
				// the root of the bind mount is relative to the root of the test drive file system
				assertFieldEqual(t, event, "mount.root.path", "/"+bindMountSourceName)
				assertSerializedFieldEqual(t, event, "$.mount.mp.path", bindMountPointPath)
				assertSerializedFieldEqual(t, event, "$.mount.root.path", "/"+bindMountSourceName)
			}

			test.validateMountSchema(t, event)
			validateSyscallContext(t, event, "$.syscall.mount.path")
			validateSyscallContext(t, event, "$.syscall.mount.destination_path")
			validateSyscallContext(t, event, "$.syscall.mount.fs_type")
		}, "test_mount_bind")
	})

	const dockerMountDest = "/host_root"

	t.Run("mount-in-container-root", func(t *testing.T) {
		SkipIfNotAvailable(t)
		flake.MarkOnJobName(t, "ubuntu_25.10")

		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *ebpfkernel.Version) bool {
			return kv.IsSuse12Kernel()
		})

		wrapperTruePositive, err := newDockerCmdWrapper("/", dockerMountDest, "alpine", "")
		if err != nil {
			t.Fatalf("failed to start docker wrapper: %v", err)
		}

		test.WaitSignalFromRule(t, func() error {
			if _, err := wrapperTruePositive.start(); err != nil {
				return err
			}
			if _, err := wrapperTruePositive.stop(); err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mount_in_container_root")
			assertFieldEqual(t, event, "mount.mountpoint.path", "/host_root")
			assertFieldEqual(t, event, "mount.source.path", "/")
			assertFieldNotEqual(t, event, "mount.fs_type", "overlay")
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")
			assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Mount namespace inode not captured")

			test.validateMountSchema(t, event)
			validateSyscallContext(t, event, "$.syscall.mount.path")
			validateSyscallContext(t, event, "$.syscall.mount.destination_path")
			validateSyscallContext(t, event, "$.syscall.mount.fs_type")
		}, "test_mount_in_container_root")
	})

	t.Run("mount-in-container-legitimate", func(t *testing.T) {
		if _, err := whichNonFatal("docker"); err != nil {
			t.Skip("Skip test where docker is unavailable")
		}

		checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *ebpfkernel.Version) bool {
			return kv.IsSuse12Kernel()
		})

		legitimateSourcePath := testDrive.Path("legitimate_source")
		if err = os.Mkdir(legitimateSourcePath, 0755); err != nil {
			t.Fatal(err)
		}

		wrapperFalsePositive, err := newDockerCmdWrapper(legitimateSourcePath, dockerMountDest, "alpine", "")
		if err != nil {
			t.Fatalf("failed to start docker wrapper: %v", err)
		}

		err = test.GetSignal(t, func() error {
			if _, err := wrapperFalsePositive.start(); err != nil {
				return err
			}
			if _, err := wrapperFalsePositive.stop(); err != nil {
				return err
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			t.Errorf("shouldn't get an event: event %s matched rule %s", test.debugEvent(event), rule.Expression)
			assert.NotEqual(t, 0, event.Mount.NamespaceInode, "Mount namespace inode not captured")
		})
		if err == nil {
			t.Error("shouldn't get an event")
		} else if otherErr, ok := err.(ErrTimeout); !ok {
			t.Fatal(otherErr)
		}
	})
}

// mountSubdirEnv is a private tmpfs holding a sub directory, so that bind mounts of this sub directory have a mount
// root different from "/"
type mountSubdirEnv struct {
	base   string
	srcDir string
	dstDir string
}

func newMountSubdirEnv(t *testing.T) *mountSubdirEnv {
	base := t.TempDir()
	if err := unix.Mount("tmpfs", base, "tmpfs", 0, "size=16M"); err != nil {
		t.Fatalf("failed to mount tmpfs: %v", err)
	}
	t.Cleanup(func() { _ = unix.Unmount(base, unix.MNT_DETACH) })

	if err := unix.Mount("", base, "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		t.Fatalf("failed to make tmpfs private: %v", err)
	}

	env := &mountSubdirEnv{
		base:   base,
		srcDir: filepath.Join(base, "src", "sub"),
		dstDir: filepath.Join(base, "dst"),
	}
	for _, dir := range []string{env.srcDir, env.dstDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	return env
}

func copyTrue(t *testing.T, dst string) {
	if err := copyFile(which(t, "true"), dst, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestMountOpenTreeSubdirMoveMount checks the path of a file under a clone of a sub directory, created by open_tree
// and attached by move_mount, whose mount root is the sub directory
func TestMountOpenTreeSubdirMoveMount(t *testing.T) {
	SkipIfNotAvailable(t)

	if !openTreeIsSupported() || !testutils.SyscallExists(unix.SYS_MOVE_MOUNT) {
		t.Skip("open_tree/move_mount not supported")
	}

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_mount_open_tree_subdir_move_mount",
		Expression: `exec.file.name == "mnt-open-tree-subdir"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	env := newMountSubdirEnv(t)
	copyTrue(t, filepath.Join(env.srcDir, "mnt-open-tree-subdir"))

	fd, err := unix.OpenTree(unix.AT_FDCWD, env.srcDir, unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)

	if err := unix.MoveMount(fd, "", unix.AT_FDCWD, env.dstDir, unix.MOVE_MOUNT_F_EMPTY_PATH); err != nil {
		t.Fatal(err)
	}
	defer unix.Unmount(env.dstDir, unix.MNT_DETACH)

	expected := filepath.Join(env.dstDir, "mnt-open-tree-subdir")
	test.WaitSignalFromRule(t, func() error {
		return exec.Command(expected).Run()
	}, func(event *model.Event, _ *rules.Rule) {
		assertFieldEqual(t, event, "exec.file.path", expected)
	}, "test_mount_open_tree_subdir_move_mount")
}

// TestMountBindSubdirPivotRoot checks the path of a file under the new root of a mount namespace, when this root is a
// bind mount of a sub directory, like the rootfs of a container that is a plain directory
func TestMountBindSubdirPivotRoot(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_mount_bind_subdir_pivot_root",
		Expression: `exec.file.name == "mnt-bind-subdir-pivot-root"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	env := newMountSubdirEnv(t)
	rootfs := env.srcDir

	// the new root has no library, nor /dev/null
	testerBin, err := syscallTesterFS.ReadFile("syscall_tester/bin/syscall_tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "mnt-bind-subdir-pivot-root"), testerBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(rootfs, "old"), 0o755); err != nil {
		t.Fatal(err)
	}

	test.WaitSignalFromRule(t, func() error {
		done := make(chan error, 1)
		go func() {
			// the thread is left in the pivoted mount namespace, the runtime terminates it when the goroutine exits
			runtime.LockOSThread()

			steps := []func() error{
				func() error { return unix.Unshare(unix.CLONE_NEWNS | unix.CLONE_FS) },
				func() error { return unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "") },
				func() error { return unix.Mount(rootfs, rootfs, "", unix.MS_BIND|unix.MS_REC, "") },
				func() error { return unix.Chdir(rootfs) },
				func() error { return unix.PivotRoot(".", "old") },
				func() error { return unix.Chdir("/") },
				func() error { return unix.Unmount("/old", unix.MNT_DETACH) },
				func() error {
					cmd := exec.Command("/mnt-bind-subdir-pivot-root")
					cmd.Stdin = strings.NewReader("")
					// syscall_tester exits with an error without argument
					var exitErr *exec.ExitError
					if _, err := cmd.CombinedOutput(); err != nil && !errors.As(err, &exitErr) {
						return err
					}
					return nil
				},
			}
			for _, step := range steps {
				if err := step(); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
		return <-done
	}, func(event *model.Event, _ *rules.Rule) {
		assertFieldEqual(t, event, "exec.file.path", "/mnt-bind-subdir-pivot-root")
	}, "test_mount_bind_subdir_pivot_root")
}

// TestMountSubmountOfBindSubdir checks the path of a file under a mount whose mount point is inside a bind mount of a
// sub directory
func TestMountSubmountOfBindSubdir(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_mount_submount_of_bind_subdir",
		Expression: `exec.file.name == "mnt-submount-of-bind-subdir"`,
	}}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	env := newMountSubdirEnv(t)
	if err := unix.Mount(env.srcDir, env.dstDir, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer unix.Unmount(env.dstDir, unix.MNT_DETACH)

	inner := filepath.Join(env.dstDir, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount("tmpfs", inner, "tmpfs", 0, "size=8M"); err != nil {
		t.Fatal(err)
	}
	defer unix.Unmount(inner, unix.MNT_DETACH)

	expected := filepath.Join(inner, "mnt-submount-of-bind-subdir")
	copyTrue(t, expected)

	test.WaitSignalFromRule(t, func() error {
		return exec.Command(expected).Run()
	}, func(event *model.Event, _ *rules.Rule) {
		assertFieldEqual(t, event, "exec.file.path", expected)
	}, "test_mount_submount_of_bind_subdir")
}

var _ = declareInlineConfig(TestMountSnapshotSubmountOfMovedBindSubdir)

// TestMountSnapshotSubmountOfMovedBindSubdir checks the path of a file under a mount known from the snapshot, once its
// parent, a bind mount of a sub directory also known from the snapshot, is moved
func TestMountSnapshotSubmountOfMovedBindSubdir(t *testing.T) {
	SkipIfNotAvailable(t)

	env := newMountSubdirEnv(t)
	parent := filepath.Join(env.base, "a")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(env.srcDir, parent, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(parent, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount("tmpfs", inner, "tmpfs", 0, "size=8M"); err != nil {
		t.Fatal(err)
	}
	copyTrue(t, filepath.Join(inner, "mnt-snapshot-moved-parent"))

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_mount_snapshot_submount_of_moved_bind_subdir",
		Expression: `exec.file.name == "mnt-snapshot-moved-parent"`,
	}}

	// forces a new snapshot including the mounts above
	test, err := newTestModule(t, nil, ruleDefs, withForceReload())
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	moved := filepath.Join(env.base, "b")
	if err := os.Mkdir(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mount(parent, moved, "", unix.MS_MOVE, ""); err != nil {
		t.Fatal(err)
	}
	defer unix.Unmount(moved, unix.MNT_DETACH)
	defer unix.Unmount(filepath.Join(moved, "inner"), unix.MNT_DETACH)

	expected := filepath.Join(moved, "inner", "mnt-snapshot-moved-parent")
	test.WaitSignalFromRule(t, func() error {
		return exec.Command(expected).Run()
	}, func(event *model.Event, _ *rules.Rule) {
		assertFieldEqual(t, event, "exec.file.path", expected)
	}, "test_mount_snapshot_submount_of_moved_bind_subdir")
}
