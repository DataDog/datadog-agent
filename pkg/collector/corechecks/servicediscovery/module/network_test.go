// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
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
		config := newConfig()
		config.BPFDebug = true
		out, err := runtimeCompile(config)
		require.NoError(t, err)
		_ = out.Close()
	})
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
				config := newConfig()
				config.BPFDebug = true
				collector, err := newNetworkCollector(config)
				require.NoError(t, err)
				t.Cleanup(func() { collector.close() })

				runServer(t, test.proto, test.addr)

				pid := uint32(os.Getpid())
				err = collector.addPid(pid)
				require.NoError(t, err)

				before, err := collector.getStats(pid)
				require.NoError(t, err)

				t.Log("stats before", before)

				for i := 0; i < iterations; i++ {
					runClient(t, test.proto, test.addr)

					after, err := collector.getStats(pid)
					require.NoError(t, err)

					t.Log("stats after", after)

					assert.Equal(t, clientToServerBytes, int(after.Rx-before.Rx))
					assert.Equal(t, serverToClientBytes, int(after.Tx-before.Tx))
					before = after
				}

				err = collector.removePid(pid)
				require.NoError(t, err)
				_, err = collector.getStats(pid)
				require.Error(t, err)
			})
		}
	})
}

func TestNetwork(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().DoAndReturn(func() time.Time {
		return time.Now()
	}).AnyTimes()

	listener, err := net.Listen("tcp4", ":8087")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	pid := os.Getpid()

	params := defaultParams()
	params.heartbeatTime = 0

	// Get the service to be recognized as started
	_ = getServicesWithParams(t, discovery.url, &params)
	_ = getServicesWithParams(t, discovery.url, &params)

	old := model.Service{}

	// The low-level stats are verified separately by TestNetworkCollector() and
	// the bps calculation is verified with mocks in TestNetworkStats(). This
	// test does some basic assertions just to ensure that everything is
	// hooked up together.
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServicesWithParams(collect, discovery.url, &params)
		service := findService(pid, resp.HeartbeatServices)
		require.NotNil(collect, service)
		assert.NotZero(collect, service.RxBytes)
		assert.NotZero(collect, service.TxBytes)
		assert.NotZero(collect, service.RxBps)
		assert.NotZero(collect, service.TxBps)
		old = *service
	}, 5*time.Second, 100*time.Millisecond)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServicesWithParams(collect, discovery.url, &params)
		service := findService(pid, resp.HeartbeatServices)
		require.NotNil(collect, service)
		assert.Greater(collect, service.RxBytes, old.RxBytes)
		assert.Greater(collect, service.TxBytes, old.TxBytes)
		assert.NotEqual(collect, old.RxBps, service.RxBps)
		assert.NotEqual(collect, old.TxBps, service.TxBps)
	}, 5*time.Second, 100*time.Millisecond)
}

func TestGetNetworkCollectorError(t *testing.T) {
	_ = setupDiscoveryModuleWithNetwork(t, func(_ *discoveryConfig) (networkCollector, error) {
		return nil, errors.New("fail")
	})
}

func TestNetworkStatsDisabled(t *testing.T) {
	t.Setenv("DD_DISCOVERY_NETWORK_STATS_ENABLED", "false")

	setupDiscoveryModuleWithNetwork(t, func(_ *discoveryConfig) (networkCollector, error) {
		t.FailNow()
		return nil, nil
	})
}

func TestNetworkStats(t *testing.T) {
	startService := func() (*exec.Cmd, context.CancelFunc) {
		listener, err := net.Listen("tcp", "")
		require.NoError(t, err)
		f, err := listener.(*net.TCPListener).File()
		listener.Close()

		// Disable close-on-exec so that the sleep gets it
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })
		disableCloseOnExec(t, f)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() { cancel() })

		cmd := exec.CommandContext(ctx, "sleep", "1000")
		cmd.Dir = "/tmp/"
		cmd.Env = append(cmd.Env, "DD_SERVICE=foo_bar")
		err = cmd.Start()
		require.NoError(t, err)
		f.Close()

		return cmd, cancel
	}

	stopService := func(cmd *exec.Cmd, cancel context.CancelFunc) {
		cancel()
		_ = cmd.Wait()
	}

	mockCtrl := gomock.NewController(t)
	mock := NewMocknetworkCollector(mockCtrl)
	discovery := setupDiscoveryModuleWithNetwork(t, func(_ *discoveryConfig) (networkCollector, error) {
		return mock, nil
	})

	// Number of calls made to timeProvider.Now() for one call of
	// getServicesWithParams()
	nowCalls := 2

	// Start the service and check we found it.
	cmd, cancel := startService()
	pid := cmd.Process.Pid

	mock.EXPECT().addPid(uint32(pid)).Times(1)
	mock.EXPECT().addPid(gomock.Not(uint32(pid))).AnyTimes()

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		now := mockedTime
		discovery.mockTimeProvider.EXPECT().Now().Return(now).Times(nowCalls)

		resp := getServices(collect, discovery.url)
		startEvent := findService(pid, resp.StartedServices)
		require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
	}, 30*time.Second, 100*time.Millisecond)

	params := defaultParams()
	params.heartbeatTime = 0

	now := mockedTime
	discovery.mockTimeProvider.EXPECT().Now().Return(now).Times(nowCalls)

	_ = getServicesWithParams(t, discovery.url, &params)

	mock.EXPECT().getStats(gomock.Not(uint32(pid))).AnyTimes().Return(NetworkStats{
		Rx: 0,
		Tx: 0,
	}, nil)
	mock.EXPECT().getStats(uint32(pid)).Return(NetworkStats{
		Rx: 1000,
		Tx: 2000,
	}, nil)

	now = now.Add(1 * time.Second)
	discovery.mockTimeProvider.EXPECT().Now().Return(now).Times(nowCalls)

	_ = getServicesWithParams(t, discovery.url, &params)

	now = now.Add(10 * time.Second)
	discovery.mockTimeProvider.EXPECT().Now().Return(now).Times(nowCalls)

	mock.EXPECT().getStats(uint32(pid)).Return(NetworkStats{
		Rx: 3000,
		Tx: 8000,
	}, nil)
	response := getServicesWithParams(t, discovery.url, &params)
	service := findService(pid, response.HeartbeatServices)
	require.NotNil(t, service)
	require.Equal(t, 3000, int(service.RxBytes))
	require.Equal(t, 8000, int(service.TxBytes))
	require.Equal(t, 200, int(service.RxBps))
	require.Equal(t, 600, int(service.TxBps))

	stopService(cmd, cancel)

	discovery.mockTimeProvider.EXPECT().Now().Return(now).AnyTimes()
	mock.EXPECT().removePid(uint32(pid)).Return(nil).Times(1)
	mock.EXPECT().removePid(gomock.Not(uint32(pid))).AnyTimes()
	r := getServicesWithParams(t, discovery.url, &params)
	t.Log(r.StoppedServices)

	mock.EXPECT().close().Times(1)
}
