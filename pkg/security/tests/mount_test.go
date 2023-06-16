// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/moby/sys/mountinfo"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestMount(t *testing.T) {
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

	test, err := newTestModule(t, nil, ruleDefs, testOpts{testDir: testDrive.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mntPath := testDrive.Path("test-mount")
	os.MkdirAll(mntPath, 0755)
	defer os.RemoveAll(mntPath)

	dstMntPath := testDrive.Path(dstMntBasename)
	os.MkdirAll(dstMntPath, 0755)
	defer os.RemoveAll(dstMntPath)

	var mntID uint32
	t.Run("mount", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			// Test mount
			if err := syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, ""); err != nil {
				return fmt.Errorf("could not create bind mount: %w", err)
			}
			return nil
		}, func(event *model.Event) bool {
			mntID = event.Mount.MountID

			if !assert.Equal(t, "mount", event.GetType(), "wrong event type") {
				return true
			}

			// filter by pid
			if pce, _ := event.ResolveProcessCacheEntry(); pce.Pid != testSuitePid {
				return false
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

		test.WaitSignal(t, func() error {
			return os.Chmod(file, 0707)
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, file, event.Chmod.File.PathnameStr, "wrong path")
		})
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
			if !assert.Equal(t, "umount", event.GetType(), "wrong event type") {
				return true
			}

			// filter by process
			if pce, _ := event.ResolveProcessCacheEntry(); pce.Pid != testSuitePid {
				return false
			}

			return assert.Equal(t, mntID, event.Umount.MountID, "wrong mount id")
		}, 3*time.Second, model.FileUmountEventType)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("release-mount", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			return syscall.Fchownat(int(releaseFile.Fd()), "", 123, 123, unix.AT_EMPTY_PATH)
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, rule, "test_rule_pending")
		})
	})
}

func TestMountPropagated(t *testing.T) {
	// - testroot
	// 		/ dir1
	// 			/ test-drive (xfs mount)
	// 		/ dir-bind-mounted (bind mount of testroot/dir1)
	// 			/ test-drive (propagated)
	//				/ test-file

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`chmod.file.path == "{{.Root}}/dir1-bind-mounted/test-drive/test-file"`),
	}}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	dir1Path, _, err := test.Path("dir1")
	if err != nil {
		t.Fatal(err)
	}

	testDrivePath := path.Join(dir1Path, "test-drive")
	if err := os.MkdirAll(testDrivePath, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDrivePath)

	testDrive, err := newTestDrive(t, "xfs", []string{}, testDrivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

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
		test.WaitSignal(t, func() error {
			return os.Chmod(file, 0700)
		}, func(event *model.Event, rule *rules.Rule) {
			t.Log(event.Open.File.PathnameStr)
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, file, event.Chmod.File.PathnameStr, "wrong path")
		})
	})
}

func TestMountSnapshot(t *testing.T) {
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

	test, err := newTestModule(t, nil, nil, testOpts{testDir: testDrive.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	tmpfsMountB, bindMountB, err := createHierarchy(rootB)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpfsMountB.unmount(0)
	defer bindMountB.unmount(0)

	mountResolver := test.probe.GetResolvers().MountResolver
	pid := uint32(utils.Getpid())

	mounts, err := kernel.ParseMountInfoFile(int32(pid))
	if err != nil {
		t.Fatal(err)
	}

	// we need to wait for the mount events of the hierarchy B to be processed
	time.Sleep(5 * time.Second)

	checkSnapshotAndModelMatch := func(mntInfo *mountinfo.Info) {
		mount, err := mountResolver.ResolveMount(uint32(mntInfo.ID), pid, "")
		if err != nil {
			t.Errorf(err.Error())
			return
		}
		assert.Equal(t, uint32(mntInfo.ID), mount.MountID, "snapshot and model mount ID mismatch")
		assert.Equal(t, uint32(mntInfo.Parent), mount.ParentMountID, "snapshot and model parent mount ID mismatch")
		assert.Equal(t, uint32(unix.Mkdev(uint32(mntInfo.Major), uint32(mntInfo.Minor))), mount.Device, "snapshot and model device mismatch")
		assert.Equal(t, mntInfo.FSType, mount.FSType, "snapshot and model fstype mismatch")
		assert.Equal(t, mntInfo.Root, mount.RootStr, "snapshot and model root mismatch")
		mountPointPath, err := mountResolver.ResolveMountPath(mount.MountID, pid, "")
		if err != nil {
			t.Errorf("failed to resolve mountpoint path of mountpoint with id %d", mount.MountID)
		}
		assert.Equal(t, mntInfo.Mountpoint, mountPointPath, "snapshot and model mountpoint path mismatch")
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

func TestMountEvent(t *testing.T) {
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
			Expression: `mount.mountpoint.path == "/host_root" && mount.source.path == "/" && mount.fs_type != "overlay" && container.id != ""`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{testDir: testDrive.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

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

		test.WaitSignal(t, func() error {
			if err := tmpfsMount.mount(); err != nil {
				return err
			}
			return tmpfsMount.unmount(syscall.MNT_FORCE)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mount_tmpfs")
			assertFieldEqual(t, event, "mount.mountpoint.path", tmpfsMountPointPath)
			assertFieldEqual(t, event, "mount.fs_type", "tmpfs")
			assertFieldEqual(t, event, "process.file.path", executable)

			test.validateMountSchema(t, event)
		})
	})

	t.Run("mount-bind", func(t *testing.T) {
		bindMount := newTestMount(
			bindMountPointPath,
			withSource(bindMountSourcePath),
			withFlags(syscall.MS_BIND),
		)

		test.WaitSignal(t, func() error {
			if err := bindMount.mount(); err != nil {
				return err
			}
			return bindMount.unmount(syscall.MNT_FORCE)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_mount_bind")
			assertFieldEqual(t, event, "mount.mountpoint.path", bindMountPointPath)
			assertFieldEqual(t, event, "mount.source.path", bindMountSourcePath)
			assertFieldEqual(t, event, "mount.fs_type", testDrive.FSType())
			assertFieldEqual(t, event, "process.file.path", executable)

			test.validateMountSchema(t, event)
		})
	})

	const dockerMountDest = "/host_root"
	wrapperTruePositive, err := newDockerCmdWrapper("/", dockerMountDest, "alpine")
	if err != nil {
		t.Skip("Skipping mounts in containers tests: Docker not available")
		return
	}

	t.Run("mount-in-container-root", func(t *testing.T) {
		test.WaitSignal(t, func() error {
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
			assertFieldNotEmpty(t, event, "container.id", "container id shouldn't be empty")

			test.validateMountSchema(t, event)
		})
	})

	legitimateSourcePath := testDrive.Path("legitimate_source")
	if err = os.Mkdir(legitimateSourcePath, 0755); err != nil {
		t.Fatal(err)
	}
	wrapperFalsePositive, err := newDockerCmdWrapper(legitimateSourcePath, dockerMountDest, "alpine")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("mount-in-container-legitimate", func(t *testing.T) {
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
		})
		if err == nil {
			t.Error("shouldn't get an event")
		} else if otherErr, ok := err.(ErrTimeout); !ok {
			t.Fatal(otherErr)
		}
	})
}
