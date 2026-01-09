// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
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
	SkipIfNotAvailable(t)

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
			Expression: `unlink.file.path == "{{.Root}}/bind/unlink.txt"`,
		},
		{
			ID:         "test_rule_rename",
			Expression: `rename.file.path in ["{{.Root}}/bind/create.txt", "{{.Root}}/bind/new.txt"]`,
		},
		{
			ID:         "test_rule_rmdir",
			Expression: `rmdir.file.path in ["{{.Root}}/bind/dir", "{{.Root}}/bind/mkdir"]`,
		},
		{
			ID:         "test_rule_chmod",
			Expression: `chmod.file.path in ["{{.Root}}/bind/chmod.txt", "{{.Root}}/bind/new.txt"]`,
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
			Expression: `chown.file.path in ["{{.Root}}/bind/chown.txt", "{{.Root}}/bind/new.txt"]`,
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

	test, err := newTestModule(t, nil, ruleDefs, withDynamicOpts(dynamicTestOpts{testDir: testDrive.Root()}))
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("not supported")
	}

	// create layers
	testLower, testUpper, testWordir, testMerged := createOverlayLayers(t, test)

	// create all the lower files
	for _, filename := range []string{
		"lower/read.txt", "lower/override.txt", "lower/create.txt", "lower/chmod.txt",
		"lower/utimes.txt", "lower/chown.txt", "lower/xattr.txt", "lower/truncate.txt", "lower/linked.txt",
		"lower/discarded.txt", "lower/invalidator.txt", "lower/unlink.txt"} {
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

	validateInodeAndLayerFallback := func(t *testing.T, filename string, expectedInode uint64, expectedUpperLayer bool) {
		fileFields, err := p.Resolvers.ProcessResolver.RetrieveFileFieldsFromProcfs(filename)
		assert.NoError(t, err, "shouldn't return an error")
		if expectedInode != 0 {
			assert.Equal(t, expectedInode, fileFields.Inode, "wrong inode using fallback")
		}
		assert.Equal(t, expectedUpperLayer, fileFields.IsInUpperLayer(), "wrong layer using fallback for inode %d", expectedInode)
	}

	validateInodeAndLayerRuntime := func(t *testing.T, expectedInode uint64, expectedUpperLayer bool, fileFields *model.FileFields) {
		if expectedInode != 0 {
			assert.Equal(t, expectedInode, fileFields.Inode, "wrong inode in runtime event")
		}
		assert.Equal(t, expectedUpperLayer, fileFields.IsInUpperLayer(), "wrong layer in runtime event for inode %d", expectedInode)
	}

	validateInodeAndLayer := func(t *testing.T, filename string, expectedInode uint64, expectedUpperLayer bool, fileFields *model.FileFields) {
		validateInodeAndLayerRuntime(t, expectedInode, expectedUpperLayer, fileFields)
		validateInodeAndLayerFallback(t, filename, expectedInode, expectedUpperLayer)
	}

	// open a file in lower in RDONLY and check that open/unlink inode are valid from userspace
	// perspective and equals
	t.Run("read-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/read.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDONLY, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, false, &event.Open.File.FileFields)
		}, "test_rule_open")
	})

	t.Run("override-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/override.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Open.File.FileFields)
		}, "test_rule_open")
	})

	t.Run("create-upper", func(t *testing.T) {
		testFile, _, err := test.Path("bind/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			f, err := os.OpenFile(testFile, os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Open.File.FileFields)
		}, "test_rule_open")
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

		test.WaitSignalFromRule(t, func() error {
			return os.Rename(oldFile, newFile)
		}, func(event *model.Event, _ *rules.Rule) {
			if value, _ := event.GetFieldValue("rename.file.path"); value.(string) != oldFile {
				t.Errorf("expected filename not found %s != %s", value.(string), oldFile)
			}

			inode = getInode(t, newFile)

			assert.Equal(t, inode, event.Rename.New.Inode, "wrong rename inode")
			assert.Equal(t, false, event.Rename.Old.IsInUpperLayer(), "should be in base layer")
			assert.Equal(t, true, event.Rename.New.IsInUpperLayer(), "should be in upper layer")

			validateInodeAndLayerFallback(t, newFile, inode, true)
		}, "test_rule_rename")
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

		test.WaitSignalFromRule(t, func() error {
			f, err := os.Create(testFile)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_parent")
		}, "test_rule_parent")

		test.WaitSignalFromRule(t, func() error {
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
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_renamed_parent")
		}, "test_rule_renamed_parent")
	})

	t.Run("rmdir-lower", func(t *testing.T) {
		testDir, _, err := test.Path("bind/dir")
		if err != nil {
			t.Fatal(err)
		}

		inode := getInode(t, testDir)

		test.WaitSignalFromRule(t, func() error {
			return os.Remove(testDir)
		}, func(event *model.Event, _ *rules.Rule) {
			assert.Equal(t, inode, event.Rmdir.File.Inode, "wrong rmdir inode")
			assert.Equal(t, false, event.Rmdir.File.IsInUpperLayer(), "should be in base layer")
		}, "test_rule_rmdir")
	})

	t.Run("chmod-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/chmod.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Chmod(testFile, 0777)
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Chmod.File.FileFields)
		}, "test_rule_chmod")
	})

	t.Run("chmod-upper", func(t *testing.T) {
		checkKernelCompatibility(t, "Oracle kernels", func(kv *kernel.Version) bool {
			// skip Oracle for now
			return kv.IsOracleUEKKernel()
		})

		testFile, _, err := test.Path("bind/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Chmod(testFile, 0777)
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Chmod.File.FileFields)
		}, "test_rule_chmod")
	})

	t.Run("mkdir-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/mkdir")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return syscall.Mkdir(testFile, 0777)
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Mkdir.File.FileFields)
		}, "test_rule_mkdir")
	})

	t.Run("utimes-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/utimes.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Chtimes(testFile, time.Now(), time.Now())
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Utimes.File.FileFields)
		}, "test_rule_utimes")
	})

	t.Run("chown-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/chown.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Chown(testFile, os.Getuid(), os.Getgid())
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Chown.File.FileFields)
		}, "test_rule_chown")
	})

	t.Run("chown-upper", func(t *testing.T) {
		checkKernelCompatibility(t, "Oracle kernels", func(kv *kernel.Version) bool {
			// skip Oracle for now
			return kv.IsOracleUEKKernel()
		})

		testFile, _, err := test.Path("bind/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Chown(testFile, os.Getuid(), os.Getgid())
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Chown.File.FileFields)
		}, "test_rule_chown")
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

		test.WaitSignalFromRule(t, func() error {
			_, _, errno := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(testFilePtr), uintptr(xattrNamePtr), uintptr(xattrValuePtr), 0, unix.XATTR_CREATE, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.SetXAttr.File.FileFields)
		}, "test_rule_xattr")
	})

	t.Run("truncate-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/truncate.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Truncate(testFile, 0)
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Open.File.FileFields)
		}, "test_rule_open")
	})

	t.Run("truncate-upper", func(t *testing.T) {
		checkKernelCompatibility(t, "Oracle kernels", func(kv *kernel.Version) bool {
			// skip Oracle for now
			return kv.IsOracleUEKKernel()
		})

		testFile, _, err := test.Path("bind/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Truncate(testFile, 0)
		}, func(event *model.Event, _ *rules.Rule) {
			inode = getInode(t, testFile)

			validateInodeAndLayer(t, testFile, inode, true, &event.Open.File.FileFields)
		}, "test_rule_open")
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

		test.WaitSignalFromRule(t, func() error {
			return os.Link(testSrc, testTarget)
		}, func(event *model.Event, _ *rules.Rule) {
			// fake inode
			validateInodeAndLayer(t, testTarget, 0, true, &event.Link.Target.FileFields)
		}, "test_rule_link")
	})

	t.Run("unlink-lower", func(t *testing.T) {
		testFile, _, err := test.Path("bind/unlink.txt")
		if err != nil {
			t.Fatal(err)
		}

		inode := getInode(t, testFile)

		test.WaitSignalFromRule(t, func() error {
			return os.Remove(testFile)
		}, func(event *model.Event, _ *rules.Rule) {
			// impossible to test with the fallback, the file is deleted
			validateInodeAndLayerRuntime(t, inode, false, &event.Unlink.File.FileFields)
		}, "test_rule_unlink")
	})

	t.Run("rename-upper", func(t *testing.T) {
		checkKernelCompatibility(t, "Oracle kernels", func(kv *kernel.Version) bool {
			// skip Oracle for now
			return kv.IsOracleUEKKernel()
		})

		oldFile, _, err := test.Path("bind/new.txt")
		if err != nil {
			t.Fatal(err)
		}

		newFile, _, err := test.Path("bind/new-renamed.txt")
		if err != nil {
			t.Fatal(err)
		}

		var inode uint64

		test.WaitSignalFromRule(t, func() error {
			return os.Rename(oldFile, newFile)
		}, func(event *model.Event, _ *rules.Rule) {
			if value, _ := event.GetFieldValue("rename.file.path"); value.(string) != oldFile {
				t.Errorf("expected filename not found %s != %s", value.(string), oldFile)
			}

			inode = getInode(t, newFile)

			assert.Equal(t, inode, event.Rename.New.Inode, "wrong rename inode")
			assert.Equal(t, true, event.Rename.Old.IsInUpperLayer(), "should be in upper layer")
			assert.Equal(t, true, event.Rename.New.IsInUpperLayer(), "should be in upper layer")

			validateInodeAndLayerFallback(t, newFile, inode, true)
		}, "test_rule_rename")
	})
}

func TestOverlayOpOverride(t *testing.T) {
	SkipIfNotAvailable(t)
	if testEnvironment == DockerEnvironment {
		t.Skip("Skip test spawning docker containers on docker")
	}

	if _, err := whichNonFatal("docker"); err != nil {
		t.Skip("Skip test where docker is unavailable")
	}

	checkKernelCompatibility(t, "broken containerd support on Suse 12", func(kv *kernel.Version) bool {
		return kv.IsSuse12Kernel()
	})

	checkDockerCompatibility(t, "this test requires docker to use overlayfs", func(docker *dockerInfo) bool {
		return docker.Info["Storage Driver"] != "overlay2"
	})

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_open",
			Expression: `open.file.path == "/tmp/target.txt"`,
		},
		{
			ID:         "test_rule_mkdir",
			Expression: `mkdir.file.path == "/target_dir"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	dockerWrapper, err := newDockerCmdWrapper(test.Root(), test.Root(), "alpine", "")
	if err != nil {
		t.Fatalf("failed to create docker wrapper: %v", err)
	}

	_, err = dockerWrapper.start()
	if err != nil {
		t.Fatalf("failed to start docker wrapper: %v", err)
	}
	t.Cleanup(func() {
		output, err := dockerWrapper.stop()
		if err != nil {
			t.Errorf("failed to stop docker wrapper: %v\n%s", err, string(output))
		}
	})

	output, err := exec.Command(dockerWrapper.executable, "inspect", "--format", "{{ .GraphDriver.Data.MergedDir }}", dockerWrapper.containerID).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to get merged dir: %s: %s", string(output), err)
	}
	containerOverlayMount := strings.TrimSpace(strings.TrimSpace(string(output)))

	openTargetFromOverlayMnt := filepath.Join(containerOverlayMount, "/tmp/target.txt")
	mkdirTargetFromOverlayMnt := filepath.Join(containerOverlayMount, "/target_dir")

	t.Run("open-from-container", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			output, err := dockerWrapper.Command("touch", []string{"/tmp/target.txt"}, nil).CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to touch file from container: %w:\n%s", err, string(output))
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_open")
			assertFieldEqual(t, event, "open.file.path", "/tmp/target.txt")
			assertFieldNotEmpty(t, event, "process.container.id", "container id shouldn't be empty")
		}, "test_rule_open")
	})

	t.Run("open-from-overlay-mnt", func(t *testing.T) {
		flake.MarkOnJobName(t, "ubuntu_25.10")
		test.WaitSignalFromRule(t, func() error {
			output, err := exec.Command("touch", openTargetFromOverlayMnt).CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to touch file from overlay mount: %w:\n%s", err, string(output))
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_open")
			assertFieldEqual(t, event, "open.file.path", openTargetFromOverlayMnt)
			assertFieldEqual(t, event, "process.container.id", "", "container id should be empty")
		}, "test_rule_open")
	})

	t.Run("mkdir-from-overlay-mnt", func(t *testing.T) {
		test.WaitSignalFromRule(t, func() error {
			output, err := exec.Command("mkdir", mkdirTargetFromOverlayMnt).CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to mkdir from overlay mount: %w:\n%s", err, string(output))
			}
			return nil
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_mkdir")
			assertFieldEqual(t, event, "mkdir.file.path", mkdirTargetFromOverlayMnt)
			assertFieldEqual(t, event, "process.container.id", "", "container id should be empty")
		}, "test_rule_mkdir")
	})
}
