// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux_bpf

package module

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httptestutil "github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

const (
	longComm = "this-is_command-name-longer-than-sixteen-bytes"
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

// TestIgnoreCommsLengths checks that the map contains names no longer than 15 bytes.
func TestIgnoreCommsLengths(t *testing.T) {
	discovery := newDiscovery()
	require.NotEmpty(t, discovery)
	require.Equal(t, len(discovery.config.ignoreComms), 10)

	for comm := range discovery.config.ignoreComms {
		assert.LessOrEqual(t, len(comm), maxCommLen, "Process name %q too big", comm)
	}
}

// buildTestBin returns the path to a binary file
func buildTestBin(t *testing.T) string {
	curDir, err := httptestutil.CurDir()
	require.NoError(t, err)

	serverBin, err := testutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "fake_server")
	require.NoError(t, err)

	return serverBin
}

// TestShouldIgnoreComm check cases of ignored and non-ignored processes
func TestShouldIgnoreComm(t *testing.T) {
	testCases := []struct {
		name   string
		comm   string
		ignore bool
	}{
		{
			name:   "should ignore command docker-proxy",
			comm:   "docker-proxy",
			ignore: true,
		},
		{
			name:   "should ignore command containerd",
			comm:   "containerd",
			ignore: true,
		},
		{
			name:   "should ignore command local-volume-provisioner",
			comm:   "local-volume-provisioner",
			ignore: true,
		},
		{
			name:   "should not ignore command java-some",
			comm:   "java-some",
			ignore: false,
		},
		{
			name:   "should not ignore command long command",
			comm:   longComm,
			ignore: false,
		},
	}

	serverBin := buildTestBin(t)
	serverDir := filepath.Dir(serverBin)
	discovery := newDiscovery()
	require.NotEmpty(t, discovery)
	require.NotEmpty(t, discovery.config.ignoreComms)
	require.Equal(t, len(discovery.config.ignoreComms), 10)

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() { cancel() })

			makeAlias(t, test.comm, serverBin)
			t.Cleanup(func() {
				os.Remove(test.comm)
			})
			bin := filepath.Join(serverDir, test.comm)
			cmd := exec.CommandContext(ctx, bin)
			err := cmd.Start()
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
			})

			var proc *process.Process
			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				proc, err = customNewProcess(int32(cmd.Process.Pid))
				assert.NoError(collect, err)
			}, 2*time.Second, 100*time.Millisecond)

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				ignore := discovery.shouldIgnoreComm(proc)
				assert.Equal(collect, test.ignore, ignore)
			}, 500*time.Millisecond, 100*time.Millisecond)
		})
	}
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

// startProcessLongComm starts a process with a long command name, used to benchmark command name extraction.
func startProcessLongComm(b *testing.B) *exec.Cmd {
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

	return cmd
}

// BenchmarkProcName benchmarks reading of entire command name from /proc.
func BenchmarkProcName(b *testing.B) {
	cmd := startProcessLongComm(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// create a new process on each iteration to eliminate name caching from the calculation
		proc, err := customNewProcess(int32(cmd.Process.Pid))
		if err != nil {
			b.Fatal(err)
		}
		comm, err := proc.Name()
		if err != nil {
			b.Fatal(err)
		}
		if len(comm) != len(longComm) {
			b.Fatalf("wrong comm length, expected: %d, got: %d (%s)", len(longComm), len(comm), comm)
		}
	}
}

// BenchmarkShouldIgnoreComm benchmarks reading of command name from /proc/<pid>/comm.
func BenchmarkShouldIgnoreComm(b *testing.B) {
	discovery := newDiscovery()
	cmd := startProcessLongComm(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		proc, err := customNewProcess(int32(cmd.Process.Pid))
		if err != nil {
			b.Fatal(err)
		}
		ok := discovery.shouldIgnoreComm(proc)
		if ok {
			b.Fatalf("process should not have been ignored")
		}
	}
}

// BenchmarkProcCommReadFile reads content of /proc/<pid>/comm with default buffer allocation.
func BenchmarkProcCommReadFile(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := os.ReadFile("/proc/1/comm")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProcCommReadFile reads content of /proc/<pid>/comm using pre-allocated pool of buffers.
func BenchmarkProcCommReadLen(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		file, err := os.Open("/proc/1/comm")
		if err != nil {
			b.Fatal(err)
		}
		buf := procCommBufferPool.Get()

		_, err = file.Read(*buf)
		if err != nil {
			b.Fatal(err)
		}
		file.Close()
		procCommBufferPool.Put(buf)
	}
}
