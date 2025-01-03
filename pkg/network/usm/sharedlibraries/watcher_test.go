// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibraries

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/testutil"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func launchProcessMonitor(t *testing.T, useEventStream bool) {
	pm := monitor.GetProcessMonitor()
	t.Cleanup(pm.Stop)
	require.NoError(t, pm.Initialize(useEventStream))
	if useEventStream {
		monitor.InitializeEventConsumer(testutil.NewTestProcessConsumer(t))
	}
}

type SharedLibrarySuite struct {
	suite.Suite
}

func TestSharedLibrary(t *testing.T) {
	if !usmconfig.TLSSupported(utils.NewUSMEmptyConfig()) {
		t.Skip("shared library tracing not supported for this platform")
	}

	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() {
		modes = append(modes, ebpftest.Prebuilt)
	}

	ebpftest.TestBuildModes(t, modes, "", func(t *testing.T) {
		t.Run("netlink", func(t *testing.T) {
			launchProcessMonitor(t, false)
			suite.Run(t, new(SharedLibrarySuite))
		})
		t.Run("event stream", func(t *testing.T) {
			launchProcessMonitor(t, true)
			suite.Run(t, new(SharedLibrarySuite))
		})
	})
}

func (s *SharedLibrarySuite) TestSharedLibraryDetection() {
	t := s.T()

	fooPath1, fooPathID1 := createTempTestFile(t, "foo-libssl.so")

	registerRecorder := new(utils.CallbackRecorder)
	unregisterRecorder := new(utils.CallbackRecorder)

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re:           regexp.MustCompile(`foo-libssl.so`),
			RegisterCB:   registerRecorder.Callback(),
			UnregisterCB: unregisterRecorder.Callback(),
		},
	)
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Stop)

	// create files
	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)

	require.Eventuallyf(t, func() bool {
		return registerRecorder.CallsForPathID(fooPathID1) == 1
	}, time.Second*10, 100*time.Millisecond, "")

	require.NoError(t, command1.Process.Kill())

	require.Eventually(t, func() bool {
		return unregisterRecorder.CallsForPathID(fooPathID1) == 1
	}, time.Second*10, 100*time.Millisecond)
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

// Test that shared library files opened for writing only are ignored.
func (s *SharedLibrarySuite) TestSharedLibraryIgnoreWrite() {
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
			readPath, readPathID := createTempTestFile(t, "read-foo-libssl.so")
			writePath, writePathID := createTempTestFile(t, "write-foo-libssl.so")

			registerRecorder := new(utils.CallbackRecorder)
			unregisterRecorder := new(utils.CallbackRecorder)

			watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
				Rule{
					Re:           regexp.MustCompile(`foo-libssl.so`),
					RegisterCB:   registerRecorder.Callback(),
					UnregisterCB: unregisterRecorder.Callback(),
				},
			)
			require.NoError(t, err)
			watcher.Start()
			t.Cleanup(watcher.Stop)
			// Overriding PID, to allow the watcher to watch the test process
			watcher.thisPID = 0

			how := unix.OpenHow{Mode: 0644}

			require.EventuallyWithT(t, func(c *assert.CollectT) {
				how.Flags = syscall.O_CREAT | syscall.O_RDONLY
				fd, err := open(unix.AT_FDCWD, readPath, &how, tt.syscallType)
				require.NoError(c, err)
				require.NoError(c, syscall.Close(fd))
				require.GreaterOrEqual(c, 1, registerRecorder.CallsForPathID(readPathID))

				how.Flags = syscall.O_CREAT | syscall.O_WRONLY
				fd, err = open(unix.AT_FDCWD, writePath, &how, tt.syscallType)
				require.NoError(c, err)
				require.NoError(c, syscall.Close(fd))
				require.Equal(c, 0, registerRecorder.CallsForPathID(writePathID))
			}, time.Second*5, 100*time.Millisecond)
		})
	}
}

func (s *SharedLibrarySuite) TestLongPath() {
	t := s.T()

	const (
		fileName             = "foo-libssl.so"
		nullTerminatorLength = len("\x00")
	)
	padLength := LibPathMaxSize - len(fileName) - len(t.TempDir()) - len("_") - len(string(filepath.Separator)) - nullTerminatorLength
	fooPath1, fooPathID1 := createTempTestFile(t, strings.Repeat("a", padLength)+"_"+fileName)
	// fooPath2 is longer than the limit we have, thus it will be ignored.
	fooPath2, fooPathID2 := createTempTestFile(t, strings.Repeat("a", padLength+1)+"_"+fileName)

	registerRecorder := new(utils.CallbackRecorder)
	unregisterRecorder := new(utils.CallbackRecorder)

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re:           regexp.MustCompile(`foo-libssl.so`),
			RegisterCB:   registerRecorder.Callback(),
			UnregisterCB: unregisterRecorder.Callback(),
		},
	)
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Stop)

	// create files
	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)

	command2, err := fileopener.OpenFromAnotherProcess(t, fooPath2)
	require.NoError(t, err)

	require.Eventuallyf(t, func() bool {
		return registerRecorder.CallsForPathID(fooPathID1) == 1 &&
			registerRecorder.CallsForPathID(fooPathID2) == 0
	}, time.Second*10, 100*time.Millisecond, "")

	require.NoError(t, command1.Process.Kill())
	require.NoError(t, command2.Process.Kill())

	require.Eventually(t, func() bool {
		return unregisterRecorder.CallsForPathID(fooPathID1) == 1 &&
			unregisterRecorder.CallsForPathID(fooPathID2) == 0
	}, time.Second*10, 100*time.Millisecond)
}

// Tests that the periodic scan is able to detect processes which are missed by
// the eBPF-based watcher.
func (s *SharedLibrarySuite) TestSharedLibraryDetectionPeriodic() {
	t := s.T()

	// Construct a large path to exceed the limits of the eBPF-based watcher
	// (LIB_PATH_MAX_SIZE).  255 is the max filename size of ext4.  The path
	// size will also include the directories leading up to this filename so the
	// total size will be more.
	var b strings.Builder
	final := "foo-libssl.so"
	for i := 0; i < 255-len(final); i++ {
		b.WriteByte('x')
	}
	b.WriteString(final)
	filename := b.String()

	// Reduce interval to speed up test
	orig := scanTerminatedProcessesInterval
	t.Cleanup(func() { scanTerminatedProcessesInterval = orig })
	scanTerminatedProcessesInterval = 10 * time.Millisecond

	fooPath1, fooPathID1 := createTempTestFile(t, filename)
	errPath, errorPathID := createTempTestFile(t, strings.Replace(filename, "xfoo", "yfoo", 1))

	registerRecorder := new(utils.CallbackRecorder)
	unregisterRecorder := new(utils.CallbackRecorder)

	registerCallback := registerRecorder.Callback()

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re: regexp.MustCompile(`foo-libssl.so`),
			RegisterCB: func(fp utils.FilePath) error {
				registerCallback(fp)
				if fp.ID == errorPathID {
					return utils.ErrEnvironment
				}
				return nil
			},
			UnregisterCB: unregisterRecorder.Callback(),
		},
	)
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Stop)

	// create files
	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	pid := command1.Process.Pid
	require.NoError(t, err)

	command2, err := fileopener.OpenFromAnotherProcess(t, errPath)
	pid2 := command2.Process.Pid
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, registerRecorder.CallsForPathID(fooPathID1), 1)

		// Check that we tried to attach to the process twice.  See w.sync() for
		// why we do it.  We don't actually need to attempt the registration
		// twice, we just need to ensure that the maps were scanned twice but we
		// don't have a hook for that so this check should be good enough.
		assert.Equal(c, registerRecorder.CallsForPathID(errorPathID), 2)
	}, time.Second*10, 100*time.Millisecond, "")

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		watcher.syncMutex.Lock()
		defer watcher.syncMutex.Unlock()

		assert.Contains(c, watcher.scannedPIDs, uint32(pid))
		assert.Contains(c, watcher.scannedPIDs, uint32(pid2))
	}, time.Second*10, 100*time.Millisecond)

	require.NoError(t, command1.Process.Kill())
	require.NoError(t, command2.Process.Kill())

	command1.Process.Wait()
	command2.Process.Wait()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, unregisterRecorder.CallsForPathID(fooPathID1), 1)
	}, time.Second*10, 100*time.Millisecond)

	// Check that clean up of dead processes works.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		watcher.syncMutex.Lock()
		defer watcher.syncMutex.Unlock()

		assert.NotContains(c, watcher.scannedPIDs, uint32(pid))
		assert.NotContains(c, watcher.scannedPIDs, uint32(pid2))
	}, time.Second*10, 100*time.Millisecond)
}

func (s *SharedLibrarySuite) TestSharedLibraryDetectionWithPIDAndRootNamespace() {
	t := s.T()
	_, err := os.Stat("/usr/bin/busybox")
	if err != nil {
		t.Skip("skip for the moment as some distro are not friendly with busybox package")
	}

	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "root")
	err = os.MkdirAll(root, 0755)
	require.NoError(t, err)

	libpath := "/fooroot-crypto.so"

	err = exec.Command("cp", "/usr/bin/busybox", root+"/ash").Run()
	require.NoError(t, err)
	err = exec.Command("cp", "/usr/bin/busybox", root+"/sleep").Run()
	require.NoError(t, err)

	var (
		mux          sync.Mutex
		pathDetected string
	)

	callback := func(path utils.FilePath) error {
		mux.Lock()
		defer mux.Unlock()
		pathDetected = path.HostPath
		return nil
	}

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re:           regexp.MustCompile(`fooroot-crypto.so`),
			RegisterCB:   callback,
			UnregisterCB: utils.IgnoreCB,
		},
	)
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Stop)

	time.Sleep(10 * time.Millisecond)
	// simulate a slow (1 second) : open, read, close of the file
	// in a new pid and mount namespaces
	o, err := exec.Command("unshare", "--fork", "--pid", "-R", root, "/ash", "-c",
		fmt.Sprintf("touch foo && mv foo %s && sleep 1 < %s", libpath, libpath)).CombinedOutput()
	if err != nil {
		t.Log(err, string(o))
	}
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// Ensuring there is no race
	mux.Lock()
	defer mux.Unlock()
	// assert that soWatcher detected foo-libssl.so being opened and triggered the callback
	require.True(t, strings.HasSuffix(pathDetected, libpath))

	// must fail on the host
	_, err = os.Stat(libpath)
	require.Error(t, err)
}

func (s *SharedLibrarySuite) TestSameInodeRegression() {
	t := s.T()

	fooPath1, fooPathID1 := createTempTestFile(t, "a-foo-libssl.so")
	fooPath2 := filepath.Join(t.TempDir(), "b-foo-libssl.so")

	// create a hard-link (a-foo-libssl.so and b-foo-libssl.so will share the same inode)
	require.NoError(t, os.Link(fooPath1, fooPath2))
	fooPathID2, err := utils.NewPathIdentifier(fooPath2)
	require.NoError(t, err)
	require.Equal(t, fooPathID1, fooPathID2)

	registerRecorder := new(utils.CallbackRecorder)
	unregisterRecorder := new(utils.CallbackRecorder)

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re:           regexp.MustCompile(`foo-libssl.so`),
			RegisterCB:   registerRecorder.Callback(),
			UnregisterCB: unregisterRecorder.Callback(),
		},
	)
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Stop)

	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1, fooPath2)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return registerRecorder.CallsForPathID(fooPathID1) == 1 &&
			hasPID(watcher, command1)
	}, time.Second*10, 100*time.Millisecond)

	require.NoError(t, command1.Process.Kill())

	require.Eventually(t, func() bool {
		return unregisterRecorder.CallsForPathID(fooPathID1) == 1 &&
			!hasPID(watcher, command1)
	}, time.Second*10, 100*time.Millisecond)
}

func (s *SharedLibrarySuite) TestSoWatcherLeaks() {
	t := s.T()

	fooPath1, fooPathID1 := createTempTestFile(t, "foo-libssl.so")
	fooPath2, fooPathID2 := createTempTestFile(t, "foo2-gnutls.so")

	registerRecorder := new(utils.CallbackRecorder)
	unregisterRecorder := &utils.CallbackRecorder{
		ReturnError: errors.New("fake unregisterCB error"),
	}

	registerCB := registerRecorder.Callback()
	unregisterCB := unregisterRecorder.Callback()

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re:           regexp.MustCompile(`foo-libssl.so`),
			RegisterCB:   registerCB,
			UnregisterCB: unregisterCB,
		},
		Rule{
			Re:           regexp.MustCompile(`foo2-gnutls.so`),
			RegisterCB:   registerCB,
			UnregisterCB: unregisterCB,
		},
	)
	require.NoError(t, err)
	watcher.Start()
	t.Cleanup(watcher.Stop)

	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1, fooPath2)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		// Checking register callback was executed once for each library
		// and that we're tracking the two command PIDs
		return registerRecorder.CallsForPathID(fooPathID1) == 1 &&
			registerRecorder.CallsForPathID(fooPathID2) == 1 &&
			hasPID(watcher, command1)
	}, time.Second*10, 100*time.Millisecond)

	command2, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		// Check that no more callbacks were executed, but we're tracking two PIDs now
		return registerRecorder.CallsForPathID(fooPathID1) == 1 &&
			registerRecorder.CallsForPathID(fooPathID2) == 1 &&
			hasPID(watcher, command1) &&
			hasPID(watcher, command2)
	}, time.Second*10, 100*time.Millisecond)

	require.NoError(t, command1.Process.Kill())
	require.Eventually(t, func() bool {
		// Checking that the unregisteredCB was executed only for pathID2
		return unregisterRecorder.CallsForPathID(fooPathID1) == 0 &&
			unregisterRecorder.CallsForPathID(fooPathID2) == 1
	}, time.Second*10, 100*time.Millisecond)

	require.NoError(t, command2.Process.Kill())
	require.Eventually(t, func() bool {
		// Checking that the unregisteredCB was executed now for pathID1
		return unregisterRecorder.CallsForPathID(fooPathID1) == 1
	}, time.Second*10, 100*time.Millisecond)

	// Check there are no more processes registered
	assert.Len(t, watcher.registry.GetRegisteredProcesses(), 0)
}

func (s *SharedLibrarySuite) TestSoWatcherProcessAlreadyHoldingReferences() {
	t := s.T()

	fooPath1, fooPathID1 := createTempTestFile(t, "foo-libssl.so")
	fooPath2, fooPathID2 := createTempTestFile(t, "foo2-gnutls.so")

	registerRecorder := new(utils.CallbackRecorder)
	unregisterRecorder := new(utils.CallbackRecorder)
	registerCB := registerRecorder.Callback()
	unregisterCB := unregisterRecorder.Callback()

	watcher, err := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re:           regexp.MustCompile(`foo-libssl.so`),
			RegisterCB:   registerCB,
			UnregisterCB: unregisterCB,
		},
		Rule{
			Re:           regexp.MustCompile(`foo2-gnutls.so`),
			RegisterCB:   registerCB,
			UnregisterCB: unregisterCB,
		},
	)
	require.NoError(t, err)

	command1, err := fileopener.OpenFromAnotherProcess(t, fooPath1, fooPath2)
	require.NoError(t, err)
	command2, err := fileopener.OpenFromAnotherProcess(t, fooPath1)
	require.NoError(t, err)

	watcher.Start()
	t.Cleanup(watcher.Stop)

	require.Eventually(t, func() bool {
		return registerRecorder.CallsForPathID(fooPathID1) == 1 &&
			registerRecorder.CallsForPathID(fooPathID2) == 1 &&
			hasPID(watcher, command1) &&
			hasPID(watcher, command2)
	}, time.Second*10, 100*time.Millisecond)

	require.NoError(t, command1.Process.Kill())
	require.Eventually(t, func() bool {
		// Checking that unregister callback was called for only path2 and that
		// command1 PID is no longer being tracked
		return unregisterRecorder.CallsForPathID(fooPathID1) == 0 &&
			unregisterRecorder.CallsForPathID(fooPathID2) == 1 &&
			!hasPID(watcher, command1) &&
			hasPID(watcher, command2)
	}, time.Second*10, 100*time.Millisecond)

	require.NoError(t, command2.Process.Kill())
	require.Eventually(t, func() bool {
		// Assert that unregisterCB has also been called now for pathID1
		return unregisterRecorder.CallsForPathID(fooPathID1) == 1 &&
			unregisterRecorder.CallsForPathID(fooPathID2) == 1 &&
			!hasPID(watcher, command1) &&
			!hasPID(watcher, command2)
	}, time.Second*10, 100*time.Millisecond)

	// Check there are no more processes registered
	assert.Len(t, watcher.registry.GetRegisteredProcesses(), 0)
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

func BenchmarkScanSOWatcherNew(b *testing.B) {
	w, _ := NewWatcher(utils.NewUSMEmptyConfig(), LibsetCrypto,
		Rule{
			Re: regexp.MustCompile(`libssl.so`),
		},
		Rule{
			Re: regexp.MustCompile(`libcrypto.so`),
		},
		Rule{
			Re: regexp.MustCompile(`libgnutls.so`),
		},
	)

	callback := func(path string) {
		for _, r := range w.rules {
			if r.Re.MatchString(path) {
				break
			}
		}
	}

	f := func(pid int) error {
		mapsPath := fmt.Sprintf("%s/%d/maps", w.procRoot, pid)
		maps, err := os.Open(mapsPath)
		if err != nil {
			log.Debugf("process %d parsing failed %s", pid, err)
			return nil
		}
		defer maps.Close()

		scanner := bufio.NewScanner(bufio.NewReader(maps))

		parseMapsFile(scanner, callback)
		return nil
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		kernel.WithAllProcs(w.procRoot, f)
	}
}

var mapsFile = `
7f178d0a6000-7f178d0cb000 r--p 00000000 fd:00 268741                     /usr/lib/x86_64-linux-gnu/libc-2.31.so
7f178d0cb000-7f178d243000 r-xp 00025000 fd:00 268741                     /usr/lib/x86_64-linux-gnu/libc-2.31.so
7f178d243000-7f178d28d000 r--p 0019d000 fd:00 268741                     /usr/lib/x86_64-linux-gnu/libc-2.31.so
7f178d28d000-7f178d28e000 ---p 001e7000 fd:00 268741                     /usr/lib/x86_64-linux-gnu/libc-2.31.so
7f178d28e000-7f178d291000 r--p 001e7000 fd:00 268741                     /usr/lib/x86_64-linux-gnu/libc-2.31.so
7f178d291000-7f178d294000 rw-p 001ea000 fd:00 268741                     /usr/lib/x86_64-linux-gnu/libc-2.31.so
7f178d294000-7f178d29a000 rw-p 00000000 00:00 0
7f178d29a000-7f178d29b000 r--p 00000000 fd:00 262340                     /usr/lib/locale/C.UTF-8/LC_TELEPHONE
7f178d29b000-7f178d29c000 r--p 00000000 fd:00 262333                     /usr/lib/locale/C.UTF-8/LC_MEASUREMENT
7f178d29c000-7f178d2a3000 r--s 00000000 fd:00 269008                     /usr/lib/x86_64-linux-gnu/gconv/gconv-modules.cache
7f178d2a3000-7f178d2a4000 r--p 00000000 fd:00 268737                     /usr/lib/x86_64-linux-gnu/ld-2.31.so
7f178d2a4000-7f178d2c7000 r-xp 00001000 fd:00 268737                     /usr/lib/x86_64-linux-gnu/ld-2.31.so
7f178d2c7000-7f178d2cf000 r--p 00024000 fd:00 268737                     /usr/lib/x86_64-linux-gnu/ld-2.31.so
7f178d2cf000-7f178d2d0000 r--p 00000000 fd:00 262317                     /usr/lib/locale/C.UTF-8/LC_IDENTIFICATION
7f178d2d0000-7f178d2d1000 r--p 0002c000 fd:00 268737                     /usr/lib/x86_64-linux-gnu/ld-2.31.so
7f178d2d1000-7f178d2d2000 rw-p 0002d000 fd:00 268737                     /usr/lib/x86_64-linux-gnu/ld-2.31.so
7f178d2d1000-7f178d2d2000 rw-p 0002d000 fd:00 268737                     /usr/lib/x86_64-linux-gnu/ld-2.2.so (deleted)
7f178d2d1000-7f178d2d2000 rw-p 0002d000 fd:00 0		                     /usr/lib/x86_64-linux-gnu/ld-2.2.so (deleted)
7f178d2d2000-7f178d2d3000 rw-p 00000000 00:00 0
7ffe712a4000-7ffe712c5000 rw-p 00000000 00:00 0                          [stack]
7ffe71317000-7ffe7131a000 r--p 00000000 00:00 0                          [vvar]
7ffe7131a000-7ffe7131b000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`

func Test_parseMapsFile(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(mapsFile))

	extractedEntries := make([]string, 0)
	expectedEntries := []string{
		"/usr/lib/x86_64-linux-gnu/libc-2.31.so",
		"/usr/lib/locale/C.UTF-8/LC_TELEPHONE",
		"/usr/lib/locale/C.UTF-8/LC_MEASUREMENT",
		"/usr/lib/x86_64-linux-gnu/gconv/gconv-modules.cache",
		"/usr/lib/x86_64-linux-gnu/ld-2.31.so",
		"/usr/lib/locale/C.UTF-8/LC_IDENTIFICATION",
	}
	testCallback := func(path string) {
		extractedEntries = append(extractedEntries, path)
	}

	parseMapsFile(scanner, testCallback)

	require.Equal(t, expectedEntries, extractedEntries)
}

func hasPID(w *Watcher, cmd *exec.Cmd) bool {
	activePIDs := w.registry.GetRegisteredProcesses()
	_, ok := activePIDs[uint32(cmd.Process.Pid)]
	return ok
}
