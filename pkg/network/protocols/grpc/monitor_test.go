// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package grpc

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
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
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
	fixtures grpcFixtures
}

func TestGRPCScenarios(t *testing.T) {
	// t.Skip("tests are broken after upgrading go-grpc to 1.58")
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http2.MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", http2.MinimumKernelVersion)
	}

	rand.Seed(time.Now().UnixNano())

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "without TLS", func(t *testing.T) {
		grpcSuite := &USMgRPCSuite{
			fixtures: &plainGRPCFixtures{},
		}

		suite.Run(t, grpcSuite)
	})

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}, "with TLS", func(t *testing.T) {
		grpcSuite := &USMgRPCSuite{
			fixtures: &tlsGRPCFixtures{},
		}

		suite.Run(t, grpcSuite)
	})
}

func getClientsArray(t *testing.T, size int, withTLS bool) []*grpc.Client {
	res := make([]*grpc.Client, size)
	for i := 0; i < size; i++ {
		client, err := grpc.NewClient(srvAddr, grpc.Options{}, withTLS)
		require.NoError(t, err)
		t.Cleanup(client.Close)
		res[i] = &client
	}

	return res
}

func getClientsIndex(index, totalCount int) int {
	return index % totalCount
}

func (s *USMgRPCSuite) TestSimpleGRPCScenarios() {
	t := s.T()
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true

	s.fixtures.StartServer(t)
	defaultCtx := context.Background()

	// c is a stream endpoint
	// a + b are unary endpoints
	tests := []struct {
		name              string
		clientsSetupFn    func(t *testing.T, clientsCount int, withTLS bool)
		expectedEndpoints map[http.Key]int
		expectedError     bool
	}{
		{
			name: "simple unary - multiple requests",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				for i := 0; i < 1000; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					require.NoError(t, client.HandleUnary(defaultCtx, "first"))
				}
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 1000,
			},
		},
		{
			name: "unary, a->b->a",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, "first"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: 1,
			},
		},
		{
			name: "unary, a->b->a->b",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, "third"))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "unary, a->b->b->a",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].HandleUnary(defaultCtx, "third"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "stream, c",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				for i := 0; i < 25; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					require.NoError(t, client.HandleStream(defaultCtx, 10))
				}
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/protobuf.Math/Max")},
					Method: http.MethodPost,
				}: 25,
			},
		},
		{
			name: "mixed, c->b->c->b",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleStream(defaultCtx, 10))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleStream(defaultCtx, 10))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].HandleUnary(defaultCtx, "second"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/protobuf.Math/Max")},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "500 headers -> b -> 500 headers -> b",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

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
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 2,
			},
			expectedError: true,
		},
		{
			name: "duplicated headers -> b -> duplicated headers -> b",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 20)
				for i := 1; i <= 20; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value")
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := randStringRunes(1024 * 1024)
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, string(longName)))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, string(longName)))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].GetFeature(ctxWithoutHeaders, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: http.Interner.GetString("/routeguide.RouteGuide/GetFeature")},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: http.Interner.GetString("/helloworld.Greeter/SayHello")},
					Method: http.MethodPost,
				}: 2,
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

				mon := s.fixtures.GetMonitor(t, cfg)

				s.fixtures.RunClients(t, clientCount, tt.clientsSetupFn)

				res := make(map[http.Key]int)
				require.Eventually(t, func() bool {
					http2Stats := mon.GetHTTP2Stats()
					if http2Stats == nil {
						return false
					}
					for key, stat := range http2Stats {
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
						if val != count {
							return false
						}
					}

					return true
				}, time.Second*5, time.Millisecond*100, "%v != %v", res, tt.expectedEndpoints)
			})
		}
	}
}

type captureRange struct {
	lower int
	upper int
}

func (s *USMgRPCSuite) TestLargeBodiesGRPCScenarios() {
	t := s.T()
	cfg := config.New()
	cfg.EnableHTTP2Monitoring = true
	cfg.BPFDebug = true

	s.fixtures.StartServer(t)
	defaultCtx := context.Background()

	// Random string generation is an heavy operation, and it's proportional for the length (30MB)
	// Instead of generating the same long string (long name) for every test, we're generating it only once.
	longRandomString := randStringRunes(30 * 1024 * 1024)
	shortRandomString := longRandomString[:5*1024*1024]

	// c is a stream endpoint
	// a + b are unary endpoints
	tests := []struct {
		name              string
		clientsSetupFn    func(t *testing.T, clientsCount int, withTLS bool)
		expectedEndpoints map[http.Key]captureRange
	}{
		{
			name: "request with large body (30MB)",
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

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
			clientsSetupFn: func(t *testing.T, clientsCount int, withTLS bool) {
				clients := getClientsArray(t, clientsCount, withTLS)

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
				mon := s.fixtures.GetMonitor(t, cfg)

				s.fixtures.RunClients(t, clientCount, tt.clientsSetupFn)

				res := make(map[http.Key]int)
				assert.Eventually(t, func() bool {
					http2Stats := mon.GetHTTP2Stats()
					if http2Stats == nil {
						return false
					}
					for key, stat := range http2Stats {
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
			})
		}
	}
}

type grpcFixtures interface {
	StartServer(t *testing.T)
	GetMonitor(t *testing.T, cfg *config.Config) abstractMonitor
	RunClients(t *testing.T, clientCount int, clientsSetupFn func(t *testing.T, clientsCount int, withTLS bool))
}

type abstractMonitor interface {
	GetHTTP2Stats() map[http.Key]*http.RequestStats
}

type plainGRPCFixtures struct{}

func (f *plainGRPCFixtures) StartServer(t *testing.T) {
	t.Helper()

	srv, err := grpc.NewServer(srvAddr, false)
	require.NoError(t, err)
	srv.Run()
	t.Cleanup(srv.Stop)
}

func (f *plainGRPCFixtures) GetMonitor(t *testing.T, cfg *config.Config) abstractMonitor {
	t.Helper()

	monitor, err := usm.NewMonitor(cfg, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, monitor.Start())
	t.Cleanup(monitor.Stop)

	// Make the monitor dump the maps on failure
	t.Cleanup(func() {
		if t.Failed() {
			o, err := monitor.DumpMaps("http2_in_flight")
			if err != nil {
				t.Logf("failed dumping http2_in_flight: %s", err)
			} else {
				t.Log(o)
			}
			o, err = monitor.DumpMaps("http2_dynamic_table")
			if err != nil {
				t.Logf("failed dumping http2_dynamic_table: %s", err)
			} else {
				t.Log(o)
			}
		}
	})

	return &usmMonitorWrapper{monitor}
}

func (f *plainGRPCFixtures) RunClients(t *testing.T, clientCount int, clientsSetupFn func(t *testing.T, clientsCount int, withTLS bool)) {
	t.Helper()

	clientsSetupFn(t, clientCount, false)
}

type usmMonitorWrapper struct {
	monitor *usm.Monitor
}

func (m *usmMonitorWrapper) GetHTTP2Stats() map[http.Key]*http.RequestStats {
	stats := m.monitor.GetProtocolStats()
	http2Stats, ok := stats[protocols.HTTP2]
	if !ok {
		return nil
	}
	return http2Stats.(map[http.Key]*http.RequestStats)
}

type tlsGRPCFixtures struct{}

func (f *tlsGRPCFixtures) StartServer(t *testing.T) {
	t.Helper()

	_, cancel := grpc.NewGRPCTLSServer(t)

	t.Cleanup(cancel)
}

func (f *tlsGRPCFixtures) GetMonitor(t *testing.T, cfg *config.Config) abstractMonitor {
	t.Helper()

	tracer, err := tracer.NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tracer.Stop)

	// Make the monitor dump the maps on failure
	t.Cleanup(func() {
		if t.Failed() {
			o, err := tracer.DebugEBPFMaps("http2_in_flight", "http2_dynamic_table")
			if err != nil {
				t.Logf("failed dumping USM maps: %s", err)
			} else {
				t.Log(o)
			}
		}
	})

	return &tracerWrapper{tracer}
}

func (f *tlsGRPCFixtures) RunClients(t *testing.T, clientCount int, clientsSetupFn func(t *testing.T, clientsCount int, withTLS bool)) {
	t.Helper()

	clientsSetupFn(t, clientCount, true)
}

type tracerWrapper struct {
	tracer *tracer.Tracer
}

func (t *tracerWrapper) GetHTTP2Stats() map[http.Key]*http.RequestStats {
	const clientID = "1"

	connections, err := t.tracer.GetActiveConnections(clientID)
	if err != nil {
		return nil
	}

	return connections.HTTP2
}
