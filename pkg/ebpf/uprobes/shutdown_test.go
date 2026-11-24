// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package uprobes

import (
	"encoding/json"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	"strings"
)

// TestUprobeShutdownScenario is an opt-in subprocess scenario, not a latency
// assertion for CI. A supervisor waits for the ready record, delivers SIGTERM or
// SIGKILL, samples kernel stacks, and measures time until this process is reaped.
// See doc/how-to/test/system-probe-shutdown/README.md.
func TestUprobeShutdownScenario(t *testing.T) {
	mode := os.Getenv("DD_UPROBE_SHUTDOWN_MODE")
	if mode == "" {
		t.Skip("opt-in shutdown scenario")
	}
	require.Contains(t, []string{"perf", "multi"}, mode)
	count, err := strconv.Atoi(os.Getenv("DD_UPROBE_SHUTDOWN_COUNT"))
	require.NoError(t, err)
	require.Greater(t, count, 0)
	require.LessOrEqual(t, count, 10000)
	target := os.Getenv("DD_UPROBE_SHUTDOWN_TARGET")
	object := os.Getenv("DD_UPROBE_SHUTDOWN_OBJECT")
	require.NotEmpty(t, target)
	require.NotEmpty(t, object)
	require.NoError(t, unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{Cur: unix.RLIM_INFINITY, Max: unix.RLIM_INFINITY}))
	require.NoError(t, unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 65536, Max: 65536}))

	// Block shutdown before attaching: the supervisor can signal immediately
	// after receiving ready, without racing signal handler installation.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)

	file, err := safeelf.Open(target)
	require.NoError(t, err)
	defer file.Close()
	symbols, err := file.Symbols()
	require.NoError(t, err)
	var base uint64
	for _, symbol := range symbols {
		if symbol.Name != "probe_targets" {
			continue
		}
		for _, segment := range file.Progs {
			if segment.Type == safeelf.PT_LOAD && symbol.Value >= segment.Vaddr && symbol.Value < segment.Vaddr+segment.Filesz {
				base = symbol.Value - segment.Vaddr + segment.Off
			}
		}
	}
	require.NotZero(t, base)
	stride := uint64(1)
	if runtime.GOARCH == "arm64" {
		stride = 4
	} else {
		require.Equal(t, "amd64", runtime.GOARCH)
	}
	offsets := make([]uint64, count)
	for i := range offsets {
		offsets[i] = base + uint64(i)*stride
	}

	mgr := &manager.Manager{}
	var managerStopOnce sync.Once
	var managerStopErr error
	stopManager := func() error {
		managerStopOnce.Do(func() { managerStopErr = mgr.Stop(manager.CleanAll) })
		return managerStopErr
	}
	var prepared *MultiProgramSpec
	reader, err := os.Open(object)
	require.NoError(t, err)
	defer reader.Close()
	if mode == "multi" {
		mgr.InstructionPatchers = append(mgr.InstructionPatchers, func(mgr *manager.Manager) error {
			var err error
			prepared, err = PrepareUprobeMultiPrograms(mgr, []string{"uprobe__main"}, nil)
			return err
		})
	}
	require.NoError(t, mgr.InitWithOptions(reader, manager.Options{KeepUnmappedProgramSpecs: true}))
	// Avoid Manager.Start's host-wide stale tracefs cleanup. AddHook attaches
	// immediately on an initialized manager; the multi path owns its links.
	t.Cleanup(func() { require.NoError(t, stopManager()) })
	var loaded *MultiPrograms
	if mode == "multi" {
		loaded, err = prepared.Load(mgr, manager.Options{}.VerifierOptions, "shutdown-scenario")
		require.NoError(t, err)
		t.Cleanup(loaded.Close)
	}

	attacher, err := NewUprobeAttacher("shutdown-scenario", "shutdown-scenario", AttacherConfig{}, mgr, nil, AttacherDependencies{ProcessMonitor: newMockProcessMonitor(), Inspector: &NativeBinaryInspector{}})
	require.NoError(t, err)
	if loaded != nil {
		attacher.UseUprobeMultiPrograms(loaded.Programs())
	}
	require.NoError(t, attacher.Start())
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(attacher.Stop) }
	t.Cleanup(stop)
	rule := &AttachRule{ProbeOptionsOverride: map[string]ProbeOptions{"uprobe__main": {Symbol: "probe_targets", IsManualReturn: true}}}
	selector := &manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__main"}}
	attach := func(path utils.FilePath) error {
		return attacher.attachProbeSelector(selector, path, getUID(path.ID), rule, map[string]bininspect.FunctionMetadata{"probe_targets": {ReturnLocations: offsets}})
	}
	require.NoError(t, attacher.fileRegistry.Register(target, uint32(os.Getpid()), attach, attacher.detachFromBinary, utils.IgnoreCB))
	perfFDs, linkFDs := 0, 0
	fds, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	for _, fd := range fds {
		target, _ := os.Readlink("/proc/self/fd/" + fd.Name())
		if strings.Contains(target, "perf_event") {
			perfFDs++
		}
		if strings.Contains(target, "bpf-link") || strings.Contains(target, "bpf_link") {
			linkFDs++
		}
	}
	if mode == "multi" {
		require.Equal(t, 0, perfFDs)
		require.Equal(t, 1, linkFDs)
	} else {
		require.Equal(t, count, perfFDs)
	}

	record := func(value any) { require.NoError(t, json.NewEncoder(os.Stdout).Encode(value)) }
	record(map[string]any{"phase": "ready", "pid": os.Getpid(), "probes": count, "mode": mode, "perf_fds": perfFDs, "link_fds": linkFDs})
	<-signals
	profileDone := make(chan struct{})
	defer close(profileDone)
	go func() {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			_ = pprof.Lookup("goroutine").WriteTo(os.Stderr, 2)
		case <-profileDone:
		}
	}()
	start := time.Now()
	stop()
	record(map[string]any{"phase": "attacher_stopped", "seconds": time.Since(start).Seconds()})
	start = time.Now()
	loaded.Close()
	require.NoError(t, stopManager())
	record(map[string]any{"phase": "manager_stopped", "seconds": time.Since(start).Seconds()})
}
