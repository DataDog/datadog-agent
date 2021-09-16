// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

	"github.com/DataDog/datadog-agent/pkg/security/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
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

	testDrive, err := newTestDrive("xfs", []string{})
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
				t.Fatalf("could not create bind mount: %s", err)
			}
			return nil
		}, func(event *sprobe.Event) bool {
			assert.Equal(t, "mount", event.GetType(), "wrong event type")
			assert.Equal(t, "/"+dstMntBasename, event.Mount.MountPointStr, "wrong mount point")
			assert.Equal(t, "xfs", event.Mount.GetFSType(), "wrong mount fs type")

			mntID = event.Mount.MountID
			return true
		}, 3*time.Second, model.FileMountEventType)
		if err != nil {
			t.Error(err)
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

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(file)

		err = test.GetSignal(t, func() error {
			return os.Chmod(file, 0707)
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "chmod", event.GetType(), "wrong event type")
			assert.Equal(t, file, event.Chmod.File.PathnameStr, "wrong path")
		})
		if err != nil {
			t.Error(err)
		}
	})

	releaseFile, err := os.Create(path.Join(dstMntPath, "test-release"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFile.Close()

	t.Run("umount", func(t *testing.T) {
		err = test.GetProbeEvent(func() error {
			// Test umount
			if err := syscall.Unmount(dstMntPath, syscall.MNT_DETACH); err != nil {
				t.Fatalf("could not unmount test-mount: %s", err)
			}
			return nil
		}, func(event *sprobe.Event) bool {
			assert.Equal(t, "umount", event.GetType(), "wrong event type")
			assert.Equal(t, mntID, event.Umount.MountID, "wrong mount id")
			return true
		}, 3*time.Second, model.FileUmountEventType)
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("release-mount", func(t *testing.T) {
		err = test.GetSignal(t, func() error {
			return syscall.Fchownat(int(releaseFile.Fd()), "", 123, 123, unix.AT_EMPTY_PATH)
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, rule, "test_rule_pending")
		})
		if err != nil {
			t.Error(err)
		}
	})
}
