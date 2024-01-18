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
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSetXAttr(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `((setxattr.file.path == "{{.Root}}/test-setxattr" && setxattr.file.uid == 98 && setxattr.file.gid == 99) || setxattr.file.path == "{{.Root}}/test-setxattr-link") && setxattr.file.destination.namespace == "user" && setxattr.file.destination.name == "user.test_xattr"`,
	}

	testDrive, err := newTestDrive(t, "ext4", []string{"user_xattr"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	xattrName, err := syscall.BytePtrFromString("user.test_xattr")
	if err != nil {
		t.Fatal(err)
	}
	xattrNamePtr := unsafe.Pointer(xattrName)
	xattrValuePtr := unsafe.Pointer(&[]byte{})

	fileMode := 0o777
	expectedMode := uint16(applyUmask(fileMode))

	t.Run("setxattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.CreateWithOptions("test-setxattr", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "setxattr", event.GetType(), "wrong event type")
			assert.Equal(t, "user.test_xattr", event.SetXAttr.Name)
			assert.Equal(t, "user", event.SetXAttr.Namespace)
			assert.Equal(t, getInode(t, testFile), event.SetXAttr.File.Inode, "wrong inode")
			assertRights(t, event.SetXAttr.File.Mode, expectedMode)
			assertNearTime(t, event.SetXAttr.File.MTime)
			assertNearTime(t, event.SetXAttr.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("lsetxattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-setxattr-link")
		if err != nil {
			t.Fatal(err)
		}

		testOldFile, _, err := test.CreateWithOptions("test-setxattr-old", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testOldFile)

		if err := os.Symlink(testOldFile, testFile); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
			// Linux and Android don't support xattrs on symlinks according
			// to xattr(7), so just test that we get the proper error.
			// We should get the event though
			if errno != syscall.EACCES && errno != syscall.EPERM {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "setxattr", event.GetType(), "wrong event type")
			assert.Equal(t, "user.test_xattr", event.SetXAttr.Name)
			assert.Equal(t, "user", event.SetXAttr.Namespace)
			assert.Equal(t, getInode(t, testFile), event.SetXAttr.File.Inode, "wrong inode")
			assertRights(t, event.SetXAttr.File.Mode, 0777)
			assertNearTime(t, event.SetXAttr.File.MTime)
			assertNearTime(t, event.SetXAttr.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("fsetxattr", func(t *testing.T) {
		testFile, _, err := test.CreateWithOptions("test-setxattr", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "setxattr", event.GetType(), "wrong event type")
			assert.Equal(t, "user.test_xattr", event.SetXAttr.Name)
			assert.Equal(t, "user", event.SetXAttr.Namespace)
			assert.Equal(t, getInode(t, testFile), event.SetXAttr.File.Inode, "wrong inode")
			assertRights(t, event.SetXAttr.File.Mode, expectedMode)
			assertNearTime(t, event.SetXAttr.File.MTime)
			assertNearTime(t, event.SetXAttr.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})
}

func TestRemoveXAttr(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `((removexattr.file.path == "{{.Root}}/test-removexattr" && removexattr.file.uid == 98 && removexattr.file.gid == 99) || removexattr.file.path == "{{.Root}}/test-removexattr-link") && removexattr.file.destination.namespace == "user" && removexattr.file.destination.name == "user.test_xattr" `,
		},
	}

	testDrive, err := newTestDrive(t, "ext4", []string{"user_xattr"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	xattrName, err := syscall.BytePtrFromString("user.test_xattr")
	if err != nil {
		t.Fatal(err)
	}
	xattrNamePtr := unsafe.Pointer(xattrName)

	fileMode := 0o777
	expectedMode := applyUmask(fileMode)

	t.Run("removexattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.CreateWithOptions("test-removexattr", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), 0, 0, 1, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		test.WaitSignal(t, func() error {
			_, _, errno = syscall.Syscall(syscall.SYS_REMOVEXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "removexattr", event.GetType(), "wrong event type")
			assert.Equal(t, "user.test_xattr", event.RemoveXAttr.Name)
			assert.Equal(t, getInode(t, testFile), event.RemoveXAttr.File.Inode, "wrong inode")
			assertRights(t, event.RemoveXAttr.File.Mode, uint16(expectedMode))
			assertNearTime(t, event.RemoveXAttr.File.MTime)
			assertNearTime(t, event.RemoveXAttr.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("lremovexattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-removexattr-link")
		if err != nil {
			t.Fatal(err)
		}

		testOldFile, _, err := test.CreateWithOptions("test-setxattr-old", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testOldFile)

		if err := os.Symlink(testOldFile, testFile); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0, 0, 1, 0)
		// Linux and Android don't support xattrs on symlinks according
		// to xattr(7), so just test that we get the proper error.
		if errno != syscall.EACCES && errno != syscall.EPERM {
			t.Fatal(error(errno))
		}

		test.WaitSignal(t, func() error {
			_, _, errno = syscall.Syscall(syscall.SYS_LREMOVEXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0)
			// Linux and Android don't support xattrs on symlinks according
			// to xattr(7), so just test that we get the proper error.
			if errno != syscall.EACCES && errno != syscall.EPERM {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "removexattr", event.GetType(), "wrong event type")
			assert.Equal(t, "user.test_xattr", event.RemoveXAttr.Name)
			assert.Equal(t, getInode(t, testFile), event.RemoveXAttr.File.Inode, "wrong inode")
			assertRights(t, event.RemoveXAttr.File.Mode, 0777)
			assertNearTime(t, event.RemoveXAttr.File.MTime)
			assertNearTime(t, event.RemoveXAttr.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("fremovexattr", func(t *testing.T) {
		testFile, _, err := test.CreateWithOptions("test-removexattr", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), 0, 0, 1, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		test.WaitSignal(t, func() error {
			_, _, errno = syscall.Syscall(syscall.SYS_FREMOVEXATTR, f.Fd(), uintptr(xattrNamePtr), 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			if event.GetType() != "removexattr" {
				t.Errorf("expected removexattr event, got %s", event.GetType())
			}

			if event.RemoveXAttr.Name != "user.test_xattr" || event.RemoveXAttr.Namespace != "user" {
				t.Errorf("expected removexattr name user.test_xattr, got %s", event.RemoveXAttr.Name)
			}

			if inode := getInode(t, testFile); inode != event.RemoveXAttr.File.Inode {
				t.Errorf("expected inode %d, got %d", event.RemoveXAttr.File.Inode, inode)
			}

			if int(event.RemoveXAttr.File.Mode)&expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.RemoveXAttr.File.Mode)&expectedMode)
			}

			assertNearTime(t, event.RemoveXAttr.File.MTime)
			assertNearTime(t, event.RemoveXAttr.File.CTime)
		})
	})
}
