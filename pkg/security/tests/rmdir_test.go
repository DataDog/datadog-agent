// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestRmdir(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rmdir.file.path in ["{{.Root}}/test-rmdir", "{{.Root}}/test-unlink-rmdir"] && rmdir.file.uid == 0 && rmdir.file.gid == 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mkdirMode := 0o707
	expectedMode := applyUmask(mkdirMode)

	t.Run("rmdir", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testFile, uint32(mkdirMode)); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		inode := getInode(t, testFile)

		if _, _, err := syscall.Syscall(syscall.SYS_RMDIR, uintptr(testFilePtr), 0, 0); err != 0 {
			t.Fatal(error(err))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rmdir" {
				t.Errorf("expected rmdir event, got %s", event.GetType())
			}

			if inode != event.Rmdir.File.Inode {
				t.Logf("expected inode %d, got %d", event.Mkdir.File.Inode, inode)
			}

			if int(event.Rmdir.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Rmdir.File.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Rmdir.File.MTime.After(now) || event.Rmdir.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Rmdir.File.MTime)
			}

			if event.Rmdir.File.CTime.After(now) || event.Rmdir.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Rmdir.File.CTime)
			}

			testContainerPath(t, event, "rmdir.file.container_path")
		}
	})

	t.Run("unlinkat-at_removedir", func(t *testing.T) {
		testDir, testDirPtr, err := test.Path("test-unlink-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testDir, uint32(mkdirMode)); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testDir)

		inode := getInode(t, testDir)

		if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testDirPtr), 512); err != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rmdir" {
				t.Errorf("expected rmdir event, got %s", event.GetType())
			}

			if inode != event.Rmdir.File.Inode {
				t.Logf("expected inode %d, got %d", event.Mkdir.File.Inode, inode)
			}

			if int(event.Rmdir.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Rmdir.File.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Rmdir.File.MTime.After(now) || event.Rmdir.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Rmdir.File.MTime)
			}

			if event.Rmdir.File.CTime.After(now) || event.Rmdir.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Rmdir.File.CTime)
			}

			testContainerPath(t, event, "rmdir.file.container_path")
		}
	})
}
