// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	gotlsutils "github.com/DataDog/datadog-agent/pkg/network/protocols/tls/gotls/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	srvAddr = "127.0.0.1:5050"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) []rune {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return b
}

type usmGRPCSuite struct {
	suite.Suite
	isTLS bool
}

func TestGRPCScenarios(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http2.MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", http2.MinimumKernelVersion)
	}

	rand.Seed(time.Now().UnixNano())

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
		ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, tc.name, func(t *testing.T) {
			if tc.isTLS && !gotlsutils.GoTLSSupported(&config.Config{}) {
				t.Skip("GoTLS not supported for this setup")
			}

			suite.Run(t, &usmGRPCSuite{isTLS: tc.isTLS})
		})
	}
}

func getGRPCClientsArray(t *testing.T, size int, withTLS bool) ([]*grpc.Client, func()) {
	res := make([]*grpc.Client, size)
	for i := 0; i < size; i++ {
		client, err := grpc.NewClient(srvAddr, grpc.Options{}, withTLS)
		require.NoError(t, err)
		res[i] = &client
	}

	return res, func() {
		for i := 0; i < size; i++ {
			res[i].Close()
		}
	}
}

func (s *usmGRPCSuite) getConfig() *config.Config {
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS

	return cfg
}

func (s *usmGRPCSuite) TestSimpleGRPCScenarios() {
	t := s.T()

	srv, cancel := grpc.NewGRPCTLSServer(t, srvAddr, s.isTLS)
	t.Cleanup(cancel)
	defaultCtx := context.Background()

	// c is a stream endpoint
	// a + b are unary endpoints
	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedEndpoints map[http.Key]captureRange
		expectedError     bool
	}{
		{
			name: "simple unary - multiple requests",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				for i := 0; i < 1000; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					require.NoError(t, client.HandleUnary(defaultCtx, "first"))
				}
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 999,
					upper: 1001,
				},
			},
		},
		{
			name: "unary, a->b->a",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, "first"))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: {
					lower: 1,
					upper: 1,
				},
			},
		},
		{
			name: "unary, a->b->a->b",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, "third"))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
			},
		},
		{
			name: "unary, a->b->b->a",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].HandleUnary(defaultCtx, "third"))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
			},
		},
		{
			name: "stream, c",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				for i := 0; i < 25; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					require.NoError(t, client.HandleStream(defaultCtx, 10))
				}
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/protobuf.Math/Max")},
					Method: http.MethodPost,
				}: {
					lower: 25,
					upper: 25,
				},
			},
		},
		{
			name: "mixed, c->b->c->b",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleStream(defaultCtx, 10))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleStream(defaultCtx, 10))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].HandleUnary(defaultCtx, "second"))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/protobuf.Math/Max")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
			},
		},
		{
			name: "500 headers -> b -> 500 headers -> b",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 500)
				for i := 1; i <= 500; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value-%d", i)
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := randStringRunes(1024 * 1024)
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, string(longName)))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(ctxWithHeaders, string(longName)))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(ctxWithoutHeaders, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
			},
			expectedError: true,
		},
		{
			name: "duplicated headers -> b -> duplicated headers -> b",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 20)
				for i := 1; i <= 20; i++ {
					headers[fmt.Sprintf("header-%d", i)] = "value"
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := randStringRunes(1024 * 1024)
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, string(longName)))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, string(longName)))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].GetFeature(ctxWithoutHeaders, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
			},
			expectedError: true,
		},
	}
	for _, tt := range tests {
		for _, clientCount := range []int{1, 2, 5} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", clientCount)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				// we are currently not supporting some edge cases:
				// https://datadoghq.atlassian.net/browse/USMO-222
				if tt.expectedError {
					t.Skip("Skipping test due to known issue")
				}
				s.testGRPCScenarios(t, srv.Process.Pid, tt.runClients, tt.expectedEndpoints, clientCount)
			})
		}
	}
}

func (s *usmGRPCSuite) TestLargeBodiesGRPCScenarios() {
	t := s.T()
	if s.isTLS {
		t.Skip("Skipping TestLargeBodiesGRPCScenarios for TLS due to flakiness")
	}

	srv, cancel := grpc.NewGRPCTLSServer(t, srvAddr, s.isTLS)
	t.Cleanup(cancel)
	defaultCtx := context.Background()

	// Random string generation is an heavy operation, and it's proportional for the length (30MB)
	// Instead of generating the same long string (long name) for every test, we're generating it only once.
	longRandomString := randStringRunes(30 * 1024 * 1024)
	shortRandomString := longRandomString[:5*1024*1024]

	// c is a stream endpoint
	// a + b are unary endpoints
	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedEndpoints map[http.Key]captureRange
	}{
		{
			name: "request with large body (30MB)",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				longRandomString[0] = '0' + rune(clientsCount)
				for i := 0; i < 5; i++ {
					longRandomString[1] = 'a' + rune(i)
					require.NoError(t, clients[getClientsIndex(i, clientsCount)].HandleUnary(defaultCtx, string(longRandomString)))
				}
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 4,
					upper: 5,
				},
			},
		},
		{
			name: "request with large body (5MB) -> b -> request with large body (5MB) -> b",
			runClients: func(t *testing.T, clientsCount int) {
				clients, cleanup := getGRPCClientsArray(t, clientsCount, s.isTLS)
				defer cleanup()
				longRandomString[3] = '0' + rune(clientsCount)

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, string(shortRandomString)))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, string(shortRandomString)))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]captureRange{
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: {
					lower: 2,
					upper: 2,
				},
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: {
					lower: 1,
					upper: 2,
				},
			},
		},
	}
	for _, tt := range tests {
		for _, clientCount := range []int{1, 2, 5} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", clientCount)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				s.testGRPCScenarios(t, srv.Process.Pid, tt.runClients, tt.expectedEndpoints, clientCount)
			})
		}
	}
}

func (s *usmGRPCSuite) testGRPCScenarios(t *testing.T, srvPID int, runClientCallback func(*testing.T, int), expectedEndpoints map[http.Key]captureRange, clientCount int) {
	cfg := s.getConfig()
	usmMonitor, err := NewMonitor(cfg, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, usmMonitor.Start())
	t.Cleanup(usmMonitor.Stop)
	t.Cleanup(utils.ResetDebugger)
	if s.isTLS {
		utils.WaitForProgramsToBeTraced(t, "go-tls", srvPID)
	}

	runClientCallback(t, clientCount)

	res := make(map[http.Key]int)
	assert.Eventually(t, func() bool {
		stats := usmMonitor.GetProtocolStats()
		http2Stats, ok := stats[protocols.HTTP2]
		if !ok {
			return false
		}
		http2StatsTyped := http2Stats.(map[http.Key]*http.RequestStats)
		for key, stat := range http2StatsTyped {
			if key.DstPort == 5050 || key.SrcPort == 5050 {
				count := stat.Data[200].Count
				newKey := http.Key{
					Path:   http.Path{Content: key.Path.Content},
					Method: key.Method,
				}
				if _, ok := res[newKey]; !ok {
					res[newKey] = count
				} else {
					res[newKey] += count
				}
			}
		}

		if len(res) != len(expectedEndpoints) {
			return false
		}

		for key, count := range res {
			valRange, ok := expectedEndpoints[key]
			if !ok {
				return false
			}
			if count < valRange.lower || count > valRange.upper {
				return false
			}
		}

		return true
	}, time.Second*5, time.Millisecond*100, "%v != %v", res, expectedEndpoints)
	if t.Failed() {
		ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, "http2_in_flight", "http2_dynamic_table")
	}
}
