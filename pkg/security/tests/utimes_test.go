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
	"time"
	"unsafe"

	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestUtime(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `utimes.file.path == "{{.Root}}/test-utime" && utimes.file.uid == 98 && utimes.file.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
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

		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "utimes", "wrong event type")
			assert.Equal(t, event.Utimes.Atime.Unix(), int64(123))
			assert.Equal(t, event.Utimes.Mtime.Unix(), int64(456))
			assert.Equal(t, event.Utimes.File.Inode, getInode(t, testFile), "wrong inode")

			assertRights(t, uint16(event.Utimes.File.Mode), expectedMode)

			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "utimes.file.container_path")
			}
		}
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

		if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(unsafe.Pointer(&times[0])), 0); errno != 0 {
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "utimes", "wrong event type")
			assert.Equal(t, event.Utimes.Atime.Unix(), int64(111))

			assert.Equal(t, event.Utimes.Atime.UnixNano()%int64(time.Second)/int64(time.Microsecond), int64(222))
			assert.Equal(t, event.Utimes.File.Inode, getInode(t, testFile))
			assertRights(t, uint16(event.Utimes.File.Mode), expectedMode)

			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "utimes.file.container_path")
			}
		}
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

		if _, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&ntimes[0])), 0, 0, 0); errno != 0 {
			if errno == syscall.EINVAL {
				t.Skip("utimensat not supported")
			}
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.GetType(), "utimes", "wrong event type")
			assert.Equal(t, event.Utimes.Mtime.Unix(), int64(555))

			assert.Equal(t, event.Utimes.Mtime.UnixNano()%int64(time.Second)/int64(time.Nanosecond), int64(666))
			assert.Equal(t, event.Utimes.File.Inode, getInode(t, testFile))
			assertRights(t, uint16(event.Utimes.File.Mode), expectedMode)

			assertNearTime(t, event.Utimes.File.MTime)
			assertNearTime(t, event.Utimes.File.CTime)

			if testEnvironment == DockerEnvironment {
				testContainerPath(t, event, "utimes.file.container_path")
			}
		}
	})
}
