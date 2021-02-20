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

func TestUnlink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.file.path in ["{{.Root}}/test-unlink", "{{.Root}}/test-unlinkat"] && unlink.file.uid == 98 && unlink.file.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
	testFile, testFilePtr, err := test.CreateWithOptions("test-unlink", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	inode := getInode(t, testFile)

	t.Run("unlink", func(t *testing.T) {
		if _, _, err := syscall.Syscall(syscall.SYS_UNLINK, uintptr(testFilePtr), 0, 0); err != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "unlink" {
				t.Errorf("expected unlink event, got %s", event.GetType())
			}

			if inode != event.Unlink.File.Inode {
				t.Logf("expected inode %d, got %d", event.Unlink.File.Inode, inode)
			}

			if int(event.Unlink.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Unlink.File.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Unlink.File.MTime.After(now) || event.Unlink.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Unlink.File.MTime)
			}

			if event.Unlink.File.CTime.After(now) || event.Unlink.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Unlink.File.CTime)
			}

			testContainerPath(t, event, "unlink.file.container_path")
		}
	})

	testAtFile, testAtFilePtr, err := test.CreateWithOptions("test-unlinkat", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testAtFile)

	inode = getInode(t, testAtFile)

	t.Run("unlinkat", func(t *testing.T) {
		if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testAtFilePtr), 0); err != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "unlink" {
				t.Errorf("expected unlink event, got %s", event.GetType())
			}

			if inode != event.Unlink.File.Inode {
				t.Logf("expected inode %d, got %d", event.Unlink.File.Inode, inode)
			}

			if int(event.Unlink.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.Unlink.File.Mode)&expectedMode)
			}

			now := time.Now()
			if event.Unlink.File.MTime.After(now) || event.Unlink.File.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Unlink.File.MTime)
			}

			if event.Unlink.File.CTime.After(now) || event.Unlink.File.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Unlink.File.CTime)
			}

			testContainerPath(t, event, "unlink.file.container_path")
		}
	})
}
