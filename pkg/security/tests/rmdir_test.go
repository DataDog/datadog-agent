// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestRmdir(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rmdir.filename == "{{.Root}}/test-rmdir" || rmdir.filename == "{{.Root}}/test-unlink-rmdir"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("rmdir", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testFile, 0777); err != nil {
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

			if inode != event.Rmdir.Inode {
				t.Errorf("expected inode %d, got %d", event.Mkdir.Inode, inode)
			}

			testContainerPath(t, event, "rmdir.container_path")
		}
	})

	t.Run("unlinkat-at_removedir", func(t *testing.T) {
		testDir, testDirPtr, err := test.Path("test-unlink-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testDir, 0777); err != nil {
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

			if inode != event.Rmdir.Inode {
				t.Errorf("expected inode %d, got %d", event.Mkdir.Inode, inode)
			}

			testContainerPath(t, event, "rmdir.container_path")
		}
	})
}
