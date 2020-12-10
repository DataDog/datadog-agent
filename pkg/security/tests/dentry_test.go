// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestDentryRename(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rename.old.filename in ["{{.Root}}/test-rename", "{{.Root}}/test2-rename"]`,
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
			if value, _ := event.GetFieldValue("rename.new.filename"); value.(string) != testNewFile {
				t.Errorf("expected filename not found")
			}
		}

		// swap
		old := testOldFile
		testOldFile = testNewFile
		testNewFile = old
	}
}

func TestDentryRenameReuseInode(t *testing.T) {
	rules := []*rules.RuleDefinition{{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/test-rename-reuse-inode"`,
	}, {
		ID:         "test_rule2",
		Expression: `open.filename == "{{.Root}}/test-rename-new"`,
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
		if value, _ := event.GetFieldValue("open.filename"); value.(string) != testReuseInodeFile {
			t.Errorf("expected filename not found")
		}
	}
}

func TestDentryRenameFolder(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.basename == "test-rename" && (open.flags & O_CREAT) > 0`,
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

			if value, _ := event.GetFieldValue("open.filename"); value.(string) != filename {
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

func TestDentryUnlink(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `unlink.filename =~ "{{.Root}}/test-unlink-*"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for i := 0; i != 5; i++ {
		filename := fmt.Sprintf("test-unlink-%d", i)

		testFile, _, err := test.Path(filename)
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Create(testFile)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		os.Remove(testFile)

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "unlink" {
				t.Errorf("expected unlink event, got %s", event.GetType())
			}

			if value, _ := event.GetFieldValue("unlink.filename"); value.(string) != testFile {
				t.Errorf("expected filename not found")
			}
		}
	}
}

func TestDentryRmdir(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `rmdir.filename =~ "{{.Root}}/test-rmdir-*"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	for i := 0; i != 5; i++ {
		testFile, _, err := test.Path(fmt.Sprintf("test-rmdir-%d", i))
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testFile, 0777); err != nil {
			t.Fatal(err)
		}

		if err := syscall.Rmdir(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "rmdir" {
				t.Errorf("expected rmdir event, got %s", event.GetType())
			}

			if value, _ := event.GetFieldValue("rmdir.filename"); value.(string) != testFile {
				t.Errorf("expected filename not found")
			}
		}
	}
}
func createOverlayLayer(t *testing.T, test *testModule, name string) string {
	p, _, err := test.Path(name)
	if err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(p, os.ModePerm)

	return p
}

func createOverlayLayers(t *testing.T, test *testModule) (string, string, string, string) {
	return createOverlayLayer(t, test, "lower"),
		createOverlayLayer(t, test, "upper"),
		createOverlayLayer(t, test, "workdir"),
		createOverlayLayer(t, test, "merged")
}

func TestDentryOverlay(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip()
	}

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/merged/kiki.txt"`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testLower, testUpper, testWordir, testMerged := createOverlayLayers(t, test)

	testFile, _, err := test.Path("lower/kiki.txt")
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

	args := []string{
		"mount", "-t", "overlay", "overlay", "-o", "lowerdir=" + testLower + ",upperdir=" + testUpper + ",workdir=" + testWordir, testMerged,
	}

	_, err = exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		exec.Command("umount", testMerged).CombinedOutput()
	}()

	testFile, _, err = test.Path("merged/kiki.txt")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("read-only", func(t *testing.T) {
		f, err = os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if value, _ := event.GetFieldValue("open.filename"); value.(string) != testFile {
				t.Errorf("expected filename not found")
			}
		}
	})

	t.Run("read-write", func(t *testing.T) {
		f, err = os.OpenFile(testFile, os.O_RDWR, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if value, _ := event.GetFieldValue("open.filename"); value.(string) != testFile {
				t.Errorf("expected filename not found")
			}
		}
	})
}
