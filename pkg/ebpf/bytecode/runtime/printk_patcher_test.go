// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)

//go:generate $GOPATH/bin/include_headers pkg/ebpf/testdata/c/runtime/logdebug-test.c pkg/ebpf/bytecode/build/runtime/logdebug-test.c pkg/ebpf/c pkg/network/ebpf/c/runtime pkg/network/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/logdebug-test.c pkg/ebpf/bytecode/runtime/logdebug-test.go runtime

func TestPatchPrintkNewline(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		cfg := ddebpf.NewConfig()
		require.NotNil(t, cfg)

		buf, err := LogdebugTest.Compile(cfg, []string{"-g", "-DDEBUG=1"}, nil)
		require.NoError(t, err)
		defer buf.Close()

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

		opts := manager.Options{
			RLimit: &unix.Rlimit{
				Cur: math.MaxUint64,
				Max: math.MaxUint64,
			},
			MapEditors: make(map[string]*ebpf.Map),
		}
		mgr.InstructionPatcher = PatchPrintkNewline

		tp, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
		require.NoError(t, err)
		traceReader := bufio.NewReader(tp)
		t.Cleanup(func() { _ = tp.Close() })

		require.NoError(t, mgr.InitWithOptions(buf, opts))
		require.NoError(t, mgr.Start())
		t.Cleanup(func() { mgr.Stop(manager.CleanAll) })

		progs, ok, err := mgr.GetProgram(idPair)
		require.NoError(t, err)
		require.True(t, ok)
		require.NotNil(t, progs)
		require.NotEmpty(t, progs)

		// The logdebugtest program is a kprobe on do_vfs_ioctl, so we can use that to trigger the
		// it and check that the output is correct. We do not actually care about the arguments.
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(0), 0, uintptr(0)); errno != 0 {
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

		for _, line := range expectedLines {
			actualLine, err := traceReader.ReadString('\n')
			require.NoError(t, err)
			require.Contains(t, actualLine, line)
		}
	})
}

func TestPatchPrintkAllAssets(t *testing.T) {
	cfg := ddebpf.NewConfig()
	require.NotNil(t, cfg)
	totalPatches := 0

	err := filepath.WalkDir(cfg.BPFDir, func(path string, d os.DirEntry, err error) error {
		require.NoError(t, err)

		if !strings.HasSuffix(path, "-debug.o") {
			return nil // Ignore non-debug assets
		}

		progname := strings.TrimSuffix(filepath.Base(path), "-debug.o")

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
