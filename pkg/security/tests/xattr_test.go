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

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-xattr")
	if err != nil {
		t.Fatal(err)
	}

	xattrName, err := syscall.BytePtrFromString("user.test_xattr")
	if err != nil {
		t.Fatal(err)
	}
	xattrNamePtr := unsafe.Pointer(xattrName)
	xattrValuePtr := unsafe.Pointer(&[]byte{})

	testSetXAttr(t, testFile, testFilePtr, test, xattrNamePtr, xattrValuePtr)
	testLSetXAttr(t, testFile, testFilePtr, test, xattrNamePtr, xattrValuePtr)
	testFSetXAttr(t, testFile, testFilePtr, test, xattrNamePtr, xattrValuePtr)
}

func testSetXAttr(t *testing.T, testFile string, testFilePtr unsafe.Pointer, test *testModule, xattrNamePtr unsafe.Pointer, xattrValuePtr unsafe.Pointer) {
	t.Run("setxattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// XATTR_CREATE = 1
		_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 1, 0, 0)
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

func testLSetXAttr(t *testing.T, testFile string, testFilePtr unsafe.Pointer, test *testModule, xattrNamePtr unsafe.Pointer, xattrValuePtr unsafe.Pointer) {
	t.Run("lsetxattr", func(t *testing.T) {
		testOldFile, testOldFilePtr, err := test.Path("test-xattr-old")
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

		_, _, errno := syscall.Syscall(syscall.SYS_LINK, uintptr(testOldFilePtr), uintptr(testFilePtr), 0)
		if errno != 0 {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		// XATTR_CREATE = 1
		_, _, errno = syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 1, 0, 0)
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

func testFSetXAttr(t *testing.T, testFile string, testFilePtr unsafe.Pointer, test *testModule, xattrNamePtr unsafe.Pointer, xattrValuePtr unsafe.Pointer) {
	t.Run("fsetxattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// XATTR_CREATE = 1
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 1, 0, 0)
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

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-xattr")
	if err != nil {
		t.Fatal(err)
	}

	xattrName, err := syscall.BytePtrFromString("user.test_xattr")
	if err != nil {
		t.Fatal(err)
	}
	xattrNamePtr := unsafe.Pointer(xattrName)
	xattrValuePtr := unsafe.Pointer(&[]byte{})

	testRemoveXAttr(t, testFile, testFilePtr, test, xattrNamePtr, xattrValuePtr)
	testLRemoveXAttr(t, testFile, testFilePtr, test, xattrNamePtr, xattrValuePtr)
	testFRemoveXAttr(t, testFile, testFilePtr, test, xattrNamePtr, xattrValuePtr)
}

func testRemoveXAttr(t *testing.T, testFile string, testFilePtr unsafe.Pointer, test *testModule, xattrNamePtr unsafe.Pointer, xattrValuePtr unsafe.Pointer) {
	t.Run("removexattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 1, 0, 0)
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
}

func testLRemoveXAttr(t *testing.T, testFile string, testFilePtr unsafe.Pointer, test *testModule, xattrNamePtr unsafe.Pointer, xattrValuePtr unsafe.Pointer) {
	t.Run("lremovexattr", func(t *testing.T) {
		testOldFile, testOldFilePtr, err := test.Path("test-xattr-old")
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

		_, _, errno := syscall.Syscall(syscall.SYS_LINK, uintptr(testOldFilePtr), uintptr(testFilePtr), 0)
		if errno != 0 {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		// set xattr
		_, _, errno = syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 1, 0, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		// XATTR_CREATE = 1
		_, _, errno = syscall.Syscall(syscall.SYS_LREMOVEXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), 0)
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

func testFRemoveXAttr(t *testing.T, testFile string, testFilePtr unsafe.Pointer, test *testModule, xattrNamePtr unsafe.Pointer, xattrValuePtr unsafe.Pointer) {
	t.Run("fremovexattr", func(t *testing.T) {
		// create file
		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		defer os.Remove(testFile)

		// set xattr
		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 1, 0, 0)
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
