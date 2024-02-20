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
	"path"
	"syscall"
	"testing"

	"github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestRename(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.file.path == "{{.Root}}/test-rename" && rename.file.uid == 98 && rename.file.gid == 99 && rename.file.destination.path == "{{.Root}}/test2-rename" && rename.file.destination.uid == 98 && rename.file.destination.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint16(applyUmask(fileMode))
	testOldFile, testOldFilePtr, err := test.CreateWithOptions("test-rename", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	testNewFile, testNewFilePtr, err := test.Path("test2-rename")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testNewFile)
	defer os.Remove(testOldFile)

	renameSyscallIsSupported := false
	t.Run("rename", ifSyscallSupported("SYS_RENAME", func(t *testing.T, syscallNB uintptr) {
		renameSyscallIsSupported = true

		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall(syscallNB, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assertInode(t, event.Rename.New.Inode, getInode(t, testNewFile))
			test.validateRenameSchema(t, event)
			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)
			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	}))

	if renameSyscallIsSupported {
		if err := os.Rename(testNewFile, testOldFile); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("renameat", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_RENAMEAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assertInode(t, event.Rename.New.Inode, getInode(t, testNewFile))
			test.validateRenameSchema(t, event)
			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)
			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	t.Run("renameat2", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(unix.SYS_RENAMEAT2, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
			if errno != 0 {
				if errno == syscall.ENOSYS {
					return ErrSkipTest{"renameat2 not supported"}
				}
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assertInode(t, event.Rename.New.Inode, getInode(t, testNewFile))
			test.validateRenameSchema(t, event)
			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)
			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	t.Run("io_uring", func(t *testing.T) {
		SkipIfNotAvailable(t)

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest, err := iouring.Renameat(unix.AT_FDCWD, testOldFile, unix.AT_FDCWD, testNewFile)
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
					return ErrSkipTest{"renameat not supported by io_uring"}
				}
				return err
			}

			if ret < 0 {
				return fmt.Errorf("failed to rename file with io_uring: %d", ret)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Rename.New.Inode, "wrong inode")
			assertFieldEqual(t, event, "rename.file.destination.inode", int(getInode(t, testNewFile)), "wrong inode")
			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)
			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

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

func TestRenameInvalidate(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.file.path in ["{{.Root}}/test-rename", "{{.Root}}/test2-rename"]`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testOldFile, _, err := test.Path("test-rename")
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

	testNewFile, _, err := test.Path("test2-rename")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i != 5; i++ {
		test.WaitSignal(t, func() error {
			return os.Rename(testOldFile, testNewFile)
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "rename.file.destination.path", testNewFile)
			test.validateRenameSchema(t, event)
		})

		// swap
		old := testOldFile
		testOldFile = testNewFile
		testNewFile = old
	}
}

func TestRenameReuseInode(t *testing.T) {
	SkipIfNotAvailable(t)

	// xfs has changed the inode reuse feature in 5.15
	// https://lkml.iu.edu/hypermail/linux/kernel/2108.3/07604.html
	checkKernelCompatibility(t, ">= 5.15 kernels or EL9", func(kv *kernel.Version) bool {
		return kv.Code >= kernel.Kernel5_15 || kv.IsRH9Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-rename-reuse-inode"`,
	}, {
		ID:         "test_rule2",
		Expression: `open.file.path == "{{.Root}}/test-rename-new"`,
	}}

	testDrive, err := newTestDrive(t, "xfs", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testOldFile, _, err := test.Path("test-rename-old")
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(testOldFile)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testOldFile)

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	testNewFile, _, err := test.Path("test-rename-new")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testNewFile)

	test.WaitSignal(t, func() error {
		f, err = os.Create(testNewFile)
		if err != nil {
			return err
		}

		return f.Close()
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
	})

	testNewFileInode := getInode(t, testNewFile)

	if err := os.Rename(testOldFile, testNewFile); err != nil {
		t.Fatal(err)
	}

	// At this point, the inode of test-rename-new was freed. We then
	// create a new file - with xfs, it will recycle the inode. This test
	// checks that we properly invalidated the cache entry of this inode.

	testReuseInodeFile, _, err := test.Path("test-rename-reuse-inode")
	if err != nil {
		t.Fatal(err)
	}

	defer os.Remove(testReuseInodeFile)

	test.WaitSignal(t, func() error {
		f, err = os.Create(testReuseInodeFile)
		if err != nil {
			return err
		}
		return f.Close()
	}, func(event *model.Event, rule *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
		assertFieldEqual(t, event, "open.file.inode", int(testNewFileInode))
		assertFieldEqual(t, event, "open.file.path", testReuseInodeFile)

		test.validateOpenSchema(t, event)
	})
}

func TestRenameFolder(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.name == "test-rename" && (open.flags & O_CREAT) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testOldFolder, _, err := test.Path(path.Dir("folder/folder-old/test-rename"))
	if err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(testOldFolder, os.ModePerm)

	testNewFolder, _, err := test.Path(path.Dir("folder/folder-new/test-rename"))
	if err != nil {
		t.Fatal(err)
	}

	filename := fmt.Sprintf("%s/test-rename", testOldFolder)
	defer os.Remove(filename)

	for i := 0; i != 5; i++ {
		test.WaitSignal(t, func() error {
			testFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return testFile.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "open.file.path", filename)
			test.validateOpenSchema(t, event)

			// swap
			if err := os.Rename(testOldFolder, testNewFolder); err != nil {
				t.Error(err)
			}

			old := testOldFolder
			testOldFolder = testNewFolder
			testNewFolder = old

			filename = fmt.Sprintf("%s/test-rename", testOldFolder)
		})
	}
}
