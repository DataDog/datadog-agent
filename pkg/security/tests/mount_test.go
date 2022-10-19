// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
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

	mntPath, _, err := testDrive.Path("test-mount")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(mntPath, 0755)
	defer os.RemoveAll(mntPath)

	dstMntPath, _, err := testDrive.Path(dstMntBasename)
	if err != nil {
		t.Fatal(err)
	}
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
		}, func(event *sprobe.Event) bool {
			mntID = event.Mount.MountID

			if !assert.Equal(t, "mount", event.GetType(), "wrong event type") {
				return true
			}

			// filter by pid
			if pce := event.ResolveProcessCacheEntry(); pce.Pid != testSuitePid {
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
		file, _, err := testDrive.Path(path.Join(dstMntBasename, "test-mount"))
		if err != nil {
			t.Fatal(err)
		}

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
		}, func(event *sprobe.Event, rule *rules.Rule) {
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
		}, func(event *sprobe.Event) bool {
			if !assert.Equal(t, "umount", event.GetType(), "wrong event type") {
				return true
			}

			// filter by process
			if pce := event.ResolveProcessCacheEntry(); pce.Pid != testSuitePid {
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
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, rule, "test_rule_pending")
		})
	})
}

func TestMountPropagated(t *testing.T) {
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

	if err := syscall.Mount(dir1Path, dir1BindMntPath, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		t.Fatal(err)
	}
	defer func() {
		testDrivePath := path.Join(dir1BindMntPath, "test-drive")
		if err := syscall.Unmount(testDrivePath, syscall.MNT_FORCE); err != nil {
			t.Logf("Failed to unmount %s", testDrivePath)
		}

		if err := syscall.Unmount(dir1BindMntPath, syscall.MNT_FORCE); err != nil {
			t.Logf("Failed to unmount %s", dir1BindMntPath)
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
		}, func(event *sprobe.Event, rule *rules.Rule) {
			t.Log(event.Open.File.PathnameStr)
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, file, event.Chmod.File.PathnameStr, "wrong path")
		})
	})
}
