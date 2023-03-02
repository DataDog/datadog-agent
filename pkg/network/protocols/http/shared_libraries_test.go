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
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
)

func TestSharedLibraryDetection(t *testing.T) {
	perfHandler, doneFn := initEBPFProgram(t)
	t.Cleanup(doneFn)
	fpath := filepath.Join(t.TempDir(), "foo.so")

	// touch the file, as the file must exist when the process Exec
	// for the file identifier stat()
	f, err := os.Create(fpath)
	require.NoError(t, err)
	f.Close()

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

	time.Sleep(10 * time.Millisecond)
	simulateOpenAt(fpath)
	time.Sleep(10 * time.Millisecond)

	// assert that soWatcher detected foo.so being opened and triggered the callback
	require.Equal(t, fpath, pathDetected)
}

func TestSharedLibraryDetectionWithPIDandRootNameSpace(t *testing.T) {
	_, err := os.Stat("/usr/bin/busybox")
	if err != nil {
		t.Skip("skip for the moment as some distro are not friendly with busybox package")
	}

	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "root")
	err = os.MkdirAll(root, 0755)
	require.NoError(t, err)

	libpath := "/fooroot.so"

	simulateOpenAt(root + libpath)
	err = exec.Command("cp", "/usr/bin/busybox", root+"/ash").Run()
	require.NoError(t, err)
	err = exec.Command("cp", "/usr/bin/busybox", root+"/sleep").Run()
	require.NoError(t, err)

	perfHandler, doneFn := initEBPFProgram(t)
	t.Cleanup(doneFn)

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
	// in an new pid and mount namespaces
	o, err := exec.Command("unshare", "--fork", "--pid", "-R", root, "/ash", "-c", fmt.Sprintf("sleep 1 > %s", libpath)).CombinedOutput()
	if err != nil {
		t.Log(err, string(o))
	}
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// assert that soWatcher detected foo.so being opened and triggered the callback
	require.Equal(t, libpath, pathDetected)

	// must failed on the host
	_, err = os.Stat(libpath)
	require.Error(t, err)
}

func TestSameInodeRegression(t *testing.T) {
	perfHandler, doneFn := initEBPFProgram(t)
	t.Cleanup(doneFn)
	fpath1 := filepath.Join(t.TempDir(), "a-foo.so")
	fpath2 := filepath.Join(t.TempDir(), "b-foo.so")

	f, err := os.Create(fpath1)
	require.NoError(t, err)
	f.Close()

	// create a hard-link (a-foo.so and b-foo.so will share the same inode)
	err = os.Link(fpath1, fpath2)
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

	time.Sleep(10 * time.Millisecond)
	simulateOpenAt(fpath1)
	simulateOpenAt(fpath2)
	time.Sleep(10 * time.Millisecond)

	// assert that callback was called only once
	require.Equal(t, int64(1), registers.Load())
}

// we use this helper to open files for two reasons:
// * `touch` calls openat(2) which is what we trace in the shared library eBPF program;
// * `exec.Command` spawns a separate process; we need to do that because we filter out
// libraries being openened from system-probe process
func simulateOpenAt(path string) {
	cmd := exec.Command("touch", path)
	cmd.Run()
}

func initEBPFProgram(t *testing.T) (*ddebpf.PerfHandler, func()) {
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

	return perfHandler, func() {
		mgr.Stop(manager.CleanAll)
		perfHandler.Stop()
	}
}
