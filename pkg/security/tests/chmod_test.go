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

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"gotest.tools/assert"
)

func TestChmod(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `chmod.file.path == "{{.Root}}/test-chmod" && chmod.file.destination.mode in [0707, 0447, 0757] && chmod.file.uid == 98 && chmod.file.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint16(applyUmask(fileMode))
	testFile, testFilePtr, err := test.CreateWithOptions("test-chmod", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	t.Run("chmod", func(t *testing.T) {
		if _, _, errno := syscall.Syscall(syscall.SYS_CHMOD, uintptr(testFilePtr), uintptr(0o707), 0); errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "chmod", "wrong event type")
			assertRights(t, uint16(event.Chmod.Mode), 0o707, "wrong mode")
			assert.Equal(t, event.Chmod.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chmod.File.Mode, uint16(expectedMode), "wrong initial mode")

			assertNearTime(t, event.Chmod.File.MTime)
			assertNearTime(t, event.Chmod.File.CTime)

			testContainerPath(t, event, "chmod.file.container_path")
		}
	})

	t.Run("fchmod", func(t *testing.T) {
		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, errno := syscall.Syscall(syscall.SYS_FCHMOD, f.Fd(), uintptr(0o447), 0); errno != 0 {
			t.Fatal(err)
		}
		defer f.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "chmod", "wrong event type")
			assertRights(t, uint16(event.Chmod.Mode), 0o447)
			assert.Equal(t, event.Chmod.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chmod.File.Mode, 0o707, "wrong initial mode")

			assertNearTime(t, event.Chmod.File.MTime)
			assertNearTime(t, event.Chmod.File.CTime)

			testContainerPath(t, event, "chmod.file.container_path")
		}
	})

	t.Run("fchmodat", func(t *testing.T) {
		if _, _, errno := syscall.Syscall6(syscall.SYS_FCHMODAT, 0, uintptr(testFilePtr), uintptr(0o757), 0, 0, 0); errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "chmod", "wrong event type")
			assertRights(t, uint16(event.Chmod.Mode), 0o757)
			assert.Equal(t, event.Chmod.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chmod.File.Mode, 0o447)

			assertNearTime(t, event.Chmod.File.MTime)
			assertNearTime(t, event.Chmod.File.CTime)

			testContainerPath(t, event, "chmod.file.container_path")
		}
	})
}
