// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux

package module

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httptestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	longComm = "this_is_command_name_longer_than_sixteen_bytes"
)

// TestIgnoreComm checks that the 'sshd' command is ignored and the 'node' command is not
func TestIgnoreComm(t *testing.T) {
	serverDir := buildFakeServer(t)
	url := setupDiscoveryModule(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	badBin := filepath.Join(serverDir, "sshd")
	badCmd := exec.CommandContext(ctx, badBin)
	require.NoError(t, badCmd.Start())

	// Also run a non-ignored server so that we can use it in the eventually
	// loop below so that we don't have to wait a long time to be sure that we
	// really ignored badBin and just didn't miss it because of a race.
	goodBin := filepath.Join(serverDir, "node")
	goodCmd := exec.CommandContext(ctx, goodBin)
	require.NoError(t, goodCmd.Start())

	goodPid := goodCmd.Process.Pid
	badPid := badCmd.Process.Pid

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		svcMap := getServicesMap(t, url)
		assert.Contains(collect, svcMap, goodPid)
		require.NotContains(t, svcMap, badPid)
	}, 30*time.Second, 100*time.Millisecond)
}

// buildLongProc returns the path of a symbolic link to a binary file
func buildLongProc(t *testing.T) string {
	curDir, err := httptestutil.CurDir()
	require.NoError(t, err)

	serverBin, err := testutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "fake_server")
	require.NoError(t, err)

	makeAlias(t, longComm, serverBin)

	return filepath.Join(filepath.Dir(serverBin), longComm)
}

// TestLongComm checks that the long command name of process is fetched completely.
func TestLongComm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	bin := buildLongProc(t)
	cmd := exec.CommandContext(ctx, bin)
	require.NoError(t, cmd.Start())

	proc, err := process.NewProcess(int32(cmd.Process.Pid))
	require.NoError(t, err)

	comm, err := proc.Name()
	require.NoError(t, err)
	require.Equal(t, len(comm), len(longComm))
	require.False(t, ignoreComm(proc))
}

func makeSymLink(b *testing.B, src string, dst string) {
	target, err := os.Readlink(dst)
	if err == nil && target == dst {
		return
	}

	err = os.Symlink(src, dst)
	if err != nil {
		b.Fatal(err)
	}
}

func startBenchLongCommProcess(b *testing.B) *process.Process {
	b.Helper()

	dstPath := filepath.Join(b.TempDir(), longComm)
	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(func() {
		cancel()
		os.Remove(dstPath)
	})
	makeSymLink(b, "/bin/sleep", dstPath)

	cmd := exec.CommandContext(ctx, dstPath, "49")
	err := cmd.Start()
	if err != nil {
		b.Fatal(err)
	}
	proc, err := customNewProcess(int32(cmd.Process.Pid))
	if err != nil {
		b.Fatal(err)
	}
	return proc
}

// BenchmarkProcName benchmarks reading of entire command name from /proc.
func BenchmarkProcName(b *testing.B) {
	proc := startBenchLongCommProcess(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		comm, err := proc.Name()
		if err != nil {
			b.Fatal(err)
		}
		if len(comm) != len(longComm) {
			b.Fatalf("wrong comm length, expected: %d, got: %d", len(longComm), len(comm))
		}
	}
}

// getComm reads /rpc/<pid>/comm and returns process command.
func getComm(proc *process.Process) (string, error) {
	commPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "comm")
	contents, err := os.ReadFile(commPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(string(contents), "\n"), nil
}

// BenchmarkProcComm benchmarks reading of command name from /proc/<pid>/comm.
func BenchmarkProcComm(b *testing.B) {
	proc := startBenchLongCommProcess(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		comm, err := getComm(proc)
		if err != nil {
			b.Fatal(err)
		}
		if len(comm) != 15 {
			b.Fatalf("wrong comm length, expected: 15, got: %d", len(comm))
		}
	}
}
