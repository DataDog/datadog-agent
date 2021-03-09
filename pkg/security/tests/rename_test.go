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
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestRename(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.file.path == "{{.Root}}/test-rename" && rename.file.uid == 98 && rename.file.gid == 99 && rename.file.destination.path == "{{.Root}}/test2-rename" && rename.file.destination.uid == 98 && rename.file.destination.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
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

	t.Run("rename", func(t *testing.T) {
		_, _, errno := syscall.Syscall(syscall.SYS_RENAME, uintptr(testOldFilePtr), uintptr(testNewFilePtr), 0)
		if errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rename" {
				t.Errorf("expected rename event, got %s", event.GetType())
			}

			if inode := getInode(t, testNewFile); inode != event.Rename.New.Inode {
				t.Logf("expected inode %d, got %d", event.Rename.New.Inode, inode)
			}

			testContainerPath(t, event, "rename.file.container_path")
			testContainerPath(t, event, "rename.file.destination.container_path")

			if int(event.Rename.Old.Mode)&expectedMode != expectedMode {
				t.Errorf("expected old mode %d, got %d", expectedMode, event.Rename.Old.Mode)
			}

			now := time.Now()
			if event.Rename.Old.MTime.After(now) || event.Rename.Old.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected old mtime close to %s, got %s", now, event.Rename.Old.MTime)
			}

			if event.Rename.Old.CTime.After(now) || event.Rename.Old.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected old ctime close to %s, got %s", now, event.Rename.Old.CTime)
			}

			if int(event.Rename.New.Mode)&expectedMode != expectedMode {
				t.Errorf("expected new mode %d, got %d", expectedMode, event.Rename.New.Mode)
			}

			if event.Rename.New.MTime.After(now) || event.Rename.New.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected new mtime close to %s, got %s", now, event.Rename.New.MTime)
			}

			if event.Rename.New.CTime.After(now) || event.Rename.New.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected new ctime close to %s, got %s", now, event.Rename.New.CTime)
			}
		}
	})

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	t.Run("renameat", func(t *testing.T) {
		_, _, errno := syscall.Syscall6(syscall.SYS_RENAMEAT, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
		if errno != 0 {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rename" {
				t.Errorf("expected rename event, got %s", event.GetType())
			}

			if inode := getInode(t, testNewFile); inode != event.Rename.New.Inode {
				t.Logf("expected inode %d, got %d", event.Rename.New.Inode, inode)
			}

			testContainerPath(t, event, "rename.file.container_path")
			testContainerPath(t, event, "rename.file.destination.container_path")

			if int(event.Rename.Old.Mode)&expectedMode != expectedMode {
				t.Errorf("expected old mode %d, got %d", expectedMode, event.Rename.Old.Mode)
			}

			now := time.Now()
			if event.Rename.Old.MTime.After(now) || event.Rename.Old.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected old mtime close to %s, got %s", now, event.Rename.Old.MTime)
			}

			if event.Rename.Old.CTime.After(now) || event.Rename.Old.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected old ctime close to %s, got %s", now, event.Rename.Old.CTime)
			}

			if int(event.Rename.New.Mode)&expectedMode != expectedMode {
				t.Errorf("expected new mode %d, got %d", expectedMode, event.Rename.New.Mode)
			}

			if event.Rename.New.MTime.After(now) || event.Rename.New.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected new mtime close to %s, got %s", now, event.Rename.New.MTime)
			}

			if event.Rename.New.CTime.After(now) || event.Rename.New.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected new ctime close to %s, got %s", now, event.Rename.New.CTime)
			}
		}
	})

	if err := os.Rename(testNewFile, testOldFile); err != nil {
		t.Fatal(err)
	}

	t.Run("renameat2", func(t *testing.T) {
		var renameat2syscall uintptr
		if runtime.GOARCH == "386" {
			renameat2syscall = 353
		} else {
			renameat2syscall = 316
		}
		_, _, errno := syscall.Syscall6(renameat2syscall, 0, uintptr(testOldFilePtr), 0, uintptr(testNewFilePtr), 0, 0)
		if errno != 0 {
			if errno == syscall.ENOSYS {
				t.Skip()
				return
			}
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rename" {
				t.Errorf("expected rename event, got %s", event.GetType())
			}

			if inode := getInode(t, testNewFile); inode != event.Rename.New.Inode {
				t.Logf("expected inode %d, got %d", event.Rename.New.Inode, inode)
			}

			testContainerPath(t, event, "rename.file.container_path")
			testContainerPath(t, event, "rename.file.destination.container_path")

			if int(event.Rename.Old.Mode)&expectedMode != expectedMode {
				t.Errorf("expected old mode %d, got %d", expectedMode, event.Rename.Old.Mode)
			}

			now := time.Now()
			if event.Rename.Old.MTime.After(now) || event.Rename.Old.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected old mtime close to %s, got %s", now, event.Rename.Old.MTime)
			}

			if event.Rename.Old.CTime.After(now) || event.Rename.Old.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected old ctime close to %s, got %s", now, event.Rename.Old.CTime)
			}

			if int(event.Rename.New.Mode)&expectedMode != expectedMode {
				t.Errorf("expected new mode %d, got %d", expectedMode, event.Rename.New.Mode)
			}

			if event.Rename.New.MTime.After(now) || event.Rename.New.MTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected new mtime close to %s, got %s", now, event.Rename.New.MTime)
			}

			if event.Rename.New.CTime.After(now) || event.Rename.New.CTime.Before(now.Add(-1*time.Hour)) {
				t.Errorf("expected new ctime close to %s, got %s", now, event.Rename.New.CTime)
			}
		}
	})
}

func TestRenameInvalidate(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.file.path in ["{{.Root}}/test-rename", "{{.Root}}/test2-rename"]`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
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
		if err := os.Rename(testOldFile, testNewFile); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rename" {
				t.Errorf("expected rename event, got %s", event.GetType())
			}
			if value, _ := event.GetFieldValue("rename.file.destination.path"); value.(string) != testNewFile {
				t.Errorf("expected filename not found")
			}
		}

		// swap
		old := testOldFile
		testOldFile = testNewFile
		testNewFile = old
	}
}

func TestRenameReuseInode(t *testing.T) {
	rules := []*rules.RuleDefinition{{
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

	test, err := newTestModule(nil, rules, testOpts{testDir: testDrive.Root()})
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

	f, err = os.Create(testNewFile)
	if err != nil {
		t.Fatal(err)
	}
	testNewFileInode := getInode(t, testNewFile)

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "open" {
			t.Errorf("expected open event, got %s", event.GetType())
		}
	}

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

	f, err = os.Create(testReuseInodeFile)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testReuseInodeFile)

	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	event, _, err = test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "open" {
			t.Errorf("expected open event, got %s", event.GetType())
		}

		if inode, _ := event.GetFieldValue("open.file.inode"); inode != int(testNewFileInode) {
			t.Errorf("expected inode not found")
		}

		if value, _ := event.GetFieldValue("open.file.path"); value.(string) != testReuseInodeFile {
			t.Errorf("expected filename not found %s != %s", value.(string), testReuseInodeFile)
		}
	}
}

func TestDentryRenameFolder(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.name == "test-rename" && (open.flags & O_CREAT) > 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
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
		testFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			t.Fatal(err)
		}
		testFile.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if value, _ := event.GetFieldValue("open.file.path"); value.(string) != filename {
				t.Errorf("#%d expected filename not found, `%s` != `%s`", i, value.(string), filename)
			}

			// swap
			if err := os.Rename(testOldFolder, testNewFolder); err != nil {
				t.Fatal(err)
			}

			old := testOldFolder
			testOldFolder = testNewFolder
			testNewFolder = old

			filename = fmt.Sprintf("%s/test-rename", testOldFolder)
		}
	}
}
