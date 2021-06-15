// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"gotest.tools/assert"

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
	expectedMode := uint16(applyUmask(mkdirMode))

	t.Run("rmdir", ifSyscallSupported("SYS_RMDIR", func(t *testing.T, syscallNB uintptr) {
		testFile, testFilePtr, err := test.Path("test-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testFile, uint32(mkdirMode)); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		inode := getInode(t, testFile)

		if _, _, err := syscall.Syscall(syscallNB, uintptr(testFilePtr), 0, 0); err != 0 {
			t.Fatal(error(err))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "rmdir", "wrong event type")
			assert.Equal(t, event.Rmdir.File.Inode, inode, "wrong inode")
			assertRights(t, event.Rmdir.File.Mode, expectedMode, "wrong initial mode")

			assertNearTime(t, event.Rmdir.File.MTime)
			assertNearTime(t, event.Rmdir.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rmdir.file.container_path")
			}
		}
	}))

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
			assert.Equal(t, event.GetType(), "rmdir", "wrong event type")
			assert.Equal(t, event.Rmdir.File.Inode, inode, "wrong inode")
			assertRights(t, event.Rmdir.File.Mode, expectedMode, "wrong initial mode")

			assertNearTime(t, event.Rmdir.File.MTime)
			assertNearTime(t, event.Rmdir.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "rmdir.file.container_path")
			}
		}
	})
}

func TestRmdirInvalidate(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rmdir.file.path =~ "{{.Root}}/test-rmdir-*"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for i := 0; i != 5; i++ {
		testFile, _, err := test.Path(fmt.Sprintf("test-rmdir-%d", i))
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testFile, 0777); err != nil {
			t.Fatal(err)
		}

		if err := syscall.Rmdir(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "rmdir", "wrong event type")
			assertFieldEqual(t, event, "rmdir.file.path", testFile)
		}
	}
}
