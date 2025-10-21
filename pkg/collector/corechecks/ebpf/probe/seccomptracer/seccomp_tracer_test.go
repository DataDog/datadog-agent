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
		suite.Run(t, new(seccompTracerTestSuite))
	})
}

func (s *seccompTracerTestSuite) TestCanLoad() {
	t := s.T()

	tracer, err := NewTracer(ebpf.NewConfig())
	require.NoError(t, err)
	require.NotNil(t, tracer)
	t.Cleanup(func() { tracer.Close() })

	// Test that GetAndFlush works
	stats := tracer.GetAndFlush()
	require.NotNil(t, stats)
}

func (s *seccompTracerTestSuite) TestCanDetectSeccompDenial() {
	t := s.T()

	tracer, err := NewTracer(ebpf.NewConfig())
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
			t.Logf("Captured seccomp denial: syscall=%d (0x%x), action=0x%08x, count=%d, cgroup=%s",
				value.SyscallNr, value.SyscallNr, value.SeccompAction, value.Count, value.CgroupName)

			if value.SyscallNr == unix.SYS_GETPID {
				foundGetpid = true
				assert.Equal(c, uint64(1), value.Count, "Expected exactly one getpid denial")
			}
			if value.SyscallNr == unix.SYS_GETUID {
				foundGetuid = true
				assert.Equal(c, uint64(1), value.Count, "Expected exactly one getuid denial")
			}
		}

		assert.True(c, foundGetpid, "Expected to capture SYS_GETPID (%d) denial from seccompsample", unix.SYS_GETPID)
		assert.True(c, foundGetuid, "Expected to capture SYS_GETUID (%d) denial from seccompsample", unix.SYS_GETUID)
	}, 10*time.Second, 100*time.Millisecond, "Expected to capture seccomp denials")
}

// runSeccompSample runs the seccomp sample binary
func runSeccompSample(t *testing.T, waitTime int) (*exec.Cmd, error) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sourceFile := filepath.Join(curDir, "../../testdata/seccompsample.c")
	binaryFile := filepath.Join(curDir, "../../testdata/seccompsample")

	// Build the sample binary
	buildCmd := exec.Command("gcc", "-static", "-o", binaryFile, sourceFile, "-lseccomp")
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
