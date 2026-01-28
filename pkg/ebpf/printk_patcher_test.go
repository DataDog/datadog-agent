// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestPatchPrintkNewline(t *testing.T) {
	kernelVersion, err := ebpfkernel.NewKernelVersion()
	require.NoError(t, err)

	tracefsRoot, err := tracefs.Root()
	require.NoError(t, err)
	// Check that tracing is on, if it's off we might try to enable it
	tracingOnPath := tracefsRoot + "/tracing_on"
	tracingOn, err := os.ReadFile(tracingOnPath)
	if err == nil && strings.TrimSpace(string(tracingOn)) != "1" { // Try to continue with the tests even if we cannot read the tracing file
		if err := os.WriteFile(tracingOnPath, []byte("1"), 0); err != nil {
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

	tp, err := ebpftest.NewTracePipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tp.Close() })

	progs, ok, err := mgr.GetProgram(idPair)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, progs)
	require.NotEmpty(t, progs)

	require.NoError(t, tp.Clear())

	// The logdebugtest program is a kprobe on do_vfs_ioctl, so we can use that to trigger
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

	var foundLines []string

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
			event, err := tp.ReadLine()
			require.NoError(t, err)

			if event != nil {
				actualLine = event.Message
				foundLines = append(foundLines, event.Raw)
				if strings.Contains(actualLine, line) {
					break
				}
			}
		}
		require.Contains(t, actualLine, line, "line %s not found in output, all lines until now:\n%s", line, strings.Join(foundLines, ""))
	}
}

func TestPatchPrintkManualAssembly(t *testing.T) {
	type testCase struct {
		name                string
		instructions        asm.Instructions
		expectedImms        map[int]int64
		expectedJumpSources map[int][]int
		patches             int
	}

	testCases := []testCase{
		{
			name: "base-case",
			instructions: asm.Instructions{
				asm.LoadImm(asm.R3, 0x00000000000a6261, asm.DWord),
				asm.StoreMem(asm.RFP, -16, asm.R3, asm.DWord),
				asm.Mov.Reg(asm.R1, asm.RFP),
				asm.Add.Imm(asm.R1, -16),
				asm.Mov.Imm(asm.R2, 4),
				asm.FnTracePrintk.Call(),
				asm.Return(),
			},
			expectedImms: map[int]int64{
				0: int64(0x0000000000006261),
				4: 3,
			},
			expectedJumpSources: map[int][]int{},
			patches:             1,
		},
		{
			name: "branch-to-shared-call",
			instructions: asm.Instructions{
				asm.Mov.Imm(asm.R3, 0x00000000000a333231),                    // off 0
				asm.StoreMem(asm.RFP, -32, asm.R3, asm.DWord),                // off 1
				asm.Mov.Reg(asm.R1, asm.RFP),                                 // off 2
				asm.Add.Imm(asm.R1, -32),                                     // off 3
				asm.Mov.Imm(asm.R2, 5),                                       // off 4
				asm.Instruction{OpCode: asm.Ja.Op(asm.ImmSource), Offset: 5}, // off 5 -> 11
				asm.Mov.Imm(asm.R4, 0x00000000000a6261),                      // off 6
				asm.StoreMem(asm.RFP, -16, asm.R4, asm.DWord),                // off 7
				asm.Mov.Reg(asm.R1, asm.RFP),                                 // off 8
				asm.Add.Imm(asm.R1, -16),                                     // off 9
				asm.Mov.Imm(asm.R2, 4),                                       // off 10
				asm.FnTracePrintk.Call(),                                     // off 11
				asm.Return(),                                                 // off 12
			},
			expectedImms: map[int]int64{
				0:  int64(0x000000000000333231),
				4:  4,
				6:  int64(0x0000000000006261),
				10: 3,
			},
			expectedJumpSources: map[int][]int{
				11: {5},
			},
			patches: 2,
		},
		{
			name: "jump-over-double-wide",
			instructions: asm.Instructions{
				asm.LoadImm(asm.R3, 0x00000000000a333231, asm.DWord),         // off 0-1 (double-wide)
				asm.StoreMem(asm.RFP, -32, asm.R3, asm.DWord),                // off 2
				asm.Mov.Reg(asm.R1, asm.RFP),                                 // off 3
				asm.Add.Imm(asm.R1, -32),                                     // off 4
				asm.Mov.Imm(asm.R2, 5),                                       // off 5
				asm.Instruction{OpCode: asm.Ja.Op(asm.ImmSource), Offset: 9}, // off 6 -> 15, indexes 5 -> 13
				asm.LoadImm(asm.R7, 0x1122334455667788, asm.DWord),           // off 7-8 (double-wide filler)
				asm.Mov.Imm(asm.R0, 0),                                       // off 9
				asm.LoadImm(asm.R4, 0x00000000000a6261, asm.DWord),           // off 10-11 (double-wide)
				asm.StoreMem(asm.RFP, -16, asm.R4, asm.DWord),                // off 12
				asm.Mov.Reg(asm.R1, asm.RFP),                                 // off 13
				asm.Add.Imm(asm.R1, -16),                                     // off 14
				asm.Mov.Imm(asm.R2, 4),                                       // off 15
				asm.FnTracePrintk.Call(),                                     // off 16
				asm.Return(),                                                 // off 17
			},
			expectedImms: map[int]int64{
				0:  int64(0x000000000000333231),
				4:  4,
				8:  int64(0x0000000000006261),
				12: 3,
			},
			expectedJumpSources: map[int][]int{
				13: {5}, // the jump map is populated with the index of the instruction, not the offset, so doesn't take into account the double-wide instruction
			},
			patches: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			program := &ebpf.ProgramSpec{
				Name:         tc.name,
				Type:         ebpf.Kprobe,
				Instructions: tc.instructions,
				License:      "GPL",
			}

			patcher := newPrintkPatcher(program)
			patches, err := patcher.patch()

			log.Flush() // Ensure we flush logs before assertions to avoid messing the output
			require.NoError(t, err)
			assert.Equal(t, tc.patches, patches, "number of patches does not match expected ones")

			assert.ElementsMatch(t, slices.Collect(maps.Keys(tc.expectedJumpSources)), slices.Collect(maps.Keys(patcher.jumpSources)), "jump sources keys do not match expected ones")
			for target, sources := range tc.expectedJumpSources {
				assert.ElementsMatch(t, sources, patcher.jumpSources[target], "detected jump sources for target %d do not match expected ones", target)
			}

			for idx, expected := range tc.expectedImms {
				require.Equal(t, expected, tc.instructions[idx].Constant, "immediate for instruction %d does not match expected one", idx)
			}
		})
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
			log.Tracef("Testing program %s, path: %s", progname, path)
			spec, err := ebpf.LoadCollectionSpec(path)
			require.NoError(t, err)

			for _, prog := range spec.Programs {
				t.Run(prog.Name, func(t *testing.T) {
					patcher := newPrintkPatcher(prog)
					patches, err := patcher.patch()
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
