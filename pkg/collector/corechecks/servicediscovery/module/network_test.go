// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"

	cebpf "github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)

const (
	clientToServerBytes = 13
	serverToClientBytes = 25
	iterations          = 2
)

func runServer(t *testing.T, network, address string) {
	var listener net.Listener
	var err error
	listener, err = net.Listen(network, address)
	require.NoError(t, err)

	done := make(chan bool, 1)

	t.Cleanup(func() {
		<-done
	})

	go func() {
		defer func() {
			listener.Close()
			done <- true
		}()

		for i := 0; i < iterations; i++ {
			conn, err := listener.Accept()
			require.NoError(t, err)

			reader := bufio.NewReader(conn)

			in := make([]byte, clientToServerBytes)
			_, err = io.ReadFull(reader, in)
			require.NoError(t, err)

			out := make([]byte, serverToClientBytes)
			conn.Write(out)
			conn.Close()
		}
	}()
}

func runClient(t *testing.T, proto, addr string) {
	socatProto := strings.ToUpper(proto)
	cmd := exec.Command("socat", fmt.Sprintf("%v:%v", socatProto, addr), "-")
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)

	go func() {
		defer stdin.Close()

		b := make([]byte, clientToServerBytes)
		stdin.Write(b)
	}()

	output, err := cmd.CombinedOutput()
	t.Log("socat", string(output))
	require.NoError(t, err)
}

func TestNetworkCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		config := ebpf.NewConfig()
		config.BPFDebug = true
		out, err := runtimeCompile(config)
		require.NoError(t, err)
		_ = out.Close()
	})
}

func ebpfMapGet(collector core.NetworkCollector, pid int32) (stats core.NetworkStats, err error) {
	stats = core.NetworkStats{}
	err = collector.(*eBPFNetworkCollector).statsMap.Lookup(&core.NetworkStatsKey{Pid: uint32(pid)}, &stats)
	return stats, err
}

func TestNetworkCollector(t *testing.T) {
	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "", func(t *testing.T) {
		tests := []struct {
			proto string
			addr  string
		}{
			{
				proto: "tcp4",
				addr:  "127.0.0.1:8087",
			},
			{
				proto: "tcp6",
				addr:  "[::1]:8087",
			},
		}

		for _, test := range tests {
			t.Run(test.proto, func(t *testing.T) {
				config := ebpf.NewConfig()
				config.BPFDebug = true
				collector, err := newNetworkCollectorWithConfig(config)
				require.NoError(t, err)
				t.Cleanup(func() { collector.Close() })

				runServer(t, test.proto, test.addr)

				pid := uint32(os.Getpid())
				otherPid := pid + 1
				pidSet := make(core.PidSet, 2)
				pidSet.Add(int32(otherPid))
				pidSet.Add(int32(pid))

				stats, err := collector.GetStats(pidSet)
				require.NoError(t, err)
				require.NotContains(t, stats, uint32(pid))
				require.NotContains(t, stats, uint32(otherPid))

				_, err = ebpfMapGet(collector, int32(pid))
				require.NoError(t, err)
				_, err = ebpfMapGet(collector, int32(otherPid))
				require.NoError(t, err)

				beforeAll, err := collector.GetStats(pidSet)
				require.NoError(t, err)
				require.Contains(t, beforeAll, uint32(pid))
				require.Contains(t, beforeAll, uint32(otherPid))
				before := beforeAll[uint32(pid)]

				t.Log("stats before", before)

				for i := 0; i < iterations; i++ {
					runClient(t, test.proto, test.addr)

					afterAll, err := collector.GetStats(pidSet)
					require.NoError(t, err)
					require.Contains(t, afterAll, uint32(pid))
					require.Contains(t, afterAll, uint32(otherPid))
					after := afterAll[uint32(pid)]

					t.Log("stats after", after)

					assert.Equal(t, clientToServerBytes, int(after.Rx-before.Rx))
					assert.Equal(t, serverToClientBytes, int(after.Tx-before.Tx))
					before = after
				}

				// Remove pid and add a new one to check that the map is updated correctly
				pidSet.Remove(int32(pid))
				pidSet.Add(int32(otherPid + 1))
				removedAll, err := collector.GetStats(pidSet)
				require.NoError(t, err)
				require.NotContains(t, removedAll, uint32(pid))
				require.Contains(t, removedAll, uint32(otherPid))
				require.NotContains(t, removedAll, uint32(otherPid+1))

				_, err = ebpfMapGet(collector, int32(pid))
				require.ErrorIs(t, err, cebpf.ErrKeyNotExist)
				_, err = ebpfMapGet(collector, int32(otherPid))
				require.NoError(t, err)
				_, err = ebpfMapGet(collector, int32(otherPid+1))
				require.NoError(t, err)

				// Remove the rest and check that the map is empty
				pidSet.Remove(int32(otherPid))
				pidSet.Remove(int32(otherPid + 1))
				removedAll, err = collector.GetStats(pidSet)
				require.NoError(t, err)
				require.Empty(t, removedAll)
				_, err = ebpfMapGet(collector, int32(otherPid))
				require.ErrorIs(t, err, cebpf.ErrKeyNotExist)
				_, err = ebpfMapGet(collector, int32(otherPid+1))
				require.ErrorIs(t, err, cebpf.ErrKeyNotExist)
			})
		}
	})
}

func TestGetNetworkCollectorError(t *testing.T) {
	_ = setupDiscoveryModuleWithNetwork(t, func(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
		return nil, errors.New("fail")
	})
}

func TestNetworkStatsDisabled(t *testing.T) {
	t.Setenv("DD_DISCOVERY_NETWORK_STATS_ENABLED", "false")

	setupDiscoveryModuleWithNetwork(t, func(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
		t.FailNow()
		return nil, nil
	})
}
