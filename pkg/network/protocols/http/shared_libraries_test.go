// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
)

func registerProcessTerminationUponCleanup(t *testing.T, cmd *exec.Cmd) {
	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
	})
}

func TestSharedLibraryDetection(t *testing.T) {
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "foo.so")

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
			re:         regexp.MustCompile(`foo.so`),
			registerCB: callback,
		},
	)
	watcher.Start()

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
}

func TestSharedLibraryDetectionWithPIDAndRootNameSpace(t *testing.T) {
	_, err := os.Stat("/usr/bin/busybox")
	if err != nil {
		t.Skip("skip for the moment as some distro are not friendly with busybox package")
	}

	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "root")
	err = os.MkdirAll(root, 0755)
	require.NoError(t, err)

	libpath := "/fooroot.so"

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
			re:         regexp.MustCompile(`fooroot.so`),
			registerCB: callback,
		},
	)
	watcher.Start()

	time.Sleep(10 * time.Millisecond)
	// simulate a slow (1 second) : open, write, close of the file
	// in a new pid and mount namespaces
	o, err := exec.Command("unshare", "--fork", "--pid", "-R", root, "/ash", "-c", fmt.Sprintf("sleep 1 > %s", libpath)).CombinedOutput()
	if err != nil {
		t.Log(err, string(o))
	}
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// assert that soWatcher detected foo.so being opened and triggered the callback
	require.Equal(t, libpath, pathDetected)

	// must fail on the host
	_, err = os.Stat(libpath)
	require.Error(t, err)
}

func TestSameInodeRegression(t *testing.T) {
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "a-foo.so")
	fooPath2 := filepath.Join(t.TempDir(), "b-foo.so")

	// create a hard-link (a-foo.so and b-foo.so will share the same inode)
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
			re:         regexp.MustCompile(`foo.so`),
			registerCB: callback,
		},
	)
	watcher.Start()

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
}

func TestSoWatcherLeaks(t *testing.T) {
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "foo.so")
	fooPath2, fooPathID2 := createTempTestFile(t, "foo2.so")

	registerCB := func(id pathIdentifier, root string, path string) error { return nil }
	unregisterCB := func(id pathIdentifier) error { return nil }

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:           regexp.MustCompile(`foo.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
		soRule{
			re:           regexp.MustCompile(`foo2.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
	)
	watcher.Start()

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
}

func TestSoWatcherProcessAlreadyHoldingReferences(t *testing.T) {
	perfHandler := initEBPFProgram(t)

	fooPath1, fooPathID1 := createTempTestFile(t, "foo.so")
	fooPath2, fooPathID2 := createTempTestFile(t, "foo2.so")

	registerCB := func(id pathIdentifier, root string, path string) error { return nil }
	unregisterCB := func(id pathIdentifier) error { return nil }

	watcher := newSOWatcher(perfHandler,
		soRule{
			re:           regexp.MustCompile(`foo.so`),
			registerCB:   registerCB,
			unregisterCB: unregisterCB,
		},
		soRule{
			re:           regexp.MustCompile(`foo2.so`),
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

func initEBPFProgram(t *testing.T) *ddebpf.PerfHandler {
	c := config.New()
	if !HTTPSSupported(c) {
		t.Skip("https not supported for this setup")
	}

	probe := "do_sys_open"
	excludeSysOpen := "do_sys_openat2"
	if sysOpenAt2Supported(c) {
		probe = "do_sys_openat2"
		excludeSysOpen = "do_sys_open"
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
					EBPFFuncName: "kprobe__" + probe,
					UID:          probeUID,
				},
				KProbeMaxActive: maxActive,
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kretprobe__" + probe,
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
			dispatcherConnectionProtocolMap: {
				Type:       ebpf.Hash,
				MaxEntries: 1,
				EditorFlag: manager.EditMaxEntries,
			},
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kprobe__" + probe,
					UID:          probeUID,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "kretprobe__" + probe,
					UID:          probeUID,
				},
			},
		},
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
		"kprobe__" + excludeSysOpen,
		"kretprobe__" + excludeSysOpen,
		"kprobe__do_vfs_ioctl",
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
