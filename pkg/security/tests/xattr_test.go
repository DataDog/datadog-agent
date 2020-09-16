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
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestSetXAttr(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `setxattr.filename == "{{.Root}}/test-xattr" && setxattr.namespace == "user" && setxattr.name == "user.test_xattr"`,
	}

	testDrive, err := newTestDrive("ext4", []string{"user_xattr"})
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true, testDir: testDrive.Root()})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := testDrive.Path("test-xattr")
	if err != nil {
		t.Fatal(err)
	}

	xattrName, err := syscall.BytePtrFromString("user.test_xattr")
	if err != nil {
		t.Fatal(err)
	}
	xattrNamePtr := unsafe.Pointer(xattrName)
	xattrValuePtr := unsafe.Pointer(&[]byte{})

	t.Run("setxattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// XATTR_CREATE = 1
		_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, 1, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "setxattr" {
				t.Errorf("expected setxattr event, got %s", event.GetType())
			}

			if event.SetXAttr.Name != "user.test_xattr" || event.SetXAttr.Namespace != "user" {
				t.Errorf("expected setxattr name user.test_xattr, got %s", event.SetXAttr.Name)
			}
		}
	})

	t.Run("lsetxattr", func(t *testing.T) {
		testOldFile, testOldFilePtr, err := testDrive.Path("test-xattr-old")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testOldFile)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testOldFile)

		_, _, errno := syscall.Syscall(syscall.SYS_SYMLINK, uintptr(testOldFilePtr), uintptr(testFilePtr), 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(testFile)

		// XATTR_CREATE = 1
		_, _, errno = syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, 1, 0)
		// Linux and Android don't support xattrs on symlinks according
		// to xattr(7), so just test that we get the proper error.
		if errno != syscall.EACCES && errno != syscall.EPERM {
			t.Fatal(error(errno))
		}

		// We should get the event though
		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "setxattr" {
				t.Errorf("expected setxattr event, got %s", event.GetType())
			}

			if event.SetXAttr.Name != "user.test_xattr" || event.SetXAttr.Namespace != "user" {
				t.Errorf("expected setxattr name user.test_xattr, got %s", event.SetXAttr.Name)
			}
		}
	})

	t.Run("fsetxattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// XATTR_CREATE = 1
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, 1, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "setxattr" {
				t.Errorf("expected setxattr event, got %s", event.GetType())
			}

			if event.SetXAttr.Name != "user.test_xattr" || event.SetXAttr.Namespace != "user" {
				t.Errorf("expected setxattr name user.test_xattr, got %s", event.SetXAttr.Name)
			}
		}
	})
}

func TestRemoveXAttr(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `removexattr.filename == "{{.Root}}/test-xattr" && removexattr.namespace == "user" && removexattr.name == "user.test_xattr"`,
	}

	testDrive, err := newTestDrive("ext4", []string{"user_xattr"})
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := testDrive.Path("test-xattr")
	if err != nil {
		t.Fatal(err)
	}

	xattrName, err := syscall.BytePtrFromString("user.test_xattr")
	if err != nil {
		t.Fatal(err)
	}
	xattrNamePtr := unsafe.Pointer(xattrName)

	t.Run("removexattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), 0, 1, 0, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		// XATTR_CREATE = 1
		_, _, errno = syscall.Syscall(syscall.SYS_REMOVEXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "removexattr" {
				t.Errorf("expected removexattr event, got %s", event.GetType())
			}

			if event.RemoveXAttr.Name != "user.test_xattr" || event.RemoveXAttr.Namespace != "user" {
				t.Errorf("expected removexattr name user.test_xattr, got %s", event.RemoveXAttr.Name)
			}
		}
	})

	t.Run("lremovexattr", func(t *testing.T) {
		testOldFile, testOldFilePtr, err := testDrive.Path("test-xattr-old")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testOldFile)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testOldFile)

		_, _, errno := syscall.Syscall(syscall.SYS_SYMLINK, uintptr(testOldFilePtr), uintptr(testFilePtr), 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(testFile)

		// set xattr
		_, _, errno = syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0, 0, 1, 0)
		// Linux and Android don't support xattrs on symlinks according
		// to xattr(7), so just test that we get the proper error.
		if errno != syscall.EACCES && errno != syscall.EPERM {
			t.Fatal(error(errno))
		}

		// XATTR_CREATE = 1
		_, _, errno = syscall.Syscall(syscall.SYS_LREMOVEXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0)
		// Linux and Android don't support xattrs on symlinks according
		// to xattr(7), so just test that we get the proper error.
		if errno != syscall.EACCES && errno != syscall.EPERM {
			t.Fatal(error(errno))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "removexattr" {
				t.Errorf("expected removexattr event, got %s", event.GetType())
			}

			if event.RemoveXAttr.Name != "user.test_xattr" || event.RemoveXAttr.Namespace != "user" {
				t.Errorf("expected removexattr name user.test_xattr, got %s", event.RemoveXAttr.Name)
			}
		}
	})

	t.Run("fremovexattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), 0, 1, 0, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		// XATTR_CREATE = 1
		_, _, errno = syscall.Syscall(syscall.SYS_FREMOVEXATTR, f.Fd(), uintptr(xattrNamePtr), 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "removexattr" {
				t.Errorf("expected removexattr event, got %s", event.GetType())
			}

			if event.RemoveXAttr.Name != "user.test_xattr" || event.RemoveXAttr.Namespace != "user" {
				t.Errorf("expected removexattr name user.test_xattr, got %s", event.RemoveXAttr.Name)
			}
		}
	})
}
