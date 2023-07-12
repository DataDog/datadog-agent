// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

package tests

import (
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func createOverlayLayer(t *testing.T, test *testModule, name string) string {
	p, _, err := test.Path(name)
	if err != nil {
		t.Error(err)
		return ""
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
	checkKernelCompatibility(t, "Suse 12 kernels", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	ruleDefs := []*rules.RuleDefinition{
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
			Expression: `rmdir.file.path in ["{{.Root}}/bind/dir", "{{.Root}}/bind/mkdir"]`,
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
		{
			ID:         "test_rule_parent",
			Expression: `open.file.path == "{{.Root}}/bind/parent/child"`,
		},
		{
			ID:         "test_rule_renamed_parent",
			Expression: `open.file.path == "{{.Root}}/bind/renamed/child"`,
		},
	}

	testDrive, err := newTestDrive(t, "xfs", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer testDrive.Close()

	test, err := newTestModule(t, nil, ruleDefs, testOpts{testDir: testDrive.Root(), disableApprovers: true})
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

	mountPoint := testDrive.Path("bind")
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
		if _, err := exec.Command("umount", testMerged).CombinedOutput(); err != nil {
			t.Logf("failed to unmount %s: %v", testMerged, err)
		}
	}()

	// open a file in lower in RDONLY and check that open/unlink inode are valid from userspace
	// perspective and equals
	t.Run("read-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/read.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDONLY, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Open.File.Inode, "wrong open inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, false, inUpperLayer, "should be in base layer")
		})
	})

	t.Run("override-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/override.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")

			success := assert.Equal(t, event.Open.File.Inode, inode, "wrong open inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})

	t.Run("create-upper", func(t *testing.T) {
		testFile, _, err := test.Path("bind/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Open.File.Inode, "wrong open inode")
			success = assert.Equal(t, true, inUpperLayer, "should be in upper layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
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

		var inode uint64

		test.WaitSignal(t, func() error {
			return os.Rename(oldFile, newFile)
		}, func(event *model.Event, rule *rules.Rule) {
			success := true

			if value, _ := event.GetFieldValue("rename.file.path"); value.(string) != oldFile {
				t.Errorf("expected filename not found %s != %s", value.(string), oldFile)
				success = false
			}

			inode = getInode(t, newFile)
			inUpperLayer, _ := event.GetFieldValue("rename.file.in_upper_layer")

			success = assert.Equal(t, inode, event.Rename.New.Inode, "wrong rename inode") && success
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			inUpperLayer, _ = event.GetFieldValue("rename.file.destination.in_upper_layer")

			success = assert.Equal(t, true, inUpperLayer, "should be in upper layer") && success

			if !success {
				_ = os.Remove(newFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(newFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})

	t.Run("rename-parent", func(t *testing.T) {
		testFile, _, err := test.Path("bind/parent/child")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)

		dir := path.Dir(testFile)
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("failed to create directory: %s", err)
		}

		test.WaitSignal(t, func() error {
			f, err := os.Create(testFile)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_parent")
		})

		test.WaitSignal(t, func() error {
			newFile, _, err := test.Path("bind/renamed/child")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(newFile)

			if err = os.Rename(dir, path.Dir(newFile)); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(newFile, os.O_RDWR, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_renamed_parent")
		})
	})

	t.Run("rmdir-lower", func(t *testing.T) {
		testDir, _, err := test.Path("bind/dir")
		if err != nil {
			t.Fatal(err)
		}

		inode := getInode(t, testDir)

		test.WaitSignal(t, func() error {
			return os.Remove(testDir)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("rmdir.file.in_upper_layer")

			assert.Equal(t, inode, event.Rmdir.File.Inode, "wrong rmdir inode")
			assert.Equal(t, false, inUpperLayer, "should be in base layer")
		})
	})

	t.Run("chmod-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/chmod.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			return os.Chmod(testFile, 0777)
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("chmod.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Chmod.File.Inode, "wrong chmod inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})

	t.Run("mkdir-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/mkdir")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			return syscall.Mkdir(testFile, 0777)
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("mkdir.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Mkdir.File.Inode, "wrong mkdir inode")
			success = assert.Equal(t, true, inUpperLayer, "should be in upper layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("rmdir.file.in_upper_layer")

			assert.Equal(t, inode, event.Rmdir.File.Inode, "wrong rmdir inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})

	t.Run("utimes-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/utimes.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			return os.Chtimes(testFile, time.Now(), time.Now())
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("utimes.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Utimes.File.Inode, "wrong utimes inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})

	t.Run("chown-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/chown.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			return os.Chown(testFile, os.Getuid(), os.Getgid())
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("chown.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Chown.File.Inode, "wrong chown inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
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

		var inode uint64

		test.WaitSignal(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("setxattr.file.in_upper_layer")

			success := assert.Equal(t, inode, event.SetXAttr.File.Inode, "wrong setxattr inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})

	t.Run("truncate-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/truncate.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignal(t, func() error {
			return os.Truncate(testFile, 0)
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testFile)
			inUpperLayer, _ := event.GetFieldValue("open.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Open.File.Inode, "wrong open inode")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testFile)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
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

		var inode uint64

		test.WaitSignal(t, func() error {
			return os.Link(testSrc, testTarget)
		}, func(event *model.Event, rule *rules.Rule) {
			inode = getInode(t, testSrc)
			success := assert.Equal(t, inode, event.Link.Source.Inode, "wrong link source inode")

			inUpperLayer, _ := event.GetFieldValue("link.file.in_upper_layer")
			success = assert.Equal(t, false, inUpperLayer, "should be in base layer") && success

			inUpperLayer, _ = event.GetFieldValue("link.file.destination.in_upper_layer")
			success = assert.Equal(t, true, inUpperLayer, "should be in upper layer") && success

			if !success {
				_ = os.Remove(testSrc)
				_ = os.Remove(testTarget)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testSrc)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			success := assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			success = assert.Equal(t, true, inUpperLayer, "should be in base layer") && success

			if !success {
				_ = os.Remove(testTarget)
			}
		})

		test.WaitSignal(t, func() error {
			return os.Remove(testTarget)
		}, func(event *model.Event, rule *rules.Rule) {
			inUpperLayer, _ := event.GetFieldValue("unlink.file.in_upper_layer")

			assert.Equal(t, inode, event.Unlink.File.Inode, "wrong unlink inode")
			assert.Equal(t, true, inUpperLayer, "should be in upper layer")
		})
	})
}
