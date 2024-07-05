// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestUtimes(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `utimes.file.path == "{{.Root}}/test-utime" && utimes.file.uid == 98 && utimes.file.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("utime", ifSyscallSupported("SYS_UTIME", func(t *testing.T, syscallNB uintptr) {
		fileMode := 0o447
		expectedMode := uint16(applyUmask(fileMode))
		testFile, testFilePtr, err := test.CreateWithOptions("test-utime", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		utimbuf := &syscall.Utimbuf{
			Actime:  123,
			Modtime: 456,
		}

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "utimes", event.GetType(), "wrong event type")
			assert.Equal(t, int64(123), event.Utimes.Atime.Unix())
			assert.Equal(t, int64(456), event.Utimes.Mtime.Unix())
			assertInode(t, event.Utimes.File.Inode, getInode(t, testFile))
			assertRights(t, event.Utimes.File.Mode, expectedMode)
			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	}))

	t.Run("utimes", ifSyscallSupported("SYS_UTIMES", func(t *testing.T, syscallNB uintptr) {
		fileMode := 0o447
		expectedMode := uint16(applyUmask(fileMode))
		testFile, testFilePtr, err := test.CreateWithOptions("test-utime", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		var times = [2]syscall.Timeval{
			{
				Sec:  111,
				Usec: 222,
			},
			{
				Sec:  333,
				Usec: 444,
			},
		}

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(&times[0])), 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "utimes", event.GetType(), "wrong event type")
			assert.Equal(t, int64(111), event.Utimes.Atime.Unix())
			assert.Equal(t, int64(222), event.Utimes.Atime.UnixNano()%int64(time.Second)/int64(time.Microsecond))
			assertInode(t, event.Utimes.File.Inode, getInode(t, testFile))
			assertRights(t, event.Utimes.File.Mode, expectedMode)
			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	}))

	t.Run("utimensat", func(t *testing.T) {
		fileMode := 0o447
		expectedMode := uint16(applyUmask(fileMode))
		testFile, testFilePtr, err := test.CreateWithOptions("test-utime", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		var ntimes = [2]syscall.Timespec{
			{
				Sec:  111,
				Nsec: 222,
			},
			{
				Sec:  555,
				Nsec: 666,
			},
		}

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&ntimes[0])), 0, 0, 0); errno != 0 {
				if errno == syscall.EINVAL {
					return ErrSkipTest{"utimensat not supported"}
				}
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "utimes", event.GetType(), "wrong event type")
			assert.Equal(t, int64(555), event.Utimes.Mtime.Unix())
			assert.Equal(t, int64(666), event.Utimes.Mtime.UnixNano()%int64(time.Second)/int64(time.Nanosecond))
			assertInode(t, event.Utimes.File.Inode, getInode(t, testFile))
			assertRights(t, event.Utimes.File.Mode, expectedMode)
			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("utimensat-nil", func(t *testing.T) {
		fileMode := 0o447
		testFile, _, err := test.CreateWithOptions("test-utime", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)
		file, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		fd := file.Fd()
		defer file.Close()

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, fd, 0, 0, 0, 0, 0); errno != 0 {
				if errno == syscall.EINVAL {
					return ErrSkipTest{"utimensat not supported"}
				}
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "utimes", event.GetType(), "wrong event type")
			assertNearTime(t, uint64(event.Utimes.Mtime.UnixNano()))
			assertNearTime(t, uint64(event.Utimes.Atime.UnixNano()))
			assertInode(t, event.Utimes.File.Inode, getInode(t, testFile))
			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})
}
