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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
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
	for comm := range ignoreComms {
		assert.LessOrEqual(t, len(comm), 15, "Process name %q too big", comm)
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

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() { cancel() })

			makeAlias(t, test.comm, serverBin)
			bin := filepath.Join(serverDir, test.comm)
			cmd := exec.CommandContext(ctx, bin)
			err := cmd.Start()
			require.NoError(t, err)

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				proc, err := customNewProcess(int32(cmd.Process.Pid))
				require.NoError(t, err)
				ignore := shouldIgnoreComm(proc)
				require.Equal(t, test.ignore, ignore)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

func TestLoadIgnoredComm(t *testing.T) {
	tests := []struct {
		name   string
		comms  string
		expect []bool
	}{
		{
			name:   "empty list of ignored commands",
			comms:  "",
			expect: []bool{false},
		},
		{
			name:   "short commands in config list",
			comms:  "cron, polkitd, rsyslogd, bash, sshd, dockerd",
			expect: []bool{true, true, true, true, true, true},
		},
		{
			name:   "malformed commands list",
			comms:  "rsyslogd, , snapd, ,   udisksd, containerd, ,    ",
			expect: []bool{true, false, true, false, true, true, false, false},
		},
		{
			name:   "long commands in config list",
			comms:  "containerd-shim-runc-v2,unattended-upgrade-shutdown,kube-controller-manager",
			expect: []bool{true, true, true},
		},
		{
			name:   "commands of different lengths in the configuration list",
			comms:  "containerd-shim-runc-v2,calico-node,unattended-upgrade-shutdown,bash,kube-controller-manager",
			expect: []bool{true, true, true, true, true},
		},
	}
	defLen := len(ignoreComms)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockSystemProbe := mock.NewSystemProbe(t)
			require.NotEmpty(t, mockSystemProbe)

			mockSystemProbe.SetWithoutSource("discovery.ignore_comms", test.comms)

			LoadIgnoredComms(newConfig())

			comms := strings.Split(strings.ReplaceAll(test.comms, " ", ""), ",")
			if len(comms) > 0 {
				if len(comms[0]) == 0 {
					require.Equal(t, len(ignoreComms), defLen)
				} else {
					require.Greater(t, len(ignoreComms), defLen)
				}
			}
			for n, exp := range test.expect {
				if len(comms[n]) > 15 {
					_, found := ignoreComms[comms[n][:15]]
					require.Equal(t, exp, found)
				} else {
					_, found := ignoreComms[comms[n]]
					require.Equal(t, exp, found)
				}
			}
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
	cmd := startProcessLongComm(b)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		proc, err := customNewProcess(int32(cmd.Process.Pid))
		if err != nil {
			b.Fatal(err)
		}
		ok := shouldIgnoreComm(proc)
		if ok {
			b.Fatalf("process should not have been ignored")
		}
	}
}
