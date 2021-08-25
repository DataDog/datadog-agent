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
	"unsafe"

	"github.com/iceber/iouring-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestOpen(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("open", ifSyscallSupported("SYS_OPEN", func(t *testing.T, syscallNB uintptr) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			fd, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), syscall.O_CREAT, 0755)
			if errno != 0 {
				t.Fatal(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0755)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")

			if !validateOpenSchema(t, event) {
				t.Error(event.String())
			}
		})
	}))

	t.Run("openat", func(t *testing.T) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
			if errno != 0 {
				t.Fatal(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})
	})

	openHow := unix.OpenHow{
		Flags: unix.O_CREAT,
		Mode:  0711,
	}

	t.Run("openat2", func(t *testing.T) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			fd, _, errno := syscall.Syscall6(unix.SYS_OPENAT2, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&openHow)), unix.SizeofOpenHow, 0, 0)
			if errno != 0 {
				if errno == unix.ENOSYS {
					t.Skip("openat2 is not supported")
				}
				t.Fatal(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})
	})

	t.Run("creat", ifSyscallSupported("SYS_CREAT", func(t *testing.T, syscallNB uintptr) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			fd, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), 0711, 0)
			if errno != 0 {
				t.Fatal(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})
	}))

	t.Run("truncate", func(t *testing.T) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				t.Error(err)
			}

			syscall.Write(int(f.Fd()), []byte("this data will soon be truncated\n"))
			return f.Close()
		}, func(event *sprobe.Event, r *rules.Rule) {})
		if err != nil {
			t.Error(err)
		}

		err = test.GetSignal(t, func() error {
			// truncate
			_, _, errno := syscall.Syscall(syscall.SYS_TRUNCATE, uintptr(testFilePtr), 4, 0)
			if errno != 0 {
				t.Fatal(error(errno))
			}
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC, int(event.Open.Flags), "wrong flags")
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})
	})

	t.Run("open_by_handle_at", func(t *testing.T) {
		defer os.Remove(testFile)

		// wait for this first event
		err = test.GetSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
		})
		if err != nil {
			t.Error(err)
		}

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

		err = test.GetSignal(t, func() error {
			fdInt, err := unix.OpenByHandleAt(int(mount.Fd()), h, unix.O_CREAT)
			if err != nil {
				if err == unix.EINVAL {
					t.Skip("open_by_handle_at not supported")
				}
				t.Fatalf("OpenByHandleAt: %v", err)
			}
			return unix.Close(fdInt)
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})
	})

	t.Run("io_uring", func(t *testing.T) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			return nil
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
		})
		if err != nil {
			t.Error(err)
		}

		iour, err := iouring.New(1)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) {
				t.Fatal(err)
			}
			t.Skip("io_uring not supported")
		}
		defer iour.Close()

		prepRequest, err := iouring.Openat(unix.AT_FDCWD, testFile, syscall.O_CREAT, 0747)
		if err != nil {
			t.Fatal(err)
		}

		ch := make(chan iouring.Result, 1)
		err = test.GetSignal(t, func() error {
			if _, err := iour.SubmitRequest(prepRequest, ch); err != nil {
				t.Fatal(err)
			}

			result := <-ch
			fd, err := result.ReturnInt()
			if err != nil {
				if err != syscall.EBADF {
					t.Fatal(err)
				}
				t.Skip("openat not supported by io_uring")
			}

			if fd < 0 {
				t.Fatalf("failed to open file with io_uring: %d", fd)
			}

			return unix.Close(fd)
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			// O_LARGEFILE is added by io_uring during __io_openat_prep
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags&0xfff), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0747)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})

		prepRequest, err = iouring.Openat2(unix.AT_FDCWD, testFile, &openHow)
		if err != nil {
			t.Fatal(err)
		}

		// same with openat2
		err = test.GetSignal(t, func() error {
			if _, err := iour.SubmitRequest(prepRequest, ch); err != nil {
				t.Fatal(err)
			}

			result := <-ch
			fd, err := result.ReturnInt()
			if err != nil {
				t.Fatal(err)
			}

			if fd < 0 {
				t.Fatalf("failed to open file with io_uring: %d", fd)
			}

			return unix.Close(fd)
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			// O_LARGEFILE is added by io_uring during __io_openat_prep
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags&0xfff), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")
		})
	})

	_ = os.Remove(testFile)
}

func TestOpenMetadata(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.file.uid == 98 && open.file.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	fileMode := 0o447
	expectedMode := uint16(applyUmask(fileMode))
	testFile, _, err := test.CreateWithOptions("test-open", 98, 99, fileMode)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("metadata", func(t *testing.T) {
		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			// CreateWithOptions creates the file and then chmod the user / group. When the file was created it didn't
			// have the right uid / gid, thus didn't match the rule. Open the file again to trigger the rule.
			f, err := os.Open(testFile)
			if err != nil {
				t.Fatal(err)
			}
			return f.Close()
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assertRights(t, uint16(event.Open.File.Mode), expectedMode)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")

			assertNearTime(t, event.Open.File.MTime)
			assertNearTime(t, event.Open.File.CTime)
		})
		if err != nil {
			t.Error(err)
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
	test, err := newTestModule(b, nil, rules, testOpts{disableFilters: disableFilters})
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
	test, err := newTestModule(b, nil, rules, testOpts{})
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
