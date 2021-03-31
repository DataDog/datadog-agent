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

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"gotest.tools/assert"
)

func TestMount(t *testing.T) {
	dstMntBasename := "test-dest-mount"
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`chmod.file.path == "{{.Root}}/%s/test-mount"`, dstMntBasename),
	}

	testDrive, err := newTestDrive("xfs", []string{})
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{testDir: testDrive.Root(), wantProbeEvents: true})
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
		// Test mount
		if err := syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, ""); err != nil {
			t.Fatalf("could not create bind mount: %s", err)
		}

		event, err := test.GetProbeEvent(3*time.Second, "mount")
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "mount", "wrong event type")
			assert.Equal(t, event.Mount.MountPointStr, "/"+dstMntBasename, "wrong mount point")
			assert.Equal(t, event.Mount.GetFSType(), "xfs", "wrong mount fs type")

			mntID = event.Mount.MountID
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

		if err := os.Chmod(file, 0707); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "chmod", "wrong event type")
			assert.Equal(t, event.Chmod.File.PathnameStr, file, "wrong path")
		}
	})

	t.Run("umount", func(t *testing.T) {
		// Test umount
		if err := syscall.Unmount(dstMntPath, syscall.MNT_DETACH); err != nil {
			t.Fatalf("could not unmount test-mount: %s", err)
		}

		event, err := test.GetProbeEvent(3*time.Second, "umount")
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "umount", "wrong event type")
			assert.Equal(t, event.Umount.MountID, mntID, "wrong mount id")
		}
	})
}
