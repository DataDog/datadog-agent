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

func TestLink(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `link.file.path == "{{.Root}}/test-link" && link.file.destination.path == "{{.Root}}/test2-link" && link.file.uid == 98 && link.file.gid == 99 && link.file.destination.uid == 98 && link.file.destination.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
	testOldFile, testOldFilePtr, err := test.CreateWithOptions("test-link", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	testNewFile, testNewFilePtr, err := test.Path("test2-link")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("link", ifSyscallSupported("SYS_LINK", func(t *testing.T, syscallNB uintptr) {
		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall(syscallNB, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "link", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Link.Source.Inode, "wrong inode")
			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))
			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateLinkSchema(t, event)
		})

		if err = os.Remove(testNewFile); err != nil {
			t.Fatal(err)
		}
	}))

	t.Run("linkat", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_LINKAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "link", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Link.Source.Inode, "wrong inode")
			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))
			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateLinkSchema(t, event)
		})

		if err = os.Remove(testNewFile); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("io_uring", func(t *testing.T) {
		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest, err := iouring.Linkat(unix.AT_FDCWD, testOldFile, unix.AT_FDCWD, testNewFile, 0)
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
					return ErrSkipTest{"linkat not supported by io_uring"}
				}
				return err
			}

			if ret < 0 {
				return fmt.Errorf("failed to create a link with io_uring: %d", ret)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "link", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Link.Source.Inode, "wrong inode")
			assertRights(t, event.Link.Source.Mode, uint16(expectedMode))
			assertRights(t, event.Link.Target.Mode, uint16(expectedMode))
			assertNearTime(t, event.Link.Source.MTime)
			assertNearTime(t, event.Link.Source.CTime)
			assertNearTime(t, event.Link.Target.MTime)
			assertNearTime(t, event.Link.Target.CTime)

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
