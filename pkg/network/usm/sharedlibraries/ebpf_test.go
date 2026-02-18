// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

type EbpfProgramSuite struct {
	suite.Suite
}

func TestEbpfProgram(t *testing.T) {
	ebpftest.TestBuildModes(t, usmtestutil.SupportedBuildModes(), "", func(t *testing.T) {
		if !IsSupported(ebpf.NewConfig()) {
			t.Skip("shared-libraries monitoring is not supported on this configuration")
		}

		suite.Run(t, new(EbpfProgramSuite))
	})
}

func (s *EbpfProgramSuite) TestExpectedLibrariesAreDetected() {
	t := s.T()

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)
	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(prog.Stop)

	libsetToSampleLibraries := map[Libset][]string{
		LibsetCrypto: {
			"libssl.so",
			"libcrypto.so",
		},
		LibsetLibc: {
			"libc.so",
		},
		LibsetGPU: {
			"libcudart.so",
			"libnd4jcuda.so",
			"libcuda.so",
		},
	}

	for libset, libraries := range libsetToSampleLibraries {
		t.Run(string(libset), func(t *testing.T) {
			require.NoError(t, prog.InitWithLibsets(libset))
			for _, library := range libraries {
				t.Run(library, func(t *testing.T) {
					// Add foo prefix to ensure we only get opens from our test file
					filename := "foo-" + library
					tempFile, _ := createTempTestFile(t, filename)

					var receivedEventMutex sync.Mutex
					receivedEvents := make(map[uint32]*LibPath)
					unsub, err := prog.Subscribe(func(path LibPath) {
						receivedEventMutex.Lock()
						defer receivedEventMutex.Unlock()
						if strings.Contains(path.String(), filename) {
							receivedEvents[path.Pid] = &path
						}
					}, libset)
					require.NoError(t, err)
					t.Cleanup(unsub)

					command, err := fileopener.OpenFromAnotherProcess(t, tempFile)
					require.NoError(t, err)
					require.NotNil(t, command.Process)
					t.Cleanup(func() {
						command.Process.Kill()
					})

					require.Eventually(t, func() bool {
						receivedEventMutex.Lock()
						defer receivedEventMutex.Unlock()
						_, exists := receivedEvents[uint32(command.Process.Pid)]
						return exists
					}, 1*time.Second, 10*time.Millisecond)

					require.Equal(t, tempFile, receivedEvents[uint32(command.Process.Pid)].String())
				})
			}
		})
	}
}

func (s *EbpfProgramSuite) TestCanInstantiateMultipleTimes() {
	t := s.T()
	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(func() {
		if prog != nil {
			prog.Stop()
		}
	})

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto))
	prog.Stop()
	prog = nil

	prog2 := GetEBPFProgram(cfg)
	require.NotNil(t, prog2)
	t.Cleanup(prog2.Stop)
	require.NoError(t, prog2.InitWithLibsets(LibsetCrypto))
}

func (s *EbpfProgramSuite) TestProgramReceivesEventsWithSingleLibset() {
	t := s.T()
	fooPath1, _ := createTempTestFile(t, "foo-libssl.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(prog.Stop)

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto))

	var eventMutex sync.Mutex
	var receivedEvent *LibPath
	cb := func(path LibPath) {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		lp := path.String()
		if strings.Contains(lp, "foo-libssl.so") {
			receivedEvent = &path
		}
	}

	unsub, err := prog.Subscribe(cb, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsub)

	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)
	require.NotNil(t, command1.Process)
	t.Cleanup(func() {
		if command1 != nil && command1.Process != nil {
			command1.Process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		return receivedEvent != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPath1, receivedEvent.String())
	require.Equal(t, command1.Process.Pid, int(receivedEvent.Pid))
}

func (s *EbpfProgramSuite) TestSingleProgramReceivesMultipleLibsetEvents() {
	t := s.T()
	fooPathSsl, _ := createTempTestFile(t, "foo-libssl.so")
	fooPathCuda, _ := createTempTestFile(t, "foo-libcudart.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	prog := GetEBPFProgram(cfg)
	require.NotNil(t, prog)
	t.Cleanup(prog.Stop)

	require.NoError(t, prog.InitWithLibsets(LibsetCrypto, LibsetGPU))

	var eventMutex sync.Mutex
	var receivedEventSsl, receivedEventCuda *LibPath
	cbSsl := func(path LibPath) {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		receivedEventSsl = &path
	}
	cbCuda := func(path LibPath) {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		receivedEventCuda = &path
	}

	unsubSsl, err := prog.Subscribe(cbSsl, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsubSsl)

	unsubCuda, err := prog.Subscribe(cbCuda, LibsetGPU)
	require.NoError(t, err)
	t.Cleanup(unsubCuda)

	commandSsl, err := fileopener.OpenFromAnotherProcess(t, fooPathSsl)
	require.NoError(t, err)
	require.NotNil(t, commandSsl.Process)
	t.Cleanup(func() {
		if commandSsl != nil && commandSsl.Process != nil {
			commandSsl.Process.Kill()
		}
	})

	commandCuda, err := fileopener.OpenFromAnotherProcess(t, fooPathCuda)
	require.NoError(t, err)
	require.NotNil(t, commandCuda.Process)
	t.Cleanup(func() {
		if commandCuda != nil && commandCuda.Process != nil {
			commandCuda.Process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		eventMutex.Lock()
		defer eventMutex.Unlock()
		return receivedEventSsl != nil && receivedEventCuda != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPathSsl, receivedEventSsl.String())
	require.Equal(t, commandSsl.Process.Pid, int(receivedEventSsl.Pid))

	require.Equal(t, fooPathCuda, receivedEventCuda.String())
	require.Equal(t, commandCuda.Process.Pid, int(receivedEventCuda.Pid))
}

func (s *EbpfProgramSuite) TestMultipleProgramsReceiveMultipleLibsetEvents() {
	t := s.T()
	fooPathSsl, _ := createTempTestFile(t, "foo-libssl.so")
	fooPathCuda, _ := createTempTestFile(t, "foo-libcudart.so")

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	progSsl := GetEBPFProgram(cfg)
	require.NotNil(t, progSsl)
	t.Cleanup(progSsl.Stop)

	require.NoError(t, progSsl.InitWithLibsets(LibsetCrypto))

	// To ensure that we're not having data races in the test code
	var receivedEventMutex sync.Mutex

	var receivedEventSsl *LibPath
	cbSsl := func(path LibPath) {
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
		receivedEventSsl = &path
	}

	unsubSsl, err := progSsl.Subscribe(cbSsl, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsubSsl)

	progCuda := GetEBPFProgram(cfg)
	require.NotNil(t, progCuda)
	t.Cleanup(progCuda.Stop)

	require.NoError(t, progCuda.InitWithLibsets(LibsetGPU))

	var receivedEventCuda *LibPath
	cbCuda := func(path LibPath) {
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
		receivedEventCuda = &path
	}

	unsubCuda, err := progCuda.Subscribe(cbCuda, LibsetGPU)
	require.NoError(t, err)
	t.Cleanup(unsubCuda)

	commandSsl, err := fileopener.OpenFromAnotherProcess(t, fooPathSsl)
	require.NoError(t, err)
	require.NotNil(t, commandSsl.Process)
	t.Cleanup(func() {
		if commandSsl != nil && commandSsl.Process != nil {
			commandSsl.Process.Kill()
		}
	})

	commandCuda, err := fileopener.OpenFromAnotherProcess(t, fooPathCuda)
	require.NoError(t, err)
	require.NotNil(t, commandCuda.Process)
	t.Cleanup(func() {
		if commandCuda != nil && commandCuda.Process != nil {
			commandCuda.Process.Kill()
		}
	})

	require.Eventually(t, func() bool {
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
		return receivedEventSsl != nil && receivedEventCuda != nil
	}, 1*time.Second, 10*time.Millisecond)

	require.Equal(t, fooPathSsl, receivedEventSsl.String())
	require.Equal(t, commandSsl.Process.Pid, int(receivedEventSsl.Pid))

	require.Equal(t, fooPathCuda, receivedEventCuda.String())
	require.Equal(t, commandCuda.Process.Pid, int(receivedEventCuda.Pid))
}

func (s *EbpfProgramSuite) TestIgnoreOpenedForWrite() {
	t := s.T()
	tests := []struct {
		syscallType string
		skipFunc    func(t *testing.T)
	}{
		{
			syscallType: "open",
		},
		{
			syscallType: "openat",
		},
		{
			syscallType: "openat2",
			skipFunc: func(t *testing.T) {
				if !sysOpenAt2Supported() {
					t.Skip("openat2 not supported")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.syscallType, func(t *testing.T) {
			if tt.skipFunc != nil {
				tt.skipFunc(t)
			}
			// Since we want to detect that the write _hasn't_ been detected, verify the
			// read too to try to ensure that test isn't broken and failing to detect
			// the write due to some bug in the test itself.
			readPath, _ := createTempTestFile(t, "read-foo-libssl.so")
			writePath, _ := createTempTestFile(t, "write-foo-libssl.so")

			cfg := ebpf.NewConfig()
			require.NotNil(t, cfg)

			progSsl := GetEBPFProgram(cfg)
			require.NotNil(t, progSsl)
			t.Cleanup(progSsl.Stop)

			require.NoError(t, progSsl.InitWithLibsets(LibsetCrypto))

			var receivedEventMutex sync.Mutex
			var receivedEvents []LibPath
			callback := func(path LibPath) {
				receivedEventMutex.Lock()
				defer receivedEventMutex.Unlock()
				receivedEvents = append(receivedEvents, path)
			}

			unsub, err := progSsl.Subscribe(callback, LibsetCrypto)
			require.NoError(t, err)
			t.Cleanup(unsub)

			how := unix.OpenHow{Mode: 0644}
			how.Flags = syscall.O_CREAT | syscall.O_WRONLY

			require.EventuallyWithT(t, func(c *assert.CollectT) {
				how.Flags = syscall.O_CREAT | syscall.O_RDONLY
				fd, err := open(unix.AT_FDCWD, readPath, &how, tt.syscallType)
				require.NoError(c, err)
				require.NoError(c, syscall.Close(fd))

				how.Flags = syscall.O_CREAT | syscall.O_WRONLY
				fd, err = open(unix.AT_FDCWD, writePath, &how, tt.syscallType)
				require.NoError(c, err)
				require.NoError(c, syscall.Close(fd))

				// Sleep to ensure that the event is received
				time.Sleep(10 * time.Millisecond)

				receivedEventMutex.Lock()
				defer receivedEventMutex.Unlock()

				assert.Greater(c, len(receivedEvents), 0, "no events received")

				containsReadPath, containsWritePath := false, false
				for _, event := range receivedEvents {
					if event.String() == readPath {
						containsReadPath = true
					} else if event.String() == writePath {
						containsWritePath = true
					}
				}

				assert.True(c, containsReadPath, "expected event to be received for read path")
				assert.False(c, containsWritePath, "expected event to not be received for write path")

				// Reset the received events to avoid false positives on the next iteration
				receivedEvents = nil
			}, time.Second*3, 100*time.Millisecond)
		})
	}
}

// open abstracts open, openat, and openat2
func open(dirfd int, pathname string, how *unix.OpenHow, syscallType string) (int, error) {
	switch syscallType {
	case "open":
		return unix.Open(pathname, int(how.Flags), uint32(how.Mode))
	case "openat":
		return unix.Openat(dirfd, pathname, int(how.Flags), uint32(how.Mode))
	case "openat2":
		return unix.Openat2(dirfd, pathname, how)
	default:
		return -1, fmt.Errorf("unsupported syscall type: %s", syscallType)
	}
}

func (s *EbpfProgramSuite) TestLongPathsIgnored() {
	t := s.T()
	const (
		fileName             = "foo-libssl.so"
		nullTerminatorLength = len("\x00")
	)

	padLength := LibPathMaxSize - len(fileName) - len(t.TempDir()) - len("_") - len(string(filepath.Separator)) - nullTerminatorLength
	fooPath1, _ := createTempTestFile(t, strings.Repeat("a", padLength)+"_"+fileName)
	// fooPath2 is longer than the limit we have, thus it will be ignored.
	fooPath2, _ := createTempTestFile(t, strings.Repeat("a", padLength+1)+"_"+fileName)

	cfg := ebpf.NewConfig()
	require.NotNil(t, cfg)

	progSsl := GetEBPFProgram(cfg)
	require.NotNil(t, progSsl)
	t.Cleanup(progSsl.Stop)

	require.NoError(t, progSsl.InitWithLibsets(LibsetCrypto))

	var receivedEventMutex sync.Mutex
	var receivedEvents []LibPath
	callback := func(path LibPath) {
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()
		receivedEvents = append(receivedEvents, path)
	}

	unsub, err := progSsl.Subscribe(callback, LibsetCrypto)
	require.NoError(t, err)
	t.Cleanup(unsub)

	// create files
	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)
	t.Cleanup(func() {
		if command1 != nil && command1.Process != nil {
			command1.Process.Kill()
		}
	})
	command2, err := fileopener.OpenFromAnotherProcess(t, fooPath2)
	require.NoError(t, err)
	t.Cleanup(func() {
		if command2 != nil && command2.Process != nil {
			command2.Process.Kill()
		}
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		receivedEventMutex.Lock()
		defer receivedEventMutex.Unlock()

		assert.Greater(t, len(receivedEvents), 0, "no events received")
		containsFooPath1, containsFooPath2 := false, false
		for _, event := range receivedEvents {
			if event.String() == fooPath1 {
				containsFooPath1 = true
			} else if event.String() == fooPath2 {
				containsFooPath2 = true
			}
		}

		assert.True(c, containsFooPath1, "expected event to be received for fooPath1")
		assert.False(c, containsFooPath2, "expected event to not be received for fooPath2")

		// Reset the received events to avoid false positives on the next iteration
		receivedEvents = nil
	}, time.Second*3, 100*time.Millisecond)
}

func zeroPages(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// This test ensures that the shared library watcher correctly identifies and processes the first file path in memory,
// even when a second path is present, particularly in scenarios where the first path crosses a memory page boundary.
// The goal is to verify that the presence of the second path does not inadvertently cause the watcher to send to the
// user mode the first path. Before each iteration, the memory-mapped pages are zeroed to ensure consistent and isolated
// test conditions.
func (s *EbpfProgramSuite) TestValidPathExistsInTheMemory() {
	t := s.T()
	pageSize := os.Getpagesize()

	// We want to allocate two contiguous pages and ensure that the address
	// after the two pages is inaccessible. So allocate 3 pages and change the
	// protection of the last one with mprotect(2). If we only map two pages the
	// kernel may merge this mmaping with another existing mapping after it.
	data, err := syscall.Mmap(-1, 0, 3*pageSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Munmap(data) })

	err = syscall.Mprotect(data[2*pageSize:], 0)
	require.NoError(t, err)
	// Truncate the size so that the range loop on it in zeroPages() does not
	// access the memory we've disabled access to.
	data = data[:2*pageSize]

	dummyPath, _ := createTempTestFile(t, "dummy.text")
	soPath, _ := createTempTestFile(t, "foo-libssl.so")

	tests := []struct {
		name       string
		writePaths func(data []byte, textFilePath, soPath string) int
	}{
		{
			// Paths are written consecutively in memory, without crossing a page boundary.
			name: "sanity",
			writePaths: func(data []byte, textFilePath, soPath string) int {
				copy(data, textFilePath)
				data[len(textFilePath)] = 0 // Null-terminate the first path
				copy(data[len(textFilePath)+1:], soPath)

				return 0
			},
		},
		{
			// Paths are written consecutively in memory, at the end of a page.
			name: "end of a page",
			writePaths: func(data []byte, textFilePath, soPath string) int {
				offset := 2*pageSize - len(textFilePath) - 1 - len(soPath) - 1
				copy(data[offset:], textFilePath)
				data[offset+len(textFilePath)] = 0 // Null-terminate the first path
				copy(data[offset+len(textFilePath)+1:], soPath)
				data[offset+len(textFilePath)+1+len(soPath)] = 0 // Null-terminate the second path

				return offset
			},
		},
		{
			// The first path is written at the end of the first page, and the second path is written at the beginning
			// of the second page.
			name: "cross pages",
			writePaths: func(data []byte, textFilePath, soPath string) int {
				// Ensure the first path ends near the end of the first page, crossing into the second page
				offset := pageSize - len(textFilePath) - 1
				copy(data[offset:], textFilePath)
				data[offset+len(textFilePath)] = 0 // Null-terminate the first path
				copy(data[pageSize:], soPath)

				return offset
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zeroPages(data)

			// Ensure the first path ends near the end of the first page, crossing into the second page
			offset := tt.writePaths(data, dummyPath, soPath)

			cfg := ebpf.NewConfig()
			require.NotNil(t, cfg)

			progSsl := GetEBPFProgram(cfg)
			require.NotNil(t, progSsl)
			t.Cleanup(progSsl.Stop)

			require.NoError(t, progSsl.InitWithLibsets(LibsetCrypto))

			var receivedEventMutex sync.Mutex
			var receivedEvents []LibPath
			callback := func(path LibPath) {
				receivedEventMutex.Lock()
				defer receivedEventMutex.Unlock()
				receivedEvents = append(receivedEvents, path)
			}

			unsub, err := progSsl.Subscribe(callback, LibsetCrypto)
			require.NoError(t, err)
			t.Cleanup(unsub)

			pathPtr := uintptr(unsafe.Pointer(&data[offset]))
			dirfd := int(unix.AT_FDCWD)
			fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, uintptr(dirfd), pathPtr, uintptr(os.O_RDONLY), 0644, 0, 0)
			require.Zero(t, errno)
			t.Cleanup(func() { _ = syscall.Close(int(fd)) })

			// Since we want to verify that the write _hasn't_ been detected, we need to try it multiple times
			// to avoid race conditions.
			for i := 0; i < 10; i++ {
				time.Sleep(100 * time.Millisecond)
				// We can have events from other sources, but none should contain the paths we wrote
				for _, event := range receivedEvents {
					assert.NotContains(t, event.String(), soPath)
					assert.NotContains(t, event.String(), dummyPath)
				}
			}
		})
	}
}

func createTempTestFile(t *testing.T, name string) (string, utils.PathIdentifier) {
	fullPath := filepath.Join(t.TempDir(), name)

	f, err := os.Create(fullPath)
	f.WriteString("foobar")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() {
		os.RemoveAll(fullPath)
	})

	pathID, err := utils.NewPathIdentifier(fullPath)
	require.NoError(t, err)

	return fullPath, pathID
}
