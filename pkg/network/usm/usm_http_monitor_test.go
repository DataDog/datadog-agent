// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	usmhttp "github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

const (
	httpSrvPort = 8089
)

var (
	serverAddrIPV4 = net.JoinHostPort("127.0.0.1", strconv.Itoa(httpSrvPort))
	serverAddrIPV6 = net.JoinHostPort("::1", strconv.Itoa(httpSrvPort))
)

func getServerAddress(isIPV6 bool) string {
	if isIPV6 {
		return serverAddrIPV6
	}
	return serverAddrIPV4
}

type usmHTTPSuite struct {
	suite.Suite
	isTLS bool
}

func (s *usmHTTPSuite) getCfg() *config.Config {
	cfg := utils.NewUSMEmptyConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS
	return cfg
}

func (s *usmHTTPSuite) getSchemeURL(url string) string {
	if s.isTLS {
		return "https://" + url
	}
	return "http://" + url
}

func TestHTTPScenarios(t *testing.T) {
	skipTestIfKernelNotSupported(t)

	ebpftest.TestBuildModes(t, usmtestutil.SupportedBuildModes(), "", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			isTLS bool
		}{
			{
				name:  "without TLS",
				isTLS: false,
			},
			{
				name:  "with TLS",
				isTLS: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if tc.isTLS && !gotlsutils.GoTLSSupported(t, config.New()) {
					t.Skip("GoTLS not supported for this setup")
				}
				suite.Run(t, &usmHTTPSuite{isTLS: tc.isTLS})
			})
		}
	})
}

func (s *usmHTTPSuite) TestLoadHTTPBinary() {
	t := s.T()

	cfg := s.getCfg()

	for _, debug := range map[string]bool{"enabled": true, "disabled": false} {
		t.Run(fmt.Sprintf("debug %v", debug), func(t *testing.T) {
			cfg.BPFDebug = debug
			setupUSMTLSMonitor(t, cfg, useExistingConsumer)
		})
	}
}

func (s *usmHTTPSuite) TestSimple() {
	t := s.T()
	for name, isIPv6 := range map[string]bool{"IPv4": false, "IPv6": true} {
		t.Run(name, func(t *testing.T) {
			s.testSimple(t, isIPv6)
		})
	}
}

func (s *usmHTTPSuite) testSimple(t *testing.T, isIPv6 bool) {
	cfg := s.getCfg()

	srvDoneFn := testutil.HTTPServer(t, getServerAddress(isIPv6), testutil.Options{
		EnableTLS:       s.isTLS,
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	// Start the proxy server.
	proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, getServerAddress(isIPv6), s.isTLS, isIPv6)
	t.Cleanup(cancel)
	require.NoError(t, proxy.WaitForConnectionReady(unixPath))

	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)
	}

	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedEndpoints map[usmhttp.Key]int
	}{
		{
			name: "multiple get requests",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getHTTPUnixClientArray(clientsCount, unixPath)

				for i := 0; i < 10; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					req, err := client.Get(s.getSchemeURL(getServerAddress(isIPv6) + "/200/hello"))
					require.NoError(t, err, "could not make request")
					_ = req.Body.Close()
				}

				for _, client := range clients {
					client.CloseIdleConnections()
				}
			},
			expectedEndpoints: map[usmhttp.Key]int{
				{
					Path:   usmhttp.Path{Content: usmhttp.Interner.GetString("/200/hello")},
					Method: usmhttp.MethodGet,
				}: 10,
			},
		},
	}
	for _, tt := range tests {
		for _, clientCount := range []int{1, 2, 5} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", clientCount)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				t.Cleanup(func() { cleanProtocolMaps(t, "http", monitor.ebpfProgram.Manager.Manager) })
				tt.runClients(t, clientCount)

				res := make(map[usmhttp.Key]int)
				assert.EventuallyWithT(t, func(collect *assert.CollectT) {
					for key, stat := range getHTTPLikeProtocolStats(t, monitor, protocols.HTTP) {
						if key.DstPort == httpSrvPort || key.SrcPort == httpSrvPort {
							count := stat.Data[200].Count
							newKey := usmhttp.Key{
								Path:   usmhttp.Path{Content: key.Path.Content},
								Method: key.Method,
							}
							if _, ok := res[newKey]; !ok {
								res[newKey] = count
							} else {
								res[newKey] += count
							}
						}
					}

					require.Equal(collect, len(tt.expectedEndpoints), len(res), "expected endpoints count mismatch")

					for key, count := range res {
						value, ok := tt.expectedEndpoints[key]
						require.True(collect, ok, "expected endpoint mismatch")
						require.Equal(collect, value, count, "expected endpoint mismatch")
					}
				}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
				if t.Failed() {
					for key := range tt.expectedEndpoints {
						if _, ok := res[key]; !ok {
							t.Logf("key: %v was not found in res", key.Path.Content.Get())
						}
					}
					ebpftest.DumpMapsTestHelper(t, monitor.DumpMaps, "http_in_flight")
				}
			})
		}
	}
}

// getHTTPUnixClientArray creates an array of http clients over a unix socket.
func getHTTPUnixClientArray(size int, unixPath string) []*http.Client {
	res := make([]*http.Client, size)
	for i := 0; i < size; i++ {
		res[i] = &http.Client{
			Transport: &http.Transport{
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					return net.Dial("unix", unixPath)
				},
				DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
					return net.Dial("unix", unixPath)
				},
			},
		}
	}

	return res
}

func TestGoTLSMapCleanup(t *testing.T) {
	// This test reproduces the Go-TLS map leak by:
	// 1. Creating proxy processes that make HTTPS requests (populates conn_tup_by_go_tls_conn map)
	// 2. Abruptly terminating each proxy with cancel() (simulates SIGKILL)
	// 3. Repeating 10 times to create multiple leak opportunities
	// 4. Verifying all map entries are eventually cleaned up by tcp_close kprobe

	if !gotlsutils.GoTLSSupported(t, config.New()) {
		t.Skip("GoTLS not supported on this platform")
	}

	cfg := utils.NewUSMEmptyConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableGoTLSSupport = true
	cfg.GoTLSExcludeSelf = false

	srvDoneFn := testutil.HTTPServer(t, serverAddrIPV4, testutil.Options{
		EnableTLS:       true,
		EnableKeepAlive: true,
	})
	t.Cleanup(srvDoneFn)

	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)

	mapsName := []string{
		connectionTupleByGoTLSMap,
		goTLSConnByTupleMap,
	}
	mapsInstances := make([]*ebpf.Map, len(mapsName))
	for i, name := range mapsName {
		m, ok, err := monitor.ebpfProgram.Manager.GetMap(name)
		require.NoError(t, err)
		require.True(t, ok, "map %s should exist", name)
		mapsInstances[i] = m

		require.Zero(t, utils.CountMapEntries(t, m), "map %s should be empty at start", name)
	}

	for j := 0; j < 10; j++ {
		proxyProcess, cancel := proxy.NewExternalUnixTransparentProxyServer(t, unixPath, serverAddrIPV4, true, false)
		require.NoError(t, proxy.WaitForConnectionReady(unixPath))
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, GoTLSAttacherName, proxyProcess.Process.Pid, utils.ManualTracingFallbackEnabled)

		clients := getHTTPUnixClientArray(5, unixPath)
		for i := 0; i < 10; i++ {
			req, err := clients[getClientsIndex(i, len(clients))].Get("https://" + serverAddrIPV4 + "/200/hello")
			require.NoError(t, err, "could not make request")
			_ = req.Body.Close()
		}
		cancel()

		for _, client := range clients {
			client.CloseIdleConnections()
		}
	}

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		for _, m := range mapsInstances {
			count := utils.CountMapEntries(t, m)
			assert.Zero(collect, count, "map %s should be empty after proxy exit", m.String())
		}
	}, 5*time.Second, 100*time.Millisecond, "maps should be empty after proxy exit")
}
