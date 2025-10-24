// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package seccomptracer

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/seccomptracer/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	procutil "github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type seccompTracerTestSuite struct {
	suite.Suite
}

func TestSeccompTracer(t *testing.T) {
	// Seccomp tracer only supports CO-RE (uses bpf_task_pt_regs and bpf_get_current_task_btf)
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.CORE}, "", func(t *testing.T) {
		cfg := ebpf.NewConfig()
		isSupported, err := IsSupported(cfg)
		require.NoError(t, err)
		if !isSupported {
			t.Skip("seccomp tracer is not supported on this configuration")
		}
		suite.Run(t, new(seccompTracerTestSuite))
	})
}

func (s *seccompTracerTestSuite) TestCanLoad() {
	t := s.T()

	cfg := NewConfig()
	tracer, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tracer)
	t.Cleanup(func() { tracer.Close() })

	// Test that GetAndFlush works
	stats := tracer.GetAndFlush()
	require.NotNil(t, stats)
}

func (s *seccompTracerTestSuite) TestCanDetectSeccompDenial() {
	t := s.T()

	cfg := NewConfig()
	tracer, err := NewTracer(cfg)
	require.NoError(t, err)
	require.NotNil(t, tracer)
	t.Cleanup(func() { tracer.Close() })

	// Run the seccomp sample
	cmd, err := runSeccompSample(t, 2) // 2 second wait time
	require.NoError(t, err)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	// Wait for denials to be captured
	var stats model.SeccompStats
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		stats = tracer.GetAndFlush()
		foundGetpid := false
		foundGetuid := false

		for _, value := range stats {
			t.Logf("Captured seccomp denial: syscall=%d (0x%x), action=0x%08x, count=%d, cgroup=%s, pid=%d, comm=%s, stacks=%d",
				value.SyscallNr, value.SyscallNr, value.SeccompAction, value.Count, value.CgroupName,
				value.Pid, value.Comm, len(value.StackTraces))

			if value.SyscallNr == unix.SYS_GETPID {
				foundGetpid = true
				assert.Equal(c, uint64(1), value.Count, "Expected exactly one getpid denial")
				assert.Equal(c, uint32(unix.SECCOMP_RET_ERRNO), value.SeccompAction)
			}
			if value.SyscallNr == unix.SYS_GETUID {
				foundGetuid = true
				assert.Equal(c, uint64(1), value.Count, "Expected exactly one getuid denial")
				assert.Equal(c, uint32(unix.SECCOMP_RET_ERRNO), value.SeccompAction)
			}
		}

		assert.True(c, foundGetpid, "Expected to capture SYS_GETPID (%d) denial from seccompsample", unix.SYS_GETPID)
		assert.True(c, foundGetuid, "Expected to capture SYS_GETUID (%d) denial from seccompsample", unix.SYS_GETUID)
	}, 10*time.Second, 100*time.Millisecond, "Expected to capture seccomp denials")

	// Verify stack traces are captured
	require.NotEmpty(t, stats, "Expected to capture seccomp denials")

	foundStackTrace := false
	for _, entry := range stats {
		// Verify PID and comm are captured
		assert.Greater(t, entry.Pid, uint32(0), "PID should be captured")
		assert.NotEmpty(t, entry.Comm, "Command name should be captured")
		t.Logf("Entry: syscall=%d, pid=%d, comm=%s, stacks=%d", entry.SyscallNr, entry.Pid, entry.Comm, len(entry.StackTraces))

		if len(entry.StackTraces) > 0 {
			foundStackTrace = true

			// Verify stack trace has data
			for _, trace := range entry.StackTraces {
				assert.GreaterOrEqual(t, trace.StackID, int32(0), "Stack ID should be valid (>= 0)")
				assert.Greater(t, trace.Count, uint64(0), "Stack trace count should be > 0")
				assert.NotEmpty(t, trace.Addresses, "Stack trace should have addresses")

				// Verify addresses are non-zero
				hasNonZeroAddr := false
				for _, addr := range trace.Addresses {
					if addr != 0 {
						hasNonZeroAddr = true
						break
					}
				}
				assert.True(t, hasNonZeroAddr, "Stack trace should have at least one non-zero address")

				// Verify symbolication was applied
				assert.NotEmpty(t, trace.Symbols, "Stack trace should have symbols")
				assert.Equal(t, len(trace.Addresses), len(trace.Symbols), "Should have one symbol per address")

				// Log the symbolicated stack trace
				t.Logf("Stack trace: stackID=%d, count=%d, frames=%d, first_addr=0x%x",
					trace.StackID, trace.Count, len(trace.Addresses), trace.Addresses[0])
				for i, symbol := range trace.Symbols {
					t.Logf("  Frame %d: 0x%x -> %s", i, trace.Addresses[i], symbol)
				}
			}
		}
	}

	assert.True(t, foundStackTrace, "Expected at least one entry with stack traces")
}

func innerTestStackTraceSymbolication(t *testing.T, mode SymbolicationMode) {
	cfg := Config{
		Config:            *ebpf.NewConfig(),
		SymbolicationMode: mode,
	}
	tracer, err := NewTracer(&cfg)
	require.NoError(t, err)
	require.NotNil(t, tracer)
	t.Cleanup(func() { tracer.Close() })

	// Run the seccomp sample
	cmd, err := runSeccompSample(t, 2) // 2 second wait time
	require.NoError(t, err)
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	// Wait for the getpid denial
	var denialEntry *model.SeccompStatsEntry
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		stats := tracer.GetAndFlush()
		for _, value := range stats {
			if value.SyscallNr == unix.SYS_GETPID {
				assert.Equal(c, uint64(1), value.Count, "Expected exactly one getpid denial")
				assert.Equal(c, uint32(unix.SECCOMP_RET_ERRNO), value.SeccompAction)
				denialEntry = &value
				break
			}
		}

		assert.NotNil(c, denialEntry, "Expected to capture getpid denial")
	}, 10*time.Second, 100*time.Millisecond, "Expected to capture getpid denial")

	require.Len(t, denialEntry.StackTraces, 1, "Expected exactly one stack trace")

	foundTrigger := false
	for _, line := range denialEntry.StackTraces[0].Symbols {
		t.Logf("Symbol: %s (mode: %v)", line, mode)
		if strings.Contains(line, "trigger_getpid_level1") {
			foundTrigger = true

			if mode == SymbolicationModeDWARF {
				require.Regexp(t, `^seccompsample!trigger_getpid_level\d+\(\)$`, line)
			} else if mode == SymbolicationModeSymTable {
				// Symbol table mode may include offsets (e.g., +0xc) for return addresses
				require.Regexp(t, `^seccompsample!trigger_getpid_level\d+(\+0x[0-9a-f]+)?$`, line)
			} else {
				require.Regexp(t, `^seccompsample\+0x[0-9a-f]+$`, line)
			}
		}
	}

	if mode == SymbolicationModeDWARF || mode == SymbolicationModeSymTable {
		assert.True(t, foundTrigger, "Expected to find trigger_getpid_level1 in stack trace")
	} else {
		assert.False(t, foundTrigger, "Expected to not find trigger_getpid_level1 in stack trace")
	}

}

func (s *seccompTracerTestSuite) TestStackTraces() {
	t := s.T()

	cases := []struct {
		name string
		mode SymbolicationMode
	}{
		{name: "RawOnly", mode: SymbolicationModeRawAddresses},
		{name: "SymTableOnly", mode: SymbolicationModeSymTable},
		{name: "DWARF", mode: SymbolicationModeDWARF},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			innerTestStackTraceSymbolication(t, c.mode)
		})
	}
}

// runSeccompSample runs the seccomp sample binary
func runSeccompSample(t *testing.T, waitTime int) (*exec.Cmd, error) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "../../testdata/seccompsample.c")
	binaryFile := filepath.Join(curDir, "../../testdata/seccompsample")

	// Build the sample binary with debug info for better symbolication
	buildCmd := exec.Command("gcc", "-static", "-g", "-o", binaryFile, sourceFile, "-lseccomp", "-ggdb")
	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to compile seccompsample: %s", string(output))

	// Create scanner for output patterns
	startPattern := regexp.MustCompile("Starting SeccompSample program")
	finishedPattern := regexp.MustCompile("Seccomp denials triggered")
	scanner, err := procutil.NewScanner(startPattern, finishedPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, binaryFile, fmt.Sprintf("%d", waitTime))
	cmd.Stdout = scanner
	cmd.Stderr = scanner

	err = cmd.Start()
	if err != nil {
		scanner.PrintLogs(t)
		return nil, err
	}

	// Wait for the program to start and finish
	select {
	case <-ctx.Done():
		scanner.PrintLogs(t)
		return nil, ctx.Err()
	case <-scanner.DoneChan:
		return cmd, nil
	case <-time.After(15 * time.Second):
		scanner.PrintLogs(t)
		return nil, context.DeadlineExceeded
	}
}
