// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
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

type USMgRPCSuite struct {
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

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "without TLS", func(t *testing.T) {
		grpcSuite := &USMgRPCSuite{
			isTLS: false,
		}

		suite.Run(t, grpcSuite)
	})

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "with TLS", func(t *testing.T) {
		if !goTLSSupported() {
			t.Skip("GoTLS not supported for this setup")
		}

		// TODO fix TestHTTPGoTLSAttachProbes on these Fedora versions
		if skipFedora(t) {
			// TestHTTPGoTLSAttachProbes fails consistently in CI on Fedora 36,37
			t.Skip("TestHTTPGoTLSAttachProbes fails on this OS consistently")
		}
		grpcSuite := &USMgRPCSuite{
			isTLS: true,
		}

		suite.Run(t, grpcSuite)
	})
}

func getClientsArray(t *testing.T, size int, withTLS bool) ([]*grpc.Client, func()) {
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

func getClientsIndex(index, totalCount int) int {
	return index % totalCount
}

type captureRange struct {
	lower int
	upper int
}

func (s *USMgRPCSuite) getConfig() *config.Config {
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true
	cfg.EnableGoTLSSupport = s.isTLS
	cfg.GoTLSExcludeSelf = s.isTLS

	return cfg
}

func (s *USMgRPCSuite) TestSimpleGRPCScenarios() {
	t := s.T()

	srv, cancel := grpc.NewGRPCTLSServer(t, srvAddr, s.isTLS)
	t.Cleanup(cancel)
	defaultCtx := context.Background()

	cfg := s.getConfig()

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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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

				tr := setupTracer(t, cfg)
				require.NotNil(t, tr.usmMonitor)

				if s.isTLS {
					require.Eventuallyf(t, func() bool {
						traced := utils.GetTracedPrograms("go-tls")
						for _, prog := range traced {
							if slices.Contains[[]uint32](prog.PIDs, uint32(srv.Process.Pid)) {
								return true
							}
						}
						return false
					}, time.Second*15, time.Millisecond*100, "process %v is not traced by gotls", srv.Process.Pid)
				}

				tr.removeClient(clientID)
				initTracerState(t, tr)

				tt.runClients(t, clientCount)

				res := make(map[http.Key]int)
				assert.Eventually(t, func() bool {
					conns := getConnections(t, tr)
					for key, stat := range conns.HTTP2 {
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

					if len(res) != len(tt.expectedEndpoints) {
						return false
					}

					for key, count := range res {
						val, ok := tt.expectedEndpoints[key]
						if !ok {
							return false
						}
						if val.lower > count || val.upper < count {
							return false
						}
					}

					return true
				}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
				if t.Failed() && tr.usmMonitor != nil {
					ebpftest.DumpMapsTestHelper(t, tr.usmMonitor.DumpMaps, "http2_in_flight", "http2_dynamic_table")
				}
			})
		}
	}
}

func (s *USMgRPCSuite) TestLargeBodiesGRPCScenarios() {
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

	cfg := s.getConfig()
	tr := setupTracer(t, cfg)
	require.NotNil(t, tr.usmMonitor)

	if s.isTLS {
		require.Eventuallyf(t, func() bool {
			traced := utils.GetTracedPrograms("go-tls")
			for _, prog := range traced {
				if slices.Contains[[]uint32](prog.PIDs, uint32(srv.Process.Pid)) {
					return true
				}
			}
			return false
		}, time.Second*15, time.Millisecond*100, "process %v is not traced by gotls", srv.Process.Pid)
	}

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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				clients, cleanup := getClientsArray(t, clientsCount, s.isTLS)
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
				tr.removeClient(clientID)
				initTracerState(t, tr)

				tt.runClients(t, clientCount)

				res := make(map[http.Key]int)
				assert.Eventually(t, func() bool {
					conns := getConnections(t, tr)
					for key, stat := range conns.HTTP2 {
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

					if len(res) != len(tt.expectedEndpoints) {
						return false
					}

					for key, count := range res {
						valRange, ok := tt.expectedEndpoints[key]
						if !ok {
							return false
						}
						if count < valRange.lower || count > valRange.upper {
							return false
						}
					}

					return true
				}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)

				if t.Failed() && tr.usmMonitor != nil {
					ebpftest.DumpMapsTestHelper(t, tr.usmMonitor.DumpMaps, "http2_in_flight", "http2_dynamic_table")
				}
			})
		}
	}
}
