// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestMkdir(t *testing.T) {
	SkipIfNotAvailable(t)

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

	test, err := newTestModule(t, nil, ruleDefs)
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

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(mkdirMode), 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdir")
			assertRights(t, uint16(event.Mkdir.Mode), mkdirMode)
			assertInode(t, event.Mkdir.File.Inode, getInode(t, testFile))
			assertRights(t, event.Mkdir.File.Mode, expectedMode)
			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	}))

	t.Run("mkdirat", func(t *testing.T) {
		testatFile, testatFilePtr, err := test.Path("testat-mkdir")
		if err != nil {
			t.Fatal(err)
		}
		defer syscall.Rmdir(testatFile)

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscall.SYS_MKDIRAT, 0, uintptr(testatFilePtr), uintptr(0777)); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdirat")
			assertRights(t, uint16(event.Mkdir.Mode), 0777)
			assertRights(t, event.Mkdir.File.Mode&expectedMode, expectedMode)
			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)
			assertInode(t, event.Mkdir.File.Inode, getInode(t, testatFile))

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("io_uring", func(t *testing.T) {
		SkipIfNotAvailable(t)

		testatFile, _, err := test.Path("testat-mkdir")
		if err != nil {
			t.Fatal(err)
		}
		defer syscall.Rmdir(testatFile)

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest, err := iouring.Mkdirat(unix.AT_FDCWD, testatFile, 0777)
		if err != nil {
			t.Fatal(err)
		}

		ch := make(chan iouring.Result, 1)

		test.WaitSignal(t, func() error {
			if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			ret, err := result.ReturnInt()
			if err != nil {
				if err == syscall.EBADF || err == syscall.EINVAL {
					return ErrSkipTest{"mkdirat not supported by io_uring"}
				}
				return err
			}

			if ret < 0 {
				return fmt.Errorf("failed to create directory with io_uring: %d", ret)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdirat")
			assert.Equal(t, getInode(t, testatFile), event.Mkdir.File.Inode, "wrong inode")
			assertRights(t, uint16(event.Mkdir.Mode), 0777)
			assert.Equal(t, getInode(t, testatFile), event.Mkdir.File.Inode, "wrong inode")
			assertRights(t, event.Mkdir.File.Mode&expectedMode, expectedMode)
			assertNearTime(t, event.Mkdir.File.MTime)
			assertNearTime(t, event.Mkdir.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), true)

			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			assertFieldEqual(t, event, "process.file.path", executable)
		})
	})
}

func TestMkdirError(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_mkdirat_error",
			Expression: `process.file.name == "syscall_tester" && mkdir.retval == EACCES`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("mkdirat-error", func(t *testing.T) {
		testatFile, _, err := test.Path("testat2-mkdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chmod(test.Root(), 0711); err != nil {
			t.Fatal(err)
		}

		test.WaitSignal(t, func() error {
			return runSyscallTesterFunc(context.Background(), t, syscallTester, "mkdirat-error", testatFile)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdirat_error")
			assert.Equal(t, event.Mkdir.Retval, -int64(syscall.EACCES))
		})
	})
}
