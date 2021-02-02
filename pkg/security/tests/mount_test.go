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
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestMount(t *testing.T) {
	dstMntBasename := "test-dest-mount"
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`utimes.filename == "{{.Root}}/%s/test-mount"`, dstMntBasename),
	}

	testDrive, err := newTestDrive("ext4", []string{})
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
			if event.GetType() != "mount" {
				t.Errorf("expected mount event, got %s", event.GetType())
			}

			if event.Mount.MountPointStr != "/"+dstMntBasename {
				t.Errorf("expected %v for ParentPathStr, got %v", dstMntPath, event.Mount.MountPointStr)
			}

			// use accessor to parse properly the mount type
			if fs := event.Mount.GetFSType(); fs != "bind" {
				t.Errorf("expected a bind mount, got %v", fs)
			}
			mntID = event.Mount.MountID
		}
	})

	t.Run("mount_resolver", func(t *testing.T) {
		utimFile, utimFilePtr, err := testDrive.Path(path.Join(dstMntBasename, "test-mount"))
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(utimFile)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(utimFile)

		utimbuf := &syscall.Utimbuf{
			Actime:  123,
			Modtime: 456,
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_UTIME, uintptr(utimFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "utimes" {
				t.Errorf("expected utimes event, got %s", event.GetType())
			}

			if event.Utimes.PathnameStr != utimFile {
				t.Errorf("expected %v for PathnameStr, got %v", utimFile, event.Utimes.PathnameStr)
			}
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
			if event.GetType() != "umount" {
				t.Errorf("expected umount event, got %s", event.GetType())
			}

			if uMntID := event.Umount.MountID; uMntID != mntID {
				t.Errorf("expected mount_id %v, got %v", mntID, uMntID)
			}
		}
	})
}
