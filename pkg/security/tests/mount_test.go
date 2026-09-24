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
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

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

			return ebpfLessEnabled || assert.Equal(t, mntID.Load(), event.Umount.MountID, "wrong mount id")
		}, 3*time.Second, model.FileUmountEventType)
		if err != nil {
			t.Error(err)
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
