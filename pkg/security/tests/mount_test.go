// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestMount(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `utimes.filename == "{{.Root}}/test-mount"`,
	}

	test, err := newTestProbe(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mntPath, _, err := test.Path("test-mount")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(mntPath, 0755)

	dstMntPath, _, err := test.Path("test-dest-mount")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dstMntPath, 0755)

	var mntID uint32
	t.Run("mount", func(t *testing.T) {
		// Test mount
		if err := syscall.Mount(mntPath, dstMntPath, "bind", syscall.MS_BIND, ""); err != nil {
			t.Fatalf("could not create bind mount: %s", err)
		}

		event, err := test.GetEvent(3 * time.Second)
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "mount" {
				t.Errorf("expected mount event, got %s", event.GetType())
			}

			p := event.Mount.MountPointStr
			p = strings.Replace(p, "/tmp", "", 1)
			if p != strings.Replace(dstMntPath, "/tmp", "", 1) {
				t.Errorf("expected %v for ParentPathStr, got %v", mntPath, p)
			}

			if fs := event.Mount.FSType; fs != "bind" {
				t.Errorf("expected a bind mount, got %v", fs)
			}
			mntID = event.Mount.NewMountID
		}
	})

	t.Run("umount", func(t *testing.T) {
		// Test umount
		if err := syscall.Unmount(dstMntPath, syscall.MNT_DETACH); err != nil {
			t.Fatalf("could not unmount test-mount: %s", err)
		}

		event, err := test.GetEvent(3 * time.Second)
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
