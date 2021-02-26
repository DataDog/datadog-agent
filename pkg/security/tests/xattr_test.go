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

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"golang.org/x/sys/unix"
)

func TestSetXAttr(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `((setxattr.file.path == "{{.Root}}/test-setxattr" && setxattr.file.uid == 98 && setxattr.file.gid == 99) || setxattr.file.path == "{{.Root}}/test-setxattr-link") && setxattr.file.destination.namespace == "user" && setxattr.file.destination.name == "user.test_xattr"`,
	}

	testDrive, err := newTestDrive("ext4", []string{"user_xattr"})
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{testDir: testDrive.Root()})
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
	expectedMode := applyUmask(fileMode)

	t.Run("setxattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.CreateWithOptions("test-setxattr", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
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

			if inode := getInode(t, testFile); inode != event.SetXAttr.File.Inode {
				t.Logf("expected inode %d, got %d", event.SetXAttr.File.Inode, inode)
			}

			if int(event.SetXAttr.File.Mode) & expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.SetXAttr.File.Mode) & expectedMode)
			}

			now := time.Now()
			if event.SetXAttr.File.MTime.After(now) || event.SetXAttr.File.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.SetXAttr.File.MTime)
			}

			if event.SetXAttr.File.CTime.After(now) || event.SetXAttr.File.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.SetXAttr.File.CTime)
			}

			testContainerPath(t, event, "setxattr.file.container_path")
		}
	})

	t.Run("lsetxattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-setxattr-link")
		if err != nil {
			t.Fatal(err)
		}

		testOldFile, testOldFilePtr, err := test.CreateWithOptions("test-setxattr-old", 98, 99, fileMode)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testOldFile)

		_, _, errno := syscall.Syscall(syscall.SYS_SYMLINK, uintptr(testOldFilePtr), uintptr(testFilePtr), 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(testFile)

		_, _, errno = syscall.Syscall6(syscall.SYS_LSETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
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

		_, _, errno := syscall.Syscall6(syscall.SYS_FSETXATTR, f.Fd(), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
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

			if inode := getInode(t, testFile); inode != event.SetXAttr.File.Inode {
				t.Logf("expected inode %d, got %d", event.SetXAttr.File.Inode, inode)
			}

			if int(event.SetXAttr.File.Mode) & expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.SetXAttr.File.Mode) & expectedMode)
			}

			now := time.Now()
			if event.SetXAttr.File.MTime.After(now) || event.SetXAttr.File.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.SetXAttr.File.MTime)
			}

			if event.SetXAttr.File.CTime.After(now) || event.SetXAttr.File.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.SetXAttr.File.CTime)
			}

			testContainerPath(t, event, "setxattr.file.container_path")
		}
	})
}

func TestRemoveXAttr(t *testing.T) {
	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule",
			Expression: `((removexattr.file.path == "{{.Root}}/test-removexattr" && removexattr.file.uid == 98 && removexattr.file.gid == 99) || removexattr.file.path == "{{.Root}}/test-removexattr-link") && removexattr.file.destination.namespace == "user" && removexattr.file.destination.name == "user.test_xattr" `,
		},
	}

	testDrive, err := newTestDrive("ext4", []string{"user_xattr"})
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(nil, rules, testOpts{testDir: testDrive.Root()})
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

			if inode := getInode(t, testFile); inode != event.RemoveXAttr.File.Inode {
				t.Logf("expected inode %d, got %d", event.RemoveXAttr.File.Inode, inode)
			}

			if int(event.RemoveXAttr.File.Mode) & expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.RemoveXAttr.File.Mode) & expectedMode)
			}

			now := time.Now()
			if event.RemoveXAttr.File.MTime.After(now) || event.RemoveXAttr.File.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.RemoveXAttr.File.MTime)
			}

			if event.RemoveXAttr.File.CTime.After(now) || event.RemoveXAttr.File.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.RemoveXAttr.File.CTime)
			}

			testContainerPath(t, event, "removexattr.file.container_path")
		}
	})

	t.Run("lremovexattr", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("test-removexattr-link")
		if err != nil {
			t.Fatal(err)
		}

		testOldFile, testOldFilePtr, err := test.CreateWithOptions("test-setxattr-old", 98, 99, fileMode)
		if err != nil {
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

			if inode := getInode(t, testFile); inode != event.RemoveXAttr.File.Inode {
				t.Logf("expected inode %d, got %d", event.RemoveXAttr.File.Inode, inode)
			}

			if int(event.RemoveXAttr.File.Mode) & expectedMode != expectedMode {
				t.Errorf("expected initial mode %d, got %d", expectedMode, int(event.RemoveXAttr.File.Mode) & expectedMode)
			}

			now := time.Now()
			if event.RemoveXAttr.File.MTime.After(now) || event.RemoveXAttr.File.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.RemoveXAttr.File.MTime)
			}

			if event.RemoveXAttr.File.CTime.After(now) || event.RemoveXAttr.File.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.RemoveXAttr.File.CTime)
			}

			testContainerPath(t, event, "removexattr.file.container_path")
		}
	})
}
