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
						require.True(t, ok, "expected endpoint mismatch")
						require.Equal(t, value, count, "expected endpoint mismatch")
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
