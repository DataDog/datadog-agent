// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

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

func TestOverlayFS(t *testing.T) {
	var sles12 bool

	kv, err := kernel.NewKernelVersion()
	if err == nil {
		sles12 = kv.IsSLES12Kernel()
	}

	if sles12 {
		t.Skip()
	}

	rules := []*rules.RuleDefinition{
		{
			ID:         "test_rule_open",
			Expression: `open.file.path in ["{{.Root}}/bind/read.txt", "{{.Root}}/bind/override.txt", "{{.Root}}/bind/create.txt", "{{.Root}}/bind/new.txt", "{{.Root}}/bind/truncate.txt", "{{.Root}}/bind/linked.txt"]`,
		},
		{
			ID:         "test_rule_unlink",
			Expression: `unlink.file.path in ["{{.Root}}/bind/read.txt", "{{.Root}}/bind/override.txt", "{{.Root}}/bind/renamed.txt", "{{.Root}}/bind/new.txt", "{{.Root}}/bind/chmod.txt", "{{.Root}}/bind/utimes.txt", "{{.Root}}/bind/chown.txt", "{{.Root}}/bind/xattr.txt", "{{.Root}}/bind/truncate.txt", "{{.Root}}/bind/link.txt", "{{.Root}}/bind/linked.txt"]`,
		},
		{
			ID:         "test_rule_rename",
			Expression: `rename.file.path == "{{.Root}}/bind/create.txt"`,
		},
		{
			ID:         "test_rule_rmdir",
			Expression: `rmdir.file.path == "{{.Root}}/bind/dir"`,
		},
		{
			ID:         "test_rule_chmod",
			Expression: `chmod.file.path == "{{.Root}}/bind/chmod.txt"`,
		},
		{
			ID:         "test_rule_mkdir",
			Expression: `mkdir.file.path == "{{.Root}}/bind/mkdir"`,
		},
		{
			ID:         "test_rule_utimes",
			Expression: `utimes.file.path == "{{.Root}}/bind/utimes.txt"`,
		},
		{
			ID:         "test_rule_chown",
			Expression: `chown.file.path == "{{.Root}}/bind/chown.txt"`,
		},
		{
			ID:         "test_rule_xattr",
			Expression: `setxattr.file.path == "{{.Root}}/bind/xattr.txt"`,
		},
		{
			ID:         "test_rule_link",
			Expression: `link.file.path == "{{.Root}}/bind/linked.txt"`,
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
	testDir, _, err := test.Path("lower", "dir")
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

	mountPoint, _, err := testDrive.Path("bind")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(mountPoint)

	if err := os.Mkdir(mountPoint, 0777); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Mount(testMerged, mountPoint, "bind", syscall.MS_BIND, ""); err != nil {
		t.Fatalf("could not create bind mount: %s", err)
	}
	defer syscall.Unmount(mountPoint, syscall.MNT_DETACH)

	// wait until the mount event is reported until the event ordered bug is fixed
	time.Sleep(2 * time.Second)

	defer func() {
		exec.Command("umount", testMerged).CombinedOutput()
	}()

	// open a file in lower in RDONLY and check that open/unlink inode are valid from userspace
	// perspective and equals
	t.Run("read-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/read.txt")
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
			assert.Equal(t, event.Open.File.Inode, inode, "wrong open inode")
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}
	})

	t.Run("override-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/override.txt")
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
			assert.Equal(t, inode, event.Open.File.Inode, "wrong open inode")
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("create-upper", func(t *testing.T) {
		testFile, _, err := test.Path("bind/new.txt")
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
			assert.Equal(t, event.Open.File.Inode, inode, "wrong open inode")
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("rename-lower", func(t *testing.T) {
		oldFile, _, err := test.Path("bind/create.txt")
		if err != nil {
			t.Fatal(err)
		}

		newFile, _, err := test.Path("bind/renamed.txt")
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
			if value, _ := event.GetFieldValue("rename.file.path"); value.(string) != oldFile {
				t.Errorf("expected filename not found %s != %s", value.(string), oldFile)
			}

			inode = getInode(t, newFile)
			assert.Equal(t, event.Rename.New.Inode, inode, "wrong rename inode")
			inUpperLayer, _ := event.GetFieldValue("rename.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")

			inUpperLayer, _ = event.GetFieldValue("rename.file.destination.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}

		if err := os.Remove(newFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("rmdir-lower", func(t *testing.T) {
		testDir, _, err := test.Path("bind/dir")
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
			assert.Equal(t, event.Rmdir.File.Inode, inode, "wrong rmdir inode")
			inUpperLayer, _ := event.GetFieldValue("rmdir.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}
	})

	t.Run("chmod-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/chmod.txt")
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
			assert.Equal(t, event.Chmod.File.Inode, inode, "wrong chmod inode")
			inUpperLayer, _ := event.GetFieldValue("chmod.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("mkdir-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/mkdir")
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
			assert.Equal(t, event.Mkdir.File.Inode, inode, "wrong mkdir inode")
			inUpperLayer, _ := event.GetFieldValue("mkdir.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("utimes-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/utimes.txt")
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
			assert.Equal(t, event.Utimes.File.Inode, inode, "wrong utimes inode")
			inUpperLayer, _ := event.GetFieldValue("utimes.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("chown-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/chown.txt")
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
			assert.Equal(t, event.Chown.File.Inode, inode, "wrong chown inode")
			inUpperLayer, _ := event.GetFieldValue("chown.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("xattr-lower", func(t *testing.T) {
		testFile, testFilePtr, err := test.Path("bind/xattr.txt")
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
			assert.Equal(t, event.SetXAttr.File.Inode, inode, "wrong setxattr inode")
			inUpperLayer, _ := event.GetFieldValue("setxattr.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("truncate-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/truncate.txt")
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
			assert.Equal(t, event.Open.File.Inode, inode, "wrong open inode")
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")
		}

		if err := os.Remove(testFile); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("link-lower", func(t *testing.T) {
		testSrc, _, err := test.Path("bind/linked.txt")
		if err != nil {
			t.Fatal(err)
		}

		testTarget, _, err := test.Path("bind/link.txt")
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
			assert.Equal(t, event.Link.Source.Inode, inode, "wrong link source inode")

			inUpperLayer, _ := event.GetFieldValue("link.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, false, "should be in base layer")

			inUpperLayer, _ = event.GetFieldValue("link.file.destination.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}

		if err := os.Remove(testSrc); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in base layer")
		}

		if err := os.Remove(testTarget); err != nil {
			t.Fatal(err)
		}

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, event.Unlink.File.Inode, inode, "wrong unlink inode")
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")
			assert.Equal(t, inUpperLayer, true, "should be in upper layer")
		}
	})

	t.Run("invalidate-discarder", func(t *testing.T) {
		// ensure that all the previous discarder are removed
		test.probe.FlushDiscarders()

		testFile, _, err := test.Path("bind/discarded.txt")
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
		testInvalidator, _, err := test.Path("bind/invalidator.txt")
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
