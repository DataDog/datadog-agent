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

	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestLink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `link.file.path == "{{.Root}}/test-link" && link.file.destination.path == "{{.Root}}/test2-link" && link.file.uid == 98 && link.file.gid == 99 && link.file.destination.uid == 98 && link.file.destination.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
	_, testOldFilePtr, err := test.CreateWithOptions("test-link", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	testNewFile, testNewFilePtr, err := test.Path("test2-link")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("link", ifSyscallSupported("SYS_LINK", func(t *testing.T, syscallNB uintptr) {
		_, _, errno := syscall.Syscall(syscallNB, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
		if errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "link", "wrong event type")
			assert.Equal(t, event.Link.Source.Inode, getInode(t, testNewFile), "wrong inode")

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "link.file.container_path")
				testContainerPath(t, event, "link.file.destination.container_path")
			}

			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))

			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)
		}

		if err := os.Remove(testNewFile); err != nil {
			t.Fatal(err)
		}
	}))

	t.Run("linkat", func(t *testing.T) {
		_, _, errno := syscall.Syscall6(syscall.SYS_LINKAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
		if errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "link", "wrong event type")
			assert.Equal(t, event.Link.Source.Inode, getInode(t, testNewFile), "wrong inode")

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "link.file.container_path")
				testContainerPath(t, event, "link.file.destination.container_path")
			}

			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))

			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)
		}
	})
}
