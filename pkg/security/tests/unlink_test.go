// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

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

func TestUnlink(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.file.path in ["{{.Root}}/test-unlink", "{{.Root}}/test-unlinkat"] && unlink.file.uid == 98 && unlink.file.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint16(applyUmask(fileMode))
	testFile, testFilePtr, err := test.CreateWithOptions("test-unlink", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	inode := getInode(t, testFile)

	t.Run("unlink", ifSyscallSupported("SYS_UNLINK", func(t *testing.T, syscallNB uintptr) {
		test.WaitSignal(t, func() error {
			if _, _, err := syscall.Syscall(syscallNB, uintptr(testFilePtr), 0, 0); err != 0 {
				return error(err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assertInode(t, event.Unlink.File.Inode, inode)
			assertRights(t, event.Unlink.File.Mode, expectedMode)
			assertNearTime(t, event.Unlink.File.MTime)
			assertNearTime(t, event.Unlink.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	}))

	testAtFile, testAtFilePtr, err := test.CreateWithOptions("test-unlinkat", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testAtFile)

	inode = getInode(t, testAtFile)

	t.Run("unlinkat", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(testAtFilePtr), 0); err != 0 {
				return error(err)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assertInode(t, event.Unlink.File.Inode, inode)
			assertRights(t, event.Unlink.File.Mode, expectedMode)
			assertNearTime(t, event.Unlink.File.MTime)
			assertNearTime(t, event.Unlink.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	testAtFile, testAtFilePtr, err = test.CreateWithOptions("test-unlinkat", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testAtFile)

	inode = getInode(t, testAtFile)

	t.Run("io_uring", func(t *testing.T) {
		SkipIfNotAvailable(t)

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest, err := iouring.Unlinkat(unix.AT_FDCWD, testAtFile, 0)
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
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong inode")
			assertRights(t, event.Unlink.File.Mode, expectedMode)
			assertNearTime(t, event.Unlink.File.MTime)
			assertNearTime(t, event.Unlink.File.CTime)

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

func TestUnlinkInvalidate(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.file.path =~ "{{.Root}}/test-unlink-*"`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for i := 0; i != 5; i++ {
		filename := fmt.Sprintf("test-unlink-%d", i)

		testFile, _, err := test.Path(filename)
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "unlink", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "unlink.file.path", testFile)
		})
	}
}
