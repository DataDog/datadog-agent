// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/cobaugh/osrelease"
	"gotest.tools/assert"
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
			t.Errorf("expected filename not found %s != %s", value.(string), testReuseInodeFile)
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
	sles12 := false
	osrelease, err := osrelease.Read()
	if err == nil {
		sles12 = osrelease["NAME"] == "SLES" && strings.HasPrefix(osrelease["VERSION_ID"], "12")
	}

	if testEnvironment == DockerEnvironment || sles12 {
		t.Skip()
	}

	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule_open",
			Expression: `open.filename in ["{{.Root}}/merged/read.txt", "{{.Root}}/merged/override.txt", "{{.Root}}/merged/create.txt", "{{.Root}}/merged/new.txt", "{{.Root}}/merged/truncate.txt", "{{.Root}}/merged/linked.txt"]`,
		},
		{
			ID:         "test_rule_unlink",
			Expression: `unlink.filename in ["{{.Root}}/merged/read.txt", "{{.Root}}/merged/override.txt", "{{.Root}}/merged/renamed.txt", "{{.Root}}/merged/new.txt", "{{.Root}}/merged/chmod.txt", "{{.Root}}/merged/utimes.txt", "{{.Root}}/merged/chown.txt", "{{.Root}}/merged/xattr.txt", "{{.Root}}/merged/truncate.txt", "{{.Root}}/merged/link.txt", "{{.Root}}/merged/linked.txt"]`,
		},
		{
			ID:         "test_rule_rename",
			Expression: `rename.old.filename == "{{.Root}}/merged/create.txt"`,
		},
		{
			ID:         "test_rule_rmdir",
			Expression: `rmdir.filename == "{{.Root}}/merged/dir"`,
		},
		{
			ID:         "test_rule_chmod",
			Expression: `chmod.filename == "{{.Root}}/merged/chmod.txt"`,
		},
		{
			ID:         "test_rule_mkdir",
			Expression: `mkdir.filename == "{{.Root}}/merged/mkdir"`,
		},
		{
			ID:         "test_rule_utimes",
			Expression: `utimes.filename == "{{.Root}}/merged/utimes.txt"`,
		},
		{
			ID:         "test_rule_chown",
			Expression: `chown.filename == "{{.Root}}/merged/chown.txt"`,
		},
		{
			ID:         "test_rule_xattr",
			Expression: `setxattr.filename == "{{.Root}}/merged/xattr.txt"`,
		},
		{
			ID:         "test_rule_link",
			Expression: `link.source.filename == "{{.Root}}/merged/linked.txt"`,
		},
	}

	testDrive, err := newTestDrive("xfs", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(nil, rules, testOpts{testDir: testDrive.Root(), wantProbeEvents: true, disableApprovers: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	// create layers
	testLower, testUpper, testWordir, testMerged := createOverlayLayers(t, test)

	// create all the lower files
	for _, filename := range []string{
		"lower/read.txt", "lower/override.txt", "lower/create.txt", "lower/chmod.txt",
		"lower/utimes.txt", "lower/chown.txt", "lower/xattr.txt", "lower/truncate.txt", "lower/linked.txt",
		"lower/discarded.txt", "lower/invalidator.txt"} {
		_, _, err = test.Create(filename)
		if err != nil {
			t.Fatal(err)
		}
	}

	// create dir in lower
	testDir, _, err := test.Path("lower/dir")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(testDir, 0777); err != nil {
		t.Fatal(err)
	}

	args := []string{
		"mount", "-t", "overlay", "overlay", "-o", "lowerdir=" + testLower + ",upperdir=" + testUpper + ",workdir=" + testWordir, testMerged,
	}

	_, err = exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	// wait until the mount event is reported until the event ordered bug is fixed
	time.Sleep(2 * time.Second)

	defer func() {
		exec.Command("umount", testMerged).CombinedOutput()
	}()

	// open a file in lower in RDONLY and check that open/unlink inode are valid from userspace
	// perspective and equals
	t.Run("read-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/read.txt")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.OpenFile(testFile, os.O_RDONLY, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.Open.Inode, inode, "wrong open inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("override-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/override.txt")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.OpenFile(testFile, os.O_RDWR, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, inode, event.Open.Inode, "wrong open inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("create-upper", func(t *testing.T) {
		testFile, _, err := test.Path("merged/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.OpenFile(testFile, os.O_CREATE, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.Open.Inode, inode, "wrong open inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("rename-lower", func(t *testing.T) {
		oldFile, _, err := test.Path("merged/create.txt")
		if err != nil {
			t.Fatal(err)
		}

		newFile, _, err := test.Path("merged/renamed.txt")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Rename(oldFile, newFile); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if value, _ := event.GetFieldValue("rename.old.filename"); value.(string) != oldFile {
				t.Errorf("expected filename not found %s != %s", value.(string), oldFile)
			}

			inode = getInode(t, newFile)
			assert.Equal(t, event.Rename.New.Inode, inode, "wrong rename inode")
		}

		if err := os.Remove(newFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("rmdir-lower", func(t *testing.T) {
		testDir, _, err := test.Path("merged/dir")
		if err != nil {
			t.Fatal(err)
		}

		inode := getInode(t, testDir)

		if err := os.Remove(testDir); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Rmdir.Inode, inode, "wrong rmdir inode")
		}
	})

	t.Run("chmod-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/chmod.txt")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chmod(testFile, 0777); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.Chmod.Inode, inode, "wrong chmod inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("mkdir-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/mkdir")
		if err != nil {
			t.Fatal(err)
		}

		if err := syscall.Mkdir(testFile, 0777); err != nil {
			t.Fatal(err)
		}

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode := getInode(t, testFile)
			assert.Equal(t, event.Mkdir.Inode, inode, "wrong mkdir inode")
		}
	})

	t.Run("utimes-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/utimes.txt")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chtimes(testFile, time.Now(), time.Now()); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.Utimes.Inode, inode, "wrong utimes inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("chown-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/chown.txt")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Chown(testFile, os.Getuid(), os.Getgid()); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.Chown.Inode, inode, "wrong chown inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("xattr-lower", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("merged/xattr.txt")
		if err != nil {
			t.Fatal(err)
		}

		xattrName, err := syscall.BytePtrFromString("user.test_xattr")
		if err != nil {
			t.Fatal(err)
		}
		xattrNamePtr := unsafe.Pointer(xattrName)
		xattrValuePtr := unsafe.Pointer(&[]byte{})

		_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.SetXAttr.Inode, inode, "wrong setxattr inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("truncate-lower", func(t *testing.T) {
		testFile, _, err := test.Path("merged/truncate.txt")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Truncate(testFile, 0); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testFile)
			assert.Equal(t, event.Open.Inode, inode, "wrong open inode")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("link-lower", func(t *testing.T) {
		testSrc, _, err := test.Path("merged/linked.txt")
		if err != nil {
			t.Fatal(err)
		}

		testTarget, _, err := test.Path("merged/link.txt")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.Link(testSrc, testTarget); err != nil {
			t.Fatal(err)
		}

		var inode uint64

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			inode = getInode(t, testSrc)
			assert.Equal(t, event.Link.Source.Inode, inode, "wrong link inode")
		}

		if err := os.Remove(testSrc); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}

		if err := os.Remove(testTarget); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.Inode, inode, "wrong unlink inode")
		}
	})

	t.Run("invalidate-discarder", func(t *testing.T) {
		testFile, _, err := test.Path("merged/discarded.txt")
		if err != nil {
			t.Fatal(err)
		}

		// shouldn't be discarded here
		f, err := os.OpenFile(testFile, os.O_RDONLY, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}

		event, err := waitForOpenDiscarder(test, testFile)
		if err != nil {
			t.Fatalf("should get a discarder: %+v", err)
		}

		// should be now discarderd
		f, err = os.OpenFile(testFile, os.O_RDONLY, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}

		if event, err = waitForOpenProbeEvent(test, testFile); err == nil {
			t.Fatalf("shouldn't get an event: %+v", event)
		}

		// remove another file which should generate a global discarder invalidation
		testInvalidator, _, err := test.Path("merged/invalidator.txt")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(testInvalidator); err != nil {
			t.Fatal(err)
		}

		// we should be able to get an event again
		f, err = os.OpenFile(testFile, os.O_RDONLY, 0755)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}

		if _, err := waitForOpenProbeEvent(test, testFile); err != nil {
			t.Fatalf("should get an event: %+v", err)
		}
	})
}
