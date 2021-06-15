// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"runtime"
	"syscall"
	"testing"

	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestMkdir(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdir",
			Expression: `mkdir.file.path == "{{.Root}}/test-mkdir" && mkdir.file.uid == 0 && mkdir.file.gid == 0`,
		},
		{
			ID:         "test_rule_mkdirat",
			Expression: `mkdir.file.path == "{{.Root}}/testat-mkdir" && mkdir.file.uid == 0 && mkdir.file.gid == 0`,
		},
	}

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	mkdirMode := uint16(0o707)
	expectedMode := uint16(applyUmask(int(mkdirMode)))

	t.Run("mkdir", ifSyscallSupported("SYS_MKDIR", func(t *testing.T, syscallNB uintptr) {
		testFile, testFilePtr, err := test.Path("test-mkdir")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(mkdirMode), 0); errno != 0 {
			t.Fatal(err)
		}
		defer syscall.Rmdir(testFile)

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_mkdir")
			assertRights(t, uint16(event.Mkdir.Mode), mkdirMode)
			assert.Equal(t, event.Mkdir.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Mkdir.File.Mode, expectedMode)

			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "mkdir.file.container_path")
			}
		}
	}))

	t.Run("mkdirat", func(t *testing.T) {
		testatFile, testatFilePtr, err := test.Path("testat-mkdir")
		if err != nil {
			t.Fatal(err)
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno != 0 {
			t.Fatal(error(errno))
		}
		defer syscall.Rmdir(testatFile)

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_mkdirat")

			assert.Equal(t, event.Mkdir.File.Inode, getInode(t, testatFile), "wrong inode")
			assertRights(t, uint16(event.Mkdir.Mode), 0777)
			assert.Equal(t, event.Mkdir.File.Inode, getInode(t, testatFile), "wrong inode")
			assertRights(t, event.Mkdir.File.Mode&expectedMode, expectedMode)

			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "mkdir.file.container_path")
			}
		}
	})
}

func TestMkdirError(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdirat_error",
			Expression: `process.file.name == "{{.ProcessName}}" && mkdir.retval == EACCES`,
		},
	}

	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("mkdirat-error", func(t *testing.T) {
		_, testatFilePtr, err := test.Path("testat2-mkdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chmod(test.Root(), 0711); err != nil {
			t.Fatal(err)
		}

		go func() {
			runtime.LockOSThread()
			// do not unlock, we want the thread to be killed when exiting the goroutine

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREGID, 1, 1, 0); errno != 0 {
				t.Error(err)
			}

			if _, _, errno := syscall.Syscall(syscall.SYS_SETREUID, 1, 1, 0); errno != 0 {
				t.Error(err)
			}

			if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno == 0 {
				t.Error(error(errno))
			}
		}()

		event, rule, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assertTriggeredRule(t, rule, "test_rule_mkdirat_error")
			assertReturnValue(t, event.Mkdir.Retval, -int64(syscall.EACCES))
		}
	})
}
