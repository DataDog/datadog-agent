// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestOpen(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("open", func(t *testing.T) {
		fd, _, errno := syscall.Syscall(syscall.SYS_OPEN, uintptr(testFilePtr), syscall.O_CREAT, 0755)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(testFile)
		defer syscall.Close(int(fd))

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if flags := event.Open.Flags; flags != syscall.O_CREAT {
				t.Errorf("expected open flag O_CREAT, got %d", flags)
			}

			if mode := event.Open.Mode; mode != 0755 {
				t.Errorf("expected open mode 0755, got %#o", mode)
			}

			if inode := getInode(t, testFile); inode != event.Open.File.Inode {
				t.Logf("expected inode %d, got %d", event.Open.File.Inode, inode)
			}

			testContainerPath(t, event, "open.file.container_path")
		}
	})

	t.Run("openat", func(t *testing.T) {
		fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer os.Remove(testFile)
		defer syscall.Close(int(fd))

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if flags := event.Open.Flags; flags != syscall.O_CREAT {
				t.Errorf("expected open mode O_CREAT, got %d", flags)
			}

			if mode := event.Open.Mode; mode != 0711 {
				t.Errorf("expected open mode 0711, got %#o", mode)
			}
			if inode := getInode(t, testFile); inode != event.Open.File.Inode {
				t.Logf("expected inode %d, got %d", event.Open.File.Inode, inode)
			}

			testContainerPath(t, event, "open.file.container_path")
		}
	})

	t.Run("openat2", func(t *testing.T) {
		openHow := unix.OpenHow{
			Flags: unix.O_CREAT,
			Mode:  0711,
		}

		fd, _, errno := syscall.Syscall6(unix.SYS_OPENAT2, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&openHow)), unix.SizeofOpenHow, 0, 0)
		if errno != 0 {
			if errno == unix.ENOSYS {
				t.Skip("openat2 is not supported")
			}
			t.Fatal(errno)
		}
		defer os.Remove(testFile)
		defer syscall.Close(int(fd))

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if flags := event.Open.Flags; flags != syscall.O_CREAT {
				t.Errorf("expected open mode O_CREAT, got %d", flags)
			}

			if mode := event.Open.Mode; mode != 0711 {
				t.Errorf("expected open mode 0711, got %#o", mode)
			}
			if inode := getInode(t, testFile); inode != event.Open.File.Inode {
				t.Errorf("expected inode %d, got %d", event.Open.File.Inode, inode)
			}

			testContainerPath(t, event, "open.file.container_path")
		}
	})

	t.Run("creat", func(t *testing.T) {
		fd, _, errno := syscall.Syscall(syscall.SYS_CREAT, uintptr(testFilePtr), 0, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer syscall.Close(int(fd))
		defer os.Remove(testFile)

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if flags := event.Open.Flags; flags != syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC {
				t.Errorf("expected open mode O_CREAT|O_WRONLY|O_TRUNC, got %d", flags)
			}

			if inode := getInode(t, testFile); inode != event.Open.File.Inode {
				t.Logf("expected inode %d, got %d", event.Open.File.Inode, inode)
			}

			testContainerPath(t, event, "open.file.container_path")
		}
	})

	t.Run("truncate", func(t *testing.T) {
		f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		}

		syscall.Write(int(f.Fd()), []byte("this data will soon be truncated\n"))

		// truncate
		fd, _, errno := syscall.Syscall(syscall.SYS_TRUNCATE, uintptr(testFilePtr), 4, 0)
		if errno != 0 {
			t.Fatal(error(errno))
		}
		defer syscall.Close(int(fd))

		event, _, err = test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, get %s", event.GetType())
			}

			if flags := event.Open.Flags; flags != syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC {
				t.Errorf("expected open mode O_CREAT|O_WRONLY|O_TRUNC, got %s", model.OpenFlags(flags))
			}

			if inode := getInode(t, testFile); inode != event.Open.File.Inode {
				t.Logf("expected inode %d, got %d", event.Open.File.Inode, inode)
			}

			testContainerPath(t, event, "open.file.container_path")
		}
	})

	t.Run("open_by_handle_at", func(t *testing.T) {
		h, mountID, err := unix.NameToHandleAt(unix.AT_FDCWD, testFile, 0)
		if err != nil {
			if err == unix.ENOTSUP {
				t.Skip("NameToHandleAt is not supported")
			}
			t.Fatalf("NameToHandleAt: %v", err)
		}
		mount, err := openMountByID(mountID)
		if err != nil {
			t.Fatalf("openMountByID: %v", err)
		}
		defer mount.Close()

		fdInt, err := unix.OpenByHandleAt(int(mount.Fd()), h, unix.O_CREAT)
		if err != nil {
			if err == unix.EINVAL {
				t.Skip("open_by_handle_at not supported")
			}
			t.Fatalf("OpenByHandleAt: %v", err)
		}
		defer unix.Close(fdInt)

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if flags := event.Open.Flags; flags != syscall.O_CREAT {
				t.Errorf("expected open mode O_RDWR, got %d", flags)
			}

			if inode := getInode(t, testFile); inode != event.Open.File.Inode {
				t.Logf("expected inode %d, got %d", event.Open.File.Inode, inode)
			}

			testContainerPath(t, event, "open.file.container_path")
		}
	})

	_ = os.Remove(testFile)
}

func TestOpenMetadata(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.file.uid == 98 && open.file.gid == 99`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := applyUmask(fileMode)
	testFile, _, err := test.CreateWithOptions("test-open", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("metadata", func(t *testing.T) {
		// CreateWithOptions creates the file and then chmod the user / group. When the file was created it didn't
		// have the right uid / gid, thus didn't match the rule. Open the file again to trigger the rule.
		f, err := os.Open(testFile)
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(testFile)
		defer f.Close()

		event, _, err := test.GetEvent()
		if err != nil {
			t.Error(err)
		} else {
			if event.GetType() != "open" {
				t.Errorf("expected open event, got %s", event.GetType())
			}

			if int(event.Open.File.Mode) & expectedMode != expectedMode {
				t.Errorf("expected mode %d, got %d", expectedMode, int(event.Open.File.Mode) & expectedMode)
			}

			now := time.Now()
			if event.Open.File.MTime.After(now) || event.Open.File.MTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected mtime close to %s, got %s", now, event.Open.File.MTime)
			}

			if event.Open.File.CTime.After(now) || event.Open.File.CTime.Before(now.Add(-1 * time.Hour)) {
				t.Errorf("expected ctime close to %s, got %s", now, event.Open.File.CTime)
			}
		}
	})
}

func openMountByID(mountID int) (f *os.File, err error) {
	mi, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer mi.Close()
	bs := bufio.NewScanner(mi)
	wantPrefix := []byte(fmt.Sprintf("%v ", mountID))
	for bs.Scan() {
		if !bytes.HasPrefix(bs.Bytes(), wantPrefix) {
			continue
		}
		fields := strings.Fields(bs.Text())
		dev := fields[4]
		return os.Open(dev)
	}
	if err := bs.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("mountID not found")
}

func benchmarkOpenSameFile(b *testing.B, disableFilters bool, rules ...*rules.RuleDefinition) {
	test, err := newTestModule(nil, rules, testOpts{disableFilters: disableFilters})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("benchtest")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0777)
		if err != nil {
			b.Fatal(err)
		}

		if err := syscall.Close(fd); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenNoApprover(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/donotmatch"`,
	}

	benchmarkOpenSameFile(b, true, rule)
}

func BenchmarkOpenWithApprover(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/donotmatch"`,
	}

	benchmarkOpenSameFile(b, false, rule)
}

func BenchmarkOpenNoKprobe(b *testing.B) {
	benchmarkOpenSameFile(b, true)
}

func createFolder(current string, filesPerFolder, maxDepth int) error {
	os.MkdirAll(current, 0777)

	for i := 0; i < filesPerFolder; i++ {
		f, err := os.Create(path.Join(current, fmt.Sprintf("file%d", i)))
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	if maxDepth > 0 {
		if err := createFolder(path.Join(current, fmt.Sprintf("dir%d", maxDepth)), filesPerFolder, maxDepth-1); err != nil {
			return err
		}
	}

	return nil
}

func benchmarkFind(b *testing.B, filesPerFolder, maxDepth int, rules ...*rules.RuleDefinition) {
	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	if err := createFolder(test.Root(), filesPerFolder, maxDepth); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		findCmd := exec.Command("/usr/bin/find", test.Root())
		if err := findCmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind(b *testing.B) {
	benchmarkFind(b, 128, 8, &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/donotmatch"`,
	})
}

func BenchmarkFindNoKprobe(b *testing.B) {
	benchmarkFind(b, 128, 8)
}
