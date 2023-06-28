// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
)

func launchProcessMonitor(t *testing.T) {
	pm := monitor.GetProcessMonitor()
	t.Cleanup(pm.Stop)
	require.NoError(t, pm.Initialize())
}

func registerProcessTerminationUponCleanup(t *testing.T, cmd *exec.Cmd) {
	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
	})
}

type SharedLibrarySuite struct {
	suite.Suite
}

func TestSharedLibrary(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		suite.Run(t, new(SharedLibrarySuite))
	})
}

func (s *SharedLibrarySuite) TestSharedLibraryDetection() {
	t := s.T()
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "foo-libssl.so")

	var (
		mux          sync.Mutex
		pathDetected string
	)

	callback := func(id pathIdentifier, root string, path string) error {
		mux.Lock()
		defer mux.Unlock()
		pathDetected = path
		return nil
	}

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:         regexp.MustCompile(`foo-libssl.so`),
			registerCB: callback,
		},
	)
	watcher.Start()
	t.Cleanup(watcher.Stop)
	launchProcessMonitor(t)

	// create files
	clientBin := buildSOWatcherClientBin(t)
	command1 := exec.Command(clientBin, fooPath1)
	require.NoError(t, command1.Start())
	registerProcessTerminationUponCleanup(t, command1)

	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPIDNotAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) {
			return false
		}

		// Checking PID1 is not associated to the path 2, and PID2 is associated only with the path2
		return fooPath1 == pathDetected
	}, time.Second*10, time.Second, "")

	require.NoError(t, command1.Process.Kill())

	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		return checkPathIDDoesNotExist(watcher, fooPathID1) && checkPIDNotAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid))
	}, time.Second*10, time.Second, "")

	tel := telemetry.ReportPayloadTelemetry("1")
	telEqual := func(t *testing.T, expected int64, m string) {
		require.Equal(t, expected, tel[m], m)
	}
	require.GreaterOrEqual(t, tel["usm.so_watcher.hits"], tel["usm.so_watcher.matches"], "usm.so_watcher.hits")
	telEqual(t, 0, "usm.so_watcher.already_registered")
	telEqual(t, 0, "usm.so_watcher.blocked")
	telEqual(t, 1, "usm.so_watcher.matches")
	telEqual(t, 1, "usm.so_watcher.registered")
	telEqual(t, 0, "usm.so_watcher.unregister_errors")
	telEqual(t, 1, "usm.so_watcher.unregister_no_callback")
	telEqual(t, 0, "usm.so_watcher.unregister_failed_cb")
	telEqual(t, 0, "usm.so_watcher.unregister_pathid_not_found")
	telEqual(t, 1, "usm.so_watcher.unregistered")
}

func (s *SharedLibrarySuite) TestSharedLibraryDetectionWithPIDandRootNameSpace() {
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

	perfHandler := initEBPFProgram(t)

	var (
		mux          sync.Mutex
		pathDetected string
	)

	callback := func(id pathIdentifier, root string, path string) error {
		mux.Lock()
		defer mux.Unlock()
		pathDetected = path
		return nil
	}

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:         regexp.MustCompile(`fooroot-crypto.so`),
			registerCB: callback,
		},
	)
	watcher.Start()
	t.Cleanup(watcher.Stop)
	launchProcessMonitor(t)

	time.Sleep(10 * time.Millisecond)
	// simulate a slow (1 second) : open, write, close of the file
	// in a new pid and mount namespaces
	o, err := exec.Command("unshare", "--fork", "--pid", "-R", root, "/ash", "-c", fmt.Sprintf("sleep 1 > %s", libpath)).CombinedOutput()
	if err != nil {
		t.Log(err, string(o))
	}
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// assert that soWatcher detected foo-libssl.so being opened and triggered the callback
	require.Equal(t, libpath, pathDetected)

	// must fail on the host
	_, err = os.Stat(libpath)
	require.Error(t, err)

	tel := telemetry.ReportPayloadTelemetry("1")
	telEqual := func(t *testing.T, expected int64, m string) {
		require.Equal(t, expected, tel[m], m)
	}
	require.GreaterOrEqual(t, tel["usm.so_watcher.hits"], tel["usm.so_watcher.matches"], "usm.so_watcher.hits")
	telEqual(t, 0, "usm.so_watcher.already_registered")
	telEqual(t, 0, "usm.so_watcher.blocked")
	telEqual(t, 1, "usm.so_watcher.matches")
	telEqual(t, 1, "usm.so_watcher.registered")
	telEqual(t, 0, "usm.so_watcher.unregister_errors")
	telEqual(t, 1, "usm.so_watcher.unregister_no_callback")
	telEqual(t, 0, "usm.so_watcher.unregister_failed_cb")
	telEqual(t, 0, "usm.so_watcher.unregister_pathid_not_found")
	telEqual(t, 1, "usm.so_watcher.unregistered")
}

func (s *SharedLibrarySuite) TestSameInodeRegression() {
	t := s.T()
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "a-foo-libssl.so")
	fooPath2 := filepath.Join(t.TempDir(), "b-foo-libssl.so")

	// create a hard-link (a-foo-libssl.so and b-foo-libssl.so will share the same inode)
	require.NoError(t, os.Link(fooPath1, fooPath2))
	fooPathID2, err := newPathIdentifier(fooPath2)
	require.NoError(t, err)

	registers := atomic.NewInt64(0)
	callback := func(id pathIdentifier, root string, path string) error {
		registers.Add(1)
		return nil
	}

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:         regexp.MustCompile(`foo-libssl.so`),
			registerCB: callback,
		},
	)
	watcher.Start()
	t.Cleanup(watcher.Stop)
	launchProcessMonitor(t)

	clientBin := buildSOWatcherClientBin(t)
	command1 := exec.Command(clientBin, fooPath1, fooPath2)
	require.NoError(t, command1.Start())
	registerProcessTerminationUponCleanup(t, command1)

	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPathIDDoesNotExist(watcher, fooPathID2) ||
			checkPIDNotAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) ||
			checkPIDNotAssociatedWithPathID(watcher, fooPathID2, uint32(command1.Process.Pid)) {
			return false
		}

		return int64(1) == registers.Load()
	}, time.Second*10, time.Second, "")

	require.Len(t, watcher.registry.byID, 1)
	require.NoError(t, command1.Process.Kill())

	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		return checkPathIDDoesNotExist(watcher, fooPathID1) && checkPathIDDoesNotExist(watcher, fooPathID2) &&
			checkPIDNotAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) &&
			checkPIDNotAssociatedWithPathID(watcher, fooPathID2, uint32(command1.Process.Pid))
	}, time.Second*10, time.Second, "")

	tel := telemetry.ReportPayloadTelemetry("1")
	telEqual := func(t *testing.T, expected int64, m string) {
		require.Equal(t, expected, tel[m], m)
	}
	require.GreaterOrEqual(t, tel["usm.so_watcher.hits"], tel["usm.so_watcher.matches"], "usm.so_watcher.hits")
	telEqual(t, 1, "usm.so_watcher.already_registered")
	telEqual(t, 0, "usm.so_watcher.blocked")
	telEqual(t, 2, "usm.so_watcher.matches") // command1 access to 2 files
	telEqual(t, 1, "usm.so_watcher.registered")
	telEqual(t, 0, "usm.so_watcher.unregister_errors")
	telEqual(t, 1, "usm.so_watcher.unregister_no_callback")
	telEqual(t, 0, "usm.so_watcher.unregister_failed_cb")
	telEqual(t, 0, "usm.so_watcher.unregister_path_id_not_found")
	telEqual(t, 1, "usm.so_watcher.unregistered")
}

func (s *SharedLibrarySuite) TestSoWatcherLeaks() {
	t := s.T()
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "foo-libssl.so")
	fooPath2, fooPathID2 := createTempTestFile(t, "foo2-gnutls.so")

	registerCB := func(id pathIdentifier, root string, path string) error { return nil }
	unregisterCB := func(id pathIdentifier) error { return errors.New("fake unregisterCB error") }

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:           regexp.MustCompile(`foo-libssl.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
		soRule{
			re:           regexp.MustCompile(`foo2-gnutls.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
	)
	watcher.Start()
	t.Cleanup(watcher.Stop)
	launchProcessMonitor(t)

	// create files
	clientBin := buildSOWatcherClientBin(t)

	command1 := exec.Command(clientBin, fooPath1, fooPath2)
	require.NoError(t, command1.Start())
	registerProcessTerminationUponCleanup(t, command1)

	// Check sowatcher map
	require.Eventuallyf(t, func() bool {
		// Checking both paths exist.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPathIDDoesNotExist(watcher, fooPathID2) {
			return false
		}

		// Checking the PID associated with the 2 paths.
		return checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID2, uint32(command1.Process.Pid))
	}, time.Second*10, time.Second, "")

	command2 := exec.Command(clientBin, fooPath1)
	require.NoError(t, command2.Start())
	registerProcessTerminationUponCleanup(t, command2)

	require.Eventuallyf(t, func() bool {
		// Checking both paths exist.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPathIDDoesNotExist(watcher, fooPathID2) {
			return false
		}

		// Checking PID1 is still associated to the 2 paths, and PID2 is associated only with the first path
		return checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID2, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command2.Process.Pid)) &&
			checkPIDNotAssociatedWithPathID(watcher, fooPathID2, uint32(command2.Process.Pid))
	}, time.Second*10, time.Second, "")

	require.NoError(t, command1.Process.Kill())
	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPathIDExists(watcher, fooPathID2) {
			return false
		}

		// Checking PID1 is not associated to the path 2, and PID2 is associated only with the path2
		return checkPIDNotAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command2.Process.Pid))
	}, time.Second*10, time.Second, "")

	require.NoError(t, command2.Process.Kill())
	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		return checkPathIDDoesNotExist(watcher, fooPathID1) && checkPathIDDoesNotExist(watcher, fooPathID2)
	}, time.Second*10, time.Second, "")

	checkWatcherStateIsClean(t, watcher)

	tel := telemetry.ReportPayloadTelemetry("1")
	telEqual := func(t *testing.T, expected int64, m string) {
		require.Equal(t, expected, tel[m], m)
	}
	require.GreaterOrEqual(t, tel["usm.so_watcher.hits"], tel["usm.so_watcher.matches"], "usm.so_watcher.hits")
	telEqual(t, 1, "usm.so_watcher.already_registered")
	telEqual(t, 0, "usm.so_watcher.blocked")
	telEqual(t, 3, "usm.so_watcher.matches") // command1 access to 2 files, command2 access to 1 file
	telEqual(t, 2, "usm.so_watcher.registered")
	telEqual(t, 0, "usm.so_watcher.unregister_errors")
	telEqual(t, 0, "usm.so_watcher.unregister_no_callback")
	telEqual(t, 2, "usm.so_watcher.unregister_failed_cb")
	telEqual(t, 0, "usm.so_watcher.unregister_path_id_not_found")
	telEqual(t, 2, "usm.so_watcher.unregistered")
}

func (s *SharedLibrarySuite) TestSoWatcherProcessAlreadyHoldingReferences() {
	t := s.T()
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "foo-libssl.so")
	fooPath2, fooPathID2 := createTempTestFile(t, "foo2-gnutls.so")

	registerCB := func(id pathIdentifier, root string, path string) error { return nil }
	unregisterCB := func(id pathIdentifier) error { return nil }

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:           regexp.MustCompile(`foo-libssl.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
		soRule{
			re:           regexp.MustCompile(`foo2-gnutls.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
	)

	// create files
	clientBin := buildSOWatcherClientBin(t)

	command1 := exec.Command(clientBin, fooPath1, fooPath2)
	require.NoError(t, command1.Start())
	registerProcessTerminationUponCleanup(t, command1)
	command2 := exec.Command(clientBin, fooPath1)
	require.NoError(t, command2.Start())
	registerProcessTerminationUponCleanup(t, command1)
	time.Sleep(time.Second)
	watcher.Start()
	t.Cleanup(watcher.Stop)
	launchProcessMonitor(t)

	require.Eventuallyf(t, func() bool {
		// Checking both paths exist.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPathIDDoesNotExist(watcher, fooPathID2) {
			return false
		}

		// Checking PID1 is still associated to the 2 paths, and PID2 is associated only with the first path
		return checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID2, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command2.Process.Pid)) &&
			checkPIDNotAssociatedWithPathID(watcher, fooPathID2, uint32(command2.Process.Pid))
	}, time.Second*10, time.Second, "")

	require.NoError(t, command1.Process.Kill())
	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		if checkPathIDDoesNotExist(watcher, fooPathID1) || checkPathIDExists(watcher, fooPathID2) {
			return false
		}

		// Checking PID1 is not associated to the path 2, and PID2 is associated only with the path2
		return checkPIDNotAssociatedWithPathID(watcher, fooPathID1, uint32(command1.Process.Pid)) &&
			checkPIDAssociatedWithPathID(watcher, fooPathID1, uint32(command2.Process.Pid))
	}, time.Second*10, time.Second, "")

	require.NoError(t, command2.Process.Kill())
	require.Eventuallyf(t, func() bool {
		// Checking path1 still exists, and path2 not.
		return checkPathIDDoesNotExist(watcher, fooPathID1) && checkPathIDDoesNotExist(watcher, fooPathID2)
	}, time.Second*10, time.Second, "")

	checkWatcherStateIsClean(t, watcher)

	tel := telemetry.ReportPayloadTelemetry("1")
	telEqual := func(t *testing.T, expected int64, m string) {
		require.Equal(t, expected, tel[m], m)
	}
	require.GreaterOrEqual(t, tel["usm.so_watcher.hits"], tel["usm.so_watcher.matches"], "usm.so_watcher.hits")
	telEqual(t, 1, "usm.so_watcher.already_registered")
	telEqual(t, 0, "usm.so_watcher.blocked")
	telEqual(t, 3, "usm.so_watcher.matches") // command1 access to 2 files, command2 access to 1 file
	telEqual(t, 2, "usm.so_watcher.registered")
	telEqual(t, 0, "usm.so_watcher.unregister_errors")
	telEqual(t, 0, "usm.so_watcher.unregister_no_callback")
	telEqual(t, 0, "usm.so_watcher.unregister_failed_cb")
	telEqual(t, 0, "usm.so_watcher.unregister_path_id_not_found")
	telEqual(t, 2, "usm.so_watcher.unregistered")
}

func buildSOWatcherClientBin(t *testing.T) string {
	const ClientSrcPath = "sowatcher_client"
	const ClientBinaryPath = "testutil/sowatcher_client/sowatcher_client"

	t.Helper()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	clientBinary := fmt.Sprintf("%s/%s", cur, ClientBinaryPath)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err = os.Stat(clientBinary); err == nil {
		return clientBinary
	}

	clientSrcDir := fmt.Sprintf("%s/testutil/%s", cur, ClientSrcPath)
	clientBuildDir, err := os.MkdirTemp("", "sowatcher_client_build-")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(clientBuildDir)
	})

	clientBinPath := fmt.Sprintf("%s/sowatcher_client", clientBuildDir)

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", clientBinPath, clientSrcDir)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build client test binary: %s\noutput: %s", err, string(out))

	return clientBinPath
}

func checkPathIDExists(watcher *soWatcher, pathID pathIdentifier) bool {
	_, ok := watcher.registry.byID[pathID]
	return ok
}

func checkPathIDDoesNotExist(watcher *soWatcher, pathID pathIdentifier) bool {
	return !checkPathIDExists(watcher, pathID)
}

func checkPIDAssociatedWithPathID(watcher *soWatcher, pathID pathIdentifier, pid uint32) bool {
	value, ok := watcher.registry.byPID[pid]
	if !ok {
		return false
	}
	_, ok = value[pathID]
	return ok
}

func checkPIDNotAssociatedWithPathID(watcher *soWatcher, pathID pathIdentifier, pid uint32) bool {
	return !checkPIDAssociatedWithPathID(watcher, pathID, pid)
}

func createTempTestFile(t *testing.T, name string) (string, pathIdentifier) {
	fullPath := filepath.Join(t.TempDir(), name)

	f, err := os.Create(fullPath)
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() {
		os.RemoveAll(fullPath)
	})

	pathID, err := newPathIdentifier(fullPath)
	require.NoError(t, err)

	return fullPath, pathID
}

func checkWatcherStateIsClean(t *testing.T, watcher *soWatcher) {
	require.True(t, len(watcher.registry.byPID) == 0 && len(watcher.registry.byID) == 0, "watcher state is not clean")
}

func getTracepointFuncName(tracepointType, name string) string {
	return fmt.Sprintf("tracepoint__syscalls__sys_%s_%s", tracepointType, name)
}

const (
	enterTracepoint = "enter"
	exitTracepoint  = "exit"
)

func initEBPFProgram(t *testing.T) *ddebpf.PerfHandler {
	c := config.New()
	if !http.HTTPSSupported(c) {
		t.Skip("https not supported for this setup")
	}

	includeOpenat2 := sysOpenAt2Supported()
	openat2Probes := []manager.ProbeIdentificationPair{
		{
			EBPFFuncName: getTracepointFuncName(enterTracepoint, openat2SysCall),
			UID:          probeUID,
		},
		{
			EBPFFuncName: getTracepointFuncName(exitTracepoint, openat2SysCall),
			UID:          probeUID,
		},
	}

	perfHandler := ddebpf.NewPerfHandler(10)
	mgr := &manager.Manager{
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: sharedLibrariesPerfMap},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					RecordHandler:      perfHandler.RecordHandler,
					LostHandler:        perfHandler.LostHandler,
					RecordGetter:       perfHandler.RecordGetter,
				},
			},
		},
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: getTracepointFuncName(enterTracepoint, openatSysCall),
					UID:          probeUID,
				},
				KProbeMaxActive: maxActive,
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: getTracepointFuncName(exitTracepoint, openatSysCall),
					UID:          probeUID,
				},
				KProbeMaxActive: maxActive,
			},
		},
	}

	options := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
		MapSpecEditors: map[string]manager.MapSpecEditor{
			// TODO: move shared library probes to their own compilation artifact
			"http_batches": {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			"http2_batches": {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			"http_in_flight": {
				Type:       ebpf.LRUHash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			"kafka_batches": {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			"kafka_last_tcp_seq_per_connection": {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			"http2_in_flight": {
				Type:       ebpf.LRUHash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			connectionStatesMap: {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
			probes.ConnectionProtocolMap: {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: getTracepointFuncName(enterTracepoint, openatSysCall),
					UID:          probeUID,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: getTracepointFuncName(exitTracepoint, openatSysCall),
					UID:          probeUID,
				},
			},
		},
	}

	if includeOpenat2 {
		for _, probe := range openat2Probes {
			mgr.Probes = append(mgr.Probes, &manager.Probe{
				ProbeIdentificationPair: probe,
				KProbeMaxActive:         maxActive,
			})

			options.ActivatedProbes = append(options.ActivatedProbes, &manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: probe.EBPFFuncName,
					UID:          probeUID,
				},
			})
		}
	}

	exclude := []string{
		"socket__http_filter",
		"socket__http2_filter",
		"socket__kafka_filter",
		"socket__protocol_dispatcher",
		"socket__protocol_dispatcher_kafka",
		"kprobe__tcp_sendmsg",
		"kretprobe__security_sock_rcv_skb",
		"tracepoint__net__netif_receive_skb",
		"kprobe__do_vfs_ioctl",
		"kprobe_handle_sync_payload",
		"kprobe_handle_close_connection",
		"kprobe_handle_connection_by_peer",
		"kprobe_handle_async_payload",
	}

	if !includeOpenat2 {
		exclude = append(exclude, getTracepointFuncName(enterTracepoint, openat2SysCall),
			getTracepointFuncName(exitTracepoint, openat2SysCall))
	}

	for _, sslProbeList := range [][]manager.ProbesSelector{openSSLProbes, cryptoProbes, gnuTLSProbes} {
		for _, singleProbe := range sslProbeList {
			for _, identifier := range singleProbe.GetProbesIdentificationPairList() {
				options.ExcludedFunctions = append(options.ExcludedFunctions, identifier.EBPFFuncName)
			}
		}
	}
	for _, probeInfo := range functionToProbes {
		if probeInfo.functionInfo != nil {
			options.ExcludedFunctions = append(options.ExcludedFunctions, probeInfo.functionInfo.ebpfFunctionName)
		}
		if probeInfo.returnInfo != nil {
			options.ExcludedFunctions = append(options.ExcludedFunctions, probeInfo.returnInfo.ebpfFunctionName)
		}

	}
	options.ExcludedFunctions = append(options.ExcludedFunctions, exclude...)

	mgr.InstructionPatcher = func(m *manager.Manager) error {
		return errtelemetry.PatchEBPFTelemetry(m, false, nil)
	}

	bc, err := netebpf.ReadHTTPModule(c.BPFDir, c.BPFDebug)
	require.NoError(t, err)
	err = mgr.InitWithOptions(bc, options)
	require.NoError(t, err)
	err = mgr.Start()
	require.NoError(t, err)

	t.Cleanup(func() {
		mgr.Stop(manager.CleanAll)
		perfHandler.Stop()
	})

	return perfHandler
}
