// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/iceber/iouring-go"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestOpen(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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

		test.WaitSignal(t, func() error {
			fd, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), syscall.O_CREAT, 0755)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0755)
			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
			assertInode(t, event.Open.File.Inode, getInode(t, testFile))
		})
	}))

	t.Run("openat", func(t *testing.T) {
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assertInode(t, event.Open.File.Inode, getInode(t, testFile))

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	openHow := unix.OpenHow{
		Flags: unix.O_CREAT,
		Mode:  0711,
	}

	t.Run("openat2", func(t *testing.T) {
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			fd, _, errno := syscall.Syscall6(unix.SYS_OPENAT2, 0, uintptr(testFilePtr), uintptr(unsafe.Pointer(&openHow)), unix.SizeofOpenHow, 0, 0)
			if errno != 0 {
				if errno == unix.ENOSYS {
					return ErrSkipTest{"openat2 is not supported"}
				}
				return error(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assertInode(t, event.Open.File.Inode, getInode(t, testFile))

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("creat", ifSyscallSupported("SYS_CREAT", func(t *testing.T, syscallNB uintptr) {
		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			fd, _, errno := syscall.Syscall(syscallNB, uintptr(testFilePtr), 0711, 0)
			if errno != 0 {
				return error(errno)
			}
			return syscall.Close(int(fd))
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC, int(event.Open.Flags), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assertInode(t, event.Open.File.Inode, getInode(t, testFile))

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	}))

	t.Run("truncate", func(t *testing.T) {
		SkipIfNotAvailable(t)

		defer os.Remove(testFile)

		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				return err
			}

			_, err = syscall.Write(int(f.Fd()), []byte("this data will soon be truncated\n"))
			if err != nil {
				return err
			}

			return f.Close()
		}, func(event *model.Event, r *rules.Rule) {})

		test.WaitSignal(t, func() error {
			// truncate
			_, _, errno := syscall.Syscall(syscall.SYS_TRUNCATE, uintptr(testFilePtr), 4, 0)
			if errno != 0 {
				return error(errno)
			}
			return nil
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_TRUNC, int(event.Open.Flags), "wrong flags")
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("open_by_handle_at", func(t *testing.T) {
		defer os.Remove(testFile)

		// wait for this first event
		test.WaitSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
		})

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

		test.WaitSignal(t, func() error {
			fdInt, err := unix.OpenByHandleAt(int(mount.Fd()), h, unix.O_CREAT)
			if err != nil {
				if err == unix.EINVAL {
					return ErrSkipTest{"open_by_handle_at not supported"}
				}
				return fmt.Errorf("OpenByHandleAt: %w", err)
			}
			return unix.Close(fdInt)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags), "wrong flags")
			assertInode(t, event.Open.File.Inode, getInode(t, testFile))
			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})

	t.Run("io_uring", func(t *testing.T) {
		SkipIfNotAvailable(t)

		defer os.Remove(testFile)

		err = test.GetSignal(t, func() error {
			f, err := os.OpenFile(testFile, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
		})
		if err != nil {
			// if the file was not created, we can't open it with io_uring
			t.Fatal(err)
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

		test.WaitSignal(t, func() error {
			if _, err = iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			fd, err := result.ReturnInt()
			if err != nil {
				if err == syscall.EBADF || err == syscall.EINVAL {
					return ErrSkipTest{"openat not supported by io_uring"}
				}
				return err
			}

			if fd < 0 {
				return fmt.Errorf("failed to open file with io_uring: %d", fd)
			}

			return unix.Close(fd)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			// O_LARGEFILE is added by io_uring during __io_openat_prep
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags&0xfff), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0747)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), true)

			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			assertFieldEqual(t, event, "process.file.path", executable)
		})

		prepRequest, err = iouring.Openat2(unix.AT_FDCWD, testFile, &openHow)
		if err != nil {
			t.Fatal(err)
		}

		// same with openat2
		test.WaitSignal(t, func() error {
			if _, err := iour.SubmitRequest(prepRequest, ch); err != nil {
				return err
			}

			result := <-ch
			fd, err := result.ReturnInt()
			if err != nil {
				if err == syscall.EBADF || err == syscall.EINVAL {
					return ErrSkipTest{"openat2 not supported by io_uring"}
				}
				return err
			}

			if fd < 0 {
				return fmt.Errorf("failed to open file with io_uring: %d", fd)
			}

			return unix.Close(fd)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			// O_LARGEFILE is added by io_uring during __io_openat_prep
			assert.Equal(t, syscall.O_CREAT, int(event.Open.Flags&0xfff), "wrong flags")
			assertRights(t, uint16(event.Open.Mode), 0711)
			assert.Equal(t, getInode(t, testFile), event.Open.File.Inode, "wrong inode")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), true)

			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			assertFieldEqual(t, event, "process.file.path", executable)
		})
	})

	_ = os.Remove(testFile)
}

func TestOpenMetadata(t *testing.T) {
	SkipIfNotAvailable(t)

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.file.uid == 98 && open.file.gid == 99`,
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{rule})
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

		test.WaitSignal(t, func() error {
			// CreateWithOptions creates the file and then chmod the user / group. When the file was created it didn't
			// have the right uid / gid, thus didn't match the rule. Open the file again to trigger the rule.
			f, err := os.OpenFile(testFile, os.O_RDONLY, os.FileMode(expectedMode))
			if err != nil {
				return err
			}
			return f.Close()
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "open", event.GetType(), "wrong event type")
			assertRights(t, event.Open.File.Mode, expectedMode)
			assertInode(t, event.Open.File.Inode, getInode(t, testFile))
			assertNearTime(t, event.Open.File.MTime)
			assertNearTime(t, event.Open.File.CTime)

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)
		})
	})
}

func TestOpenDiscarded(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_open_pipefs",
			Expression: `open.file.mode & S_IFMT == S_IFIFO && process.comm == "testsuite"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("pipefs", func(t *testing.T) {
		SkipIfNotAvailable(t)

		var pipeFDs [2]int
		if err := unix.Pipe(pipeFDs[:]); err != nil {
			t.Fatal(err)
		}
		defer unix.Close(pipeFDs[0])
		defer unix.Close(pipeFDs[1])

		path := fmt.Sprintf("/proc/self/fd/%d", pipeFDs[1])

		err := test.GetSignal(t, func() error {
			fd, err := unix.Open(path, unix.O_WRONLY, 0o0)
			if err != nil {
				return err
			}
			return unix.Close(fd)
		}, func(e *model.Event, r *rules.Rule) {
			t.Error("shouldn't have received an event")
		})
		if err == nil {
			t.Error("shouldn't have received an event")
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
	test, err := newTestModule(b, nil, rules, withStaticOpts(testOpts{disableFilters: disableFilters}))
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
	test, err := newTestModule(b, nil, rules)
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
