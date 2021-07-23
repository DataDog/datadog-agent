// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestMkdir(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdir",
			Expression: `mkdir.file.path == "{{.Root}}/test-mkdir" && mkdir.file.uid == 0 && mkdir.file.gid == 0`,
		},
		{
			ID:         "test_rule_mkdirat",
			Expression: `mkdir.file.path == "{{.Root}}/testat-mkdir" && mkdir.file.uid == 0 && mkdir.file.gid == 0`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
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
		defer syscall.Rmdir(testFile)

		err = test.GetSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(mkdirMode), 0); errno != 0 {
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdir")
			assertRights(t, uint16(event.Mkdir.Mode), mkdirMode)
			assert.Equal(t, getInode(t, testFile), event.Mkdir.File.Inode, "wrong inode")
			assertRights(t, event.Mkdir.File.Mode, expectedMode)

			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)
		})
	}))

	t.Run("mkdirat", func(t *testing.T) {
		testatFile, testatFilePtr, err := test.Path("testat-mkdir")
		if err != nil {
			t.Fatal(err)
		}
		defer syscall.Rmdir(testatFile)

		err = test.GetSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno != 0 {
				t.Fatal(error(errno))
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdirat")

			assert.Equal(t, getInode(t, testatFile), event.Mkdir.File.Inode, "wrong inode")
			assertRights(t, uint16(event.Mkdir.Mode), 0777)
			assert.Equal(t, getInode(t, testatFile), event.Mkdir.File.Inode, "wrong inode")
			assertRights(t, event.Mkdir.File.Mode&expectedMode, expectedMode)

			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)
		})
	})
}

func TestMkdirError(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdirat_error",
			Expression: `process.file.name == "{{.ProcessName}}" && mkdir.retval == EACCES`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
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

		err = test.GetSignal(t, func() error {
			var wg sync.WaitGroup
			wg.Add(1)

			go func() {
				defer wg.Done()

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

			wg.Wait()
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdirat_error")
			assertReturnValue(t, event.Mkdir.Retval, -int64(syscall.EACCES))
		})
		if err != nil {
			t.Error(err)
		}
	})
}
