// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests && !386

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestChown(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
		},
		{
			ID:         "test_rule2",
			Expression: `chown.file.path == "{{.Root}}/test-symlink" && chown.file.destination.uid in [100, 101, 102, 103] && chown.file.destination.gid in [200, 201, 202, 203]`,
		},
		{
			ID:         "test_rule3",
			Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid == 104 && chown.file.destination.gid == -1`,
		},
		{
			ID:         "test_rule4",
			Expression: `chown.file.path == "{{.Root}}/test-chown" && chown.file.destination.uid == -1 && chown.file.destination.gid == 204`,
		},
		{
			ID:         "test_rule5",
			Expression: `chown.file.destination.uid == 100 && chown.file.destination.gid == 200 && process.file.name == "syscall_tester"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
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

		test.WaitSignal(t, func() error {
			// fchown syscall
			if _, _, errno := syscall.Syscall(syscall.SYS_FCHOWN, f.Fd(), 100, 200); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(100), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(200), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	})

	t.Run("fchownat", func(t *testing.T) {
		defer func() {
			prevUID = 101
			prevGID = 201
		}()

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall6(syscall.SYS_FCHOWNAT, 0, uintptr(testFilePtr), uintptr(101), uintptr(201), 0x100, 0); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(101), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(201), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
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

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testSymlinkPtr), uintptr(102), uintptr(202)); errno != 0 {
				if errno == unix.ENOSYS {
					return ErrSkipTest{"lchown is not supported"}
				}
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, rule, "test_rule2")
			assert.Equal(t, int64(102), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(202), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testSymlink), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, 0o777, "wrong initial mode")
			assert.Equal(t, uint32(0), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(0), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	}))

	t.Run("chown", ifSyscallSupported("SYS_CHOWN", func(t *testing.T, syscallNB uintptr) {
		defer func() { prevUID, prevGID = 103, 203 }()

		test.WaitSignal(t, func() error {
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), 103, 203); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assert.Equal(t, int64(103), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(203), event.Chown.GID, "wrong user")
			assert.Equal(t, getInode(t, testFile), event.Chown.File.Inode, "wrong inode")
			assertRights(t, event.Chown.File.Mode, uint16(expectedMode), "wrong initial mode")
			assert.Equal(t, uint32(prevUID), event.Chown.File.UID, "wrong initial user")
			assert.Equal(t, uint32(prevGID), event.Chown.File.GID, "wrong initial group")
			assertNearTime(t, event.Chown.File.MTime)
			assertNearTime(t, event.Chown.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	}))

	t.Run("chown-no-group", ifSyscallSupported("SYS_CHOWN", func(t *testing.T, syscallNB uintptr) {
		defer func() { prevUID = 104 }()

		test.WaitSignal(t, func() error {
			gid := -1
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), 104, uintptr(gid)); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, r, "test_rule3")
			assert.Equal(t, int64(104), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(-1), event.Chown.GID, "wrong group")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	}))

	t.Run("chown-no-user", ifSyscallSupported("SYS_CHOWN", func(t *testing.T, syscallNB uintptr) {
		defer func() { prevGID = 204 }()

		test.WaitSignal(t, func() error {
			uid := -1
			if _, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), uintptr(uid), 204); errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "chown", event.GetType(), "wrong event type")
			assertTriggeredRule(t, r, "test_rule4")
			assert.Equal(t, int64(-1), event.Chown.UID, "wrong user")
			assert.Equal(t, int64(204), event.Chown.GID, "wrong group")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			test.validateChownSchema(t, event)
		})
	}))

	test.Run(t, "pipe-chown-discarded", func(t *testing.T, kind wrapperType, cmdFunc func(cmd string, args []string, envs []string) *exec.Cmd) {
		_ = test.GetSignal(t, func() error {
			syscallTester, err := loadSyscallTester(t, test, "syscall_tester")
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"pipe-chown"}
			cmd := cmdFunc(syscallTester, args, []string{})
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			t.Error("Event received")
		})
	})

}
