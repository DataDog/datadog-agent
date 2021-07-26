// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"path"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestRename(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.file.path == "{{.Root}}/test-rename" && rename.file.uid == 98 && rename.file.gid == 99 && rename.file.destination.path == "{{.Root}}/test2-rename" && rename.file.destination.uid == 98 && rename.file.destination.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
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

	t.Run("rename", ifSyscallSupported("SYS_RENAME", func(t *testing.T, syscallNB uintptr) {
		err = test.GetSignal(t, func() error {
			_, _, errno := syscall.Syscall(syscallNB, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
			if errno != 0 {
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Rename.New.Inode, "wrong inode")
			assertFieldEqual(t, event, "rename.file.destination.inode", int(getInode(t, testNewFile)), "wrong inode")

			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)

			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

			if !validateRenameSchema(t, event) {
				t.Error(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}

		if err := os.Rename(testNewFile, testOldFile); err != nil {
			t.Fatal(err)
		}
	}))

	t.Run("renameat", func(t *testing.T) {
		err = test.GetSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_RENAMEAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
			if errno != 0 {
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Rename.New.Inode, "wrong inode")
			assertFieldEqual(t, event, "rename.file.destination.inode", int(getInode(t, testNewFile)), "wrong inode")

			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)

			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

			if !validateRenameSchema(t, event) {
				t.Error(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}
	})

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	t.Run("renameat2", func(t *testing.T) {
		err = test.GetSignal(t, func() error {
			_, _, errno := syscall.Syscall6(unix.SYS_RENAMEAT2, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
			if errno != 0 {
				if errno == syscall.ENOSYS {
					t.Skip("renameat2 not supported")
				}
				t.Fatal(errno)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assert.Equal(t, getInode(t, testNewFile), event.Rename.New.Inode, "wrong inode")
			assertFieldEqual(t, event, "rename.file.destination.inode", int(getInode(t, testNewFile)), "wrong inode")

			assertRights(t, event.Rename.Old.Mode, expectedMode)
			assertNearTime(t, event.Rename.Old.MTime)
			assertNearTime(t, event.Rename.Old.CTime)

			assertRights(t, event.Rename.New.Mode, expectedMode)
			assertNearTime(t, event.Rename.New.MTime)
			assertNearTime(t, event.Rename.New.CTime)

			if !validateRenameSchema(t, event) {
				t.Error(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}
	})
}

func TestRenameInvalidate(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.file.path in ["{{.Root}}/test-rename", "{{.Root}}/test2-rename"]`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
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
		err = test.GetSignal(t, func() error {
			if err := os.Rename(testOldFile, testNewFile); err != nil {
				t.Fatal(err)
			}
			return nil
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "rename", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "rename.file.destination.path", testNewFile)

			if !validateRenameSchema(t, event) {
				t.Error(event.String())
			}
		})
		if err != nil {
			t.Error(err)
		}

		// swap
		old := testOldFile
		testOldFile = testNewFile
		testNewFile = old
	}
}

func TestRenameReuseInode(t *testing.T) {
	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-rename-reuse-inode"`,
	}, {
		ID:         "test_rule2",
		Expression: `open.file.path == "{{.Root}}/test-rename-new"`,
	}}

	testDrive, err := newTestDrive("xfs", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, testOpts{testDir: testDrive.Root()})
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

	err = test.GetSignal(t, func() error {
		f, err = os.Create(testNewFile)
		if err != nil {
			t.Fatal(err)
		}
		return nil
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
	})
	if err != nil {
		t.Error(err)
	}

	testNewFileInode := getInode(t, testNewFile)

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

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

	err = test.GetSignal(t, func() error {
		f, err = os.Create(testReuseInodeFile)
		if err != nil {
			t.Fatal(err)
		}
		return f.Close()
	}, func(event *sprobe.Event, rule *rules.Rule) {
		assert.Equal(t, "open", event.GetType(), "wrong event type")
		assertFieldEqual(t, event, "open.file.inode", int(testNewFileInode))
		assertFieldEqual(t, event, "open.file.path", testReuseInodeFile)

		if !validateRenameSchema(t, event) {
			t.Error(event.String())
		}
	})
	if err != nil {
		t.Error(err)
	}
}

func TestRenameFolder(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.name == "test-rename" && (open.flags & O_CREAT) > 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
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

	var filename atomic.Value
	filename.Store(fmt.Sprintf("%s/test-rename", testOldFolder))
	defer os.Remove(filename.Load().(string))

	for i := 0; i != 5; i++ {
		err = test.GetSignal(t, func() error {
			testFile, err := os.OpenFile(filename.Load().(string), os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				t.Fatal(err)
			}
			return testFile.Close()
		}, func(event *sprobe.Event, rule *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assertFieldEqual(t, event, "open.file.path", filename.Load().(string))

			if !validateRenameSchema(t, event) {
				t.Error(event.String())
			}

			// swap
			if err := os.Rename(testOldFolder, testNewFolder); err != nil {
				t.Error(err)
			}

			old := testOldFolder
			testOldFolder = testNewFolder
			testNewFolder = old

			filename.Store(fmt.Sprintf("%s/test-rename", testOldFolder))
		})
		if err != nil {
			t.Error(err)
		}
	}
}
