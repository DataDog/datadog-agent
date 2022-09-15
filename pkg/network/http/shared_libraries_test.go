// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSharedLibraryDetection(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	if !httpsSupported() {
		t.Skip("shared library detection not supported. skipping.")
	}

	perfHandler, doneFn := initEBPFProgram(t)
	fpath := filepath.Join(os.TempDir(), "foo.so")
	t.Cleanup(func() {
		os.Remove(fpath)
		doneFn()
	})

	var (
		mux          sync.Mutex
		pathDetected string
	)

	callback := func(path string) error {
		mux.Lock()
		defer mux.Unlock()
		pathDetected = path
		return nil
	}

	watcher := newSOWatcher("/proc", perfHandler,
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
	assert.Equal(t, fpath, pathDetected)
}

func TestSameInodeRegression(t *testing.T) {
	skipTestIfKernelNotSupported(t)
	if !httpsSupported() {
		t.Skip("shared library detection not supported. skipping.")
	}

	perfHandler, doneFn := initEBPFProgram(t)
	fpath1 := filepath.Join(os.TempDir(), "a-foo.so")
	fpath2 := filepath.Join(os.TempDir(), "b-foo.so")
	t.Cleanup(func() {
		os.Remove(fpath1)
		os.Remove(fpath2)
		doneFn()
	})

	f, err := os.Create(fpath1)
	require.NoError(t, err)
	f.Close()

	// create a hard-link (a-foo.so and b-foo.so will share the same inode)
	err = os.Link(fpath1, fpath2)
	require.NoError(t, err)

	registers := atomic.NewInt64(0)
	callback := func(string) error {
		registers.Add(1)
		return nil
	}

	watcher := newSOWatcher("/proc", perfHandler,
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
	assert.Equal(t, int64(1), registers.Load())
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
	c.EnableHTTPSMonitoring = true

	probe := "do_sys_open"
	if p, err := newSSLProgram(c, nil); err != nil {
		if p.sysOpenAt2Supported() {
			probe = "do_sys_openat2"
		}
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
					EBPFSection:  "kprobe/" + probe,
					EBPFFuncName: "kprobe__" + probe,
					UID:          probeUID,
				},
				KProbeMaxActive: maxActive,
			},
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "kretprobe/" + probe,
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
		},
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "kprobe/" + probe,
					EBPFFuncName: "kprobe__" + probe,
					UID:          probeUID,
				},
			},
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFSection:  "kretprobe/" + probe,
					EBPFFuncName: "kretprobe__" + probe,
					UID:          probeUID,
				},
			},
		},
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
