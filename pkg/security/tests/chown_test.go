// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests,!386

package tests

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestChown(t *testing.T) {
	ruleDef := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
	}

	ruleDef2 := &rules.RuleDefinition{
		ID:         "test_rule2",
		Expression: `chown.file.path == "{{.Root}}/test-symlink" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef, ruleDef2}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	prevUID := 98
	prevGID := 99
	fileMode := 0o447
	expectedMode := uint32(applyUmask(fileMode))
	testFile, testFilePtr, err := test.CreateWithOptions("test-chown", prevUID, prevGID, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fchown", func(t *testing.T) {
		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			f.Close()
			prevUID = 100
			prevGID = 200
		}()

		err = test.GetSignal(t, func() error {
			// fchown syscall
			if _, _, errno := syscall.Syscall(syscall.SYS_FCHOWN, f.Fd(), 100, 200); errno != 0 {
				t.Fatal(err)
			}
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, event.GetType(), "chown", "wrong event type")
			assert.Equal(t, event.Chown.UID, uint32(100), "wrong user")
			assert.Equal(t, event.Chown.GID, uint32(200), "wrong user")
			assert.Equal(t, event.Chown.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, event.Chown.File.UID, uint32(prevUID), "wrong initial user")
			assert.Equal(t, event.Chown.File.GID, uint32(prevGID), "wrong initial group")

			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			if !validateChownSchema(t, event) {
				t.Fatal(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("fchownat", func(t *testing.T) {
		defer func() {
			prevUID = 101
			prevGID = 201
		}()

		err = test.GetSignal(t, func() error {
			if _, _, errno := syscall.Syscall6(syscall.SYS_FCHOWNAT, 0, uintptr(testFilePtr), uintptr(101), uintptr(201), 0x100, 0); errno != 0 {
				t.Fatal(err)
			}
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, event.GetType(), "chown", "wrong event type")
			assert.Equal(t, event.Chown.UID, uint32(101), "wrong user")
			assert.Equal(t, event.Chown.GID, uint32(201), "wrong user")
			assert.Equal(t, event.Chown.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, event.Chown.File.UID, uint32(prevUID), "wrong initial user")
			assert.Equal(t, event.Chown.File.GID, uint32(prevGID), "wrong initial group")

			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			if !validateChownSchema(t, event) {
				t.Fatal(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("lchown", ifSyscallSupported("SYS_LCHOWN", func(t *testing.T, syscallNB uintptr) {
		testSymlink, testSymlinkPtr, err := test.Path("test-symlink")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(testFile, testSymlink); err != nil {
			t.Fatal(err)
		}

		defer os.Remove(testSymlink)

		err = test.GetSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testSymlinkPtr), uintptr(102), uintptr(202)); errno != 0 {
				if errno == unix.ENOSYS {
					t.Skip("lchown is not supported")
				}
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, event.GetType(), "chown", "wrong event type")
			assertTriggeredRule(t, rule, "test_rule2")
			assert.Equal(t, event.Chown.UID, uint32(102), "wrong user")
			assert.Equal(t, event.Chown.GID, uint32(202), "wrong user")
			assert.Equal(t, event.Chown.File.Inode, getInode(t, testSymlink), "wrong inode")
			assertRights(t, event.Chown.File.Mode, 0o777, "wrong initial mode")
			assert.Equal(t, event.Chown.File.UID, uint32(0), "wrong initial user")
			assert.Equal(t, event.Chown.File.GID, uint32(0), "wrong initial group")

			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			if !validateChownSchema(t, event) {
				t.Fatal(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}
	}))

	t.Run("chown", ifSyscallSupported("SYS_CHOWN", func(t *testing.T, syscallNB uintptr) {
		defer func() {
			prevUID = 103
			prevGID = 203
		}()

		err = test.GetSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), 103, 203); errno != 0 {
				t.Fatal(err)
			}
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, event.GetType(), "chown", "wrong event type")
			assert.Equal(t, event.Chown.UID, uint32(103), "wrong user")
			assert.Equal(t, event.Chown.GID, uint32(203), "wrong user")
			assert.Equal(t, event.Chown.File.Inode, getInode(t, testFile), "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, event.Chown.File.UID, uint32(prevUID), "wrong initial user")
			assert.Equal(t, event.Chown.File.GID, uint32(prevGID), "wrong initial group")

			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			if !validateChownSchema(t, event) {
				t.Fatal(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}
	}))
}
