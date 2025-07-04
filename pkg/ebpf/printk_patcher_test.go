// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

func TestPatchPrintkNewline(t *testing.T) {
	kernelVersion, err := ebpfkernel.NewKernelVersion()
	require.NoError(t, err)

	// Check that tracing is on, if it's off we might try to enable it
	tracingOn, err := os.ReadFile("/sys/kernel/debug/tracing/tracing_on")
	if err == nil && strings.TrimSpace(string(tracingOn)) != "1" { // Try to continue with the tests even if we cannot read the tracing file
		if err := os.WriteFile("/sys/kernel/debug/tracing/tracing_on", []byte("1"), 0); err != nil {
			t.Skipf("Cannot enable tracing: %s", err)
		}
	}

	if kernelVersion.Code <= ebpfkernel.Kernel5_9 {
		t.Skip("Skipping test on older kernels, instruction patching not used there")
	}

	cfg := NewConfig()
	require.NotNil(t, cfg)

	idPair := manager.ProbeIdentificationPair{
		EBPFFuncName: "logdebugtest",
		UID:          "logdebugtest",
	}
	mgr := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: idPair,
				XDPAttachMode:           manager.XdpAttachModeSkb,
				IfIndex:                 1,
			},
		},
	}
	mgr.InstructionPatchers = append(mgr.InstructionPatchers, patchPrintkNewline)

	err = LoadCOREAsset("logdebug-test.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		opts.RemoveRlimit = true
		opts.MapEditors = make(map[string]*ebpf.Map)

		require.NoError(t, mgr.InitWithOptions(buf, opts))
		require.NoError(t, mgr.Start())
		t.Cleanup(func() { mgr.Stop(manager.CleanAll) })

		return nil
	})
	require.NoError(t, err)

	tp, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
	require.NoError(t, err)
	traceReader := bufio.NewReader(tp)
	t.Cleanup(func() { _ = tp.Close() })

	progs, ok, err := mgr.GetProgram(idPair)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, progs)
	require.NotEmpty(t, progs)

	// The logdebugtest program is a kprobe on do_vfs_ioctl, so we can use that to trigger the
	// it and check that the output is correct. We do not actually care about the arguments.
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(0), 0xfafafefe, uintptr(0)); errno != 0 {
		// Only valid return value is ENOTTY (invalid ioctl for device) because indeed we
		// are not doing any valid ioctl, we just want to trigger the kprobe
		require.Equal(t, syscall.ENOTTY, errno)
	}

	// The logdebug-test.c program outputs several lines
	// Check that those two are the lines included in the trace_pipe output, with no
	// additional lines or empty lines. We check with "contains" to avoid issues with
	// the variable output (PID, time, etc)
	expectedLines := []string{
		"hi", "123456", "1234567", "12345678", "Goodbye, world!", "even more words a lot of words here should be several instructions",
		"12", "21", "with args: 2+2=4", "with more args and vars: 1+2=3", "with a function call in the argument: 70", "bye",
	}

	foundLines := []string{}

	for i, line := range expectedLines {
		// We allow up to two lines that don't match, to avoid failures with other tests outputting to
		// the trace pipe.
		var actualLine string
		// Only read one line, don't allow failures as our output should be coming all together.
		// If we ignored failing lines, we would miss the case where our patcher doesn't work and
		// outputs an extra newline.
		maxLinesToRead := 1
		if i == 0 {
			maxLinesToRead = 1000 // Except for the first line, we might need to flush previous trace pipe output
		}
		for readLines := 0; readLines < maxLinesToRead; readLines++ {
			actualLine, err = traceReader.ReadString('\n')
			require.NoError(t, err)
			foundLines = append(foundLines, actualLine)

			if strings.Contains(actualLine, line) {
				break
			}
		}
		require.Contains(t, actualLine, line, "line %s not found in output, all lines until now:\n%s", line, strings.Join(foundLines, ""))
	}
}

func TestPatchPrintkAllAssets(t *testing.T) {
	cfg := NewConfig()
	require.NotNil(t, cfg)
	totalPatches := 0

	err := filepath.WalkDir(cfg.BPFDir, func(path string, _ os.DirEntry, err error) error {
		require.NoError(t, err)

		if !strings.HasSuffix(path, "-debug.o") {
			return nil // Ignore non-debug assets
		}

		progname := strings.TrimSuffix(filepath.Base(path), "-debug.o")

		if progname == "dyninst_event" {
			// dyninst doesn't use the shared debugging infrastructure yet.
			return nil
		}

		t.Run(progname, func(t *testing.T) {
			spec, err := ebpf.LoadCollectionSpec(path)
			require.NoError(t, err)

			for _, prog := range spec.Programs {
				t.Run(prog.Name, func(t *testing.T) {
					patches, err := patchPrintkInstructions(prog)
					require.NoError(t, err)
					totalPatches += patches
				})
			}

		})

		return nil
	})
	require.NoError(t, err)

	// Some programs might not have log_debug calls, but at least one should
	require.NotZero(t, totalPatches)
}
