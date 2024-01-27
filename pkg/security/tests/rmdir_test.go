// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
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

func TestRmdir(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rmdir.file.path in ["{{.Root}}/test-rmdir", "{{.Root}}/test-unlink-rmdir"] && rmdir.file.uid == 0 && rmdir.file.gid == 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), 0, 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rmdir", event.GetType(), "wrong event type")
			assertInode(t, event.Rmdir.File.Inode, inode)
			assertRights(t, event.Rmdir.File.Mode, expectedMode, "wrong initial mode")
			assertNearTime(t, event.Rmdir.File.MTime)
			assertNearTime(t, event.Rmdir.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
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

		test.WaitSignal(t, func() error {
			if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testDirPtr), 512); err != 0 {
				return error(err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rmdir", event.GetType(), "wrong event type")
			assertInode(t, event.Rmdir.File.Inode, inode)
			assertRights(t, event.Rmdir.File.Mode, expectedMode, "wrong initial mode")
			assertNearTime(t, event.Rmdir.File.MTime)
			assertNearTime(t, event.Rmdir.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("unlinkat-io_uring", func(t *testing.T) {
		SkipIfNotAvailable(t)

		testDir, _, err := test.Path("test-unlink-rmdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testDir, uint32(mkdirMode)); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testDir)

		inode := getInode(t, testDir)

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest, err := iouring.Unlinkat(unix.AT_FDCWD, testDir, unix.AT_REMOVEDIR)
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
					return ErrSkipTest{"unlinkat not supported by io_uring"}
				}
				return err
			}

			if ret < 0 {
				return fmt.Errorf("failed to unlink file with io_uring: %d", ret)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rmdir", event.GetType(), "wrong event type")
			assert.Equal(t, inode, event.Rmdir.File.Inode, "wrong inode")
			assertRights(t, event.Rmdir.File.Mode, expectedMode, "wrong initial mode")
			assertNearTime(t, event.Rmdir.File.MTime)
			assertNearTime(t, event.Rmdir.File.CTime)

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

func TestRmdirInvalidate(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rmdir.file.path =~ "{{.Root}}/test-rmdir-*"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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

		test.WaitSignal(t, func() error {
			return syscall.Rmdir(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rmdir", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "rmdir.file.path", testFile)
		})
	}
}
