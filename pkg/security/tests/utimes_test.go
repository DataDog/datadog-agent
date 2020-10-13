// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestUtime(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `utimes.filename == "{{.Root}}/test-utime"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{ruleDef}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-utime")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	t.Run("utime", func(t *testing.T) {
		utimbuf := &syscall.Utimbuf{
			Actime:  123,
			Modtime: 456,
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_UTIME, uintptr(testFilePtr), uintptr(unsafe.Pointer(utimbuf)), 0); errno != 0 {
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "utimes" {
				t.Errorf("expected utimes event, got %s", event.GetType())
			}

			if atime := event.Utimes.Atime.Unix(); atime != 123 {
				t.Errorf("expected access time of 123, got %d", atime)
			}

			if mtime := event.Utimes.Mtime.Unix(); mtime != 456 {
				t.Errorf("expected modification time of 456, got %d", mtime)
			}
		}
	})

	t.Run("utimes", func(t *testing.T) {
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

		if _, _, errno := syscall.Syscall(syscall.SYS_UTIMES, uintptr(testFilePtr), uintptr(unsafe.Pointer(&times[0])), 0); errno != 0 {
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "utimes" {
				t.Errorf("expected utimes event, got %s", event.GetType())
			}

			if atime := event.Utimes.Atime.Unix(); atime != 111 {
				t.Errorf("expected access time of 111, got %d", atime)
			}

			if atime := event.Utimes.Atime.UnixNano(); atime%int64(time.Second)/int64(time.Microsecond) != 222 {
				t.Errorf("expected access microseconds of 222, got %d", atime%int64(time.Second)/int64(time.Microsecond))
			}
		}
	})

	t.Run("utimensat", func(t *testing.T) {
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

		if _, _, errno := syscall.Syscall(syscall.SYS_UTIMENSAT, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&ntimes[0]))); errno != 0 {
			if errno == syscall.EINVAL {
				t.Skip("utimensat not supported")
			}
			t.Fatal(errno)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "utimes" {
				t.Errorf("expected utimes event, got %s", event.GetType())
			}

			if mtime := event.Utimes.Mtime.Unix(); mtime != 555 {
				t.Errorf("expected modification time of 555, got %d", mtime)
			}

			if mtime := event.Utimes.Mtime.UnixNano(); mtime%int64(time.Second)/int64(time.Nanosecond) != 666 {
				t.Errorf("expected modification microseconds of 666, got %d (%d)", mtime%int64(time.Second)/int64(time.Nanosecond), mtime)
			}
		}
	})
}
