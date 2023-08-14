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

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	srvAddr = "127.0.0.1:5050"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func TestGRPCScenarios(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http2.MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", http2.MinimumKernelVersion)
	}

	rand.Seed(time.Now().UnixNano())

	ebpftest.TestBuildModes(t, []ebpftest.BuildMode{ebpftest.Prebuilt, ebpftest.RuntimeCompiled, ebpftest.CORE}, "", testGRPCScenarios)
}

func getClientsArray(t *testing.T, size int, options grpc.Options) []*grpc.Client {
	res := make([]*grpc.Client, size)
	for i := 0; i < size; i++ {
		client, err := grpc.NewClient(srvAddr, options)
		require.NoError(t, err)
		t.Cleanup(client.Close)
		res[i] = &client
	}

	return res
}

func getClientsIndex(index, totalCount int) int {
	return index % totalCount
}

func testGRPCScenarios(t *testing.T) {
	cfg := config.New()
	cfg.BPFDebug = true
	cfg.EnableHTTP2Monitoring = true

	s, err := grpc.NewServer(srvAddr)
	require.NoError(t, err)
	s.Run()
	t.Cleanup(s.Stop)
	defaultCtx := context.Background()

	// c is a stream endpoint
	// a + b are unary endpoints
	tests := []struct {
		name              string
		runClients        func(t *testing.T, clientsCount int)
		expectedEndpoints map[http.Key]int
		expectedError     bool
	}{
		{
			name: "simple unary - multiple requests",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				for i := 0; i < 1000; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					require.NoError(t, client.HandleUnary(defaultCtx, "first"))
				}
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 1000,
			},
		},
		{
			name: "unary, a->b->a",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, "first"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 1,
			},
		},
		{
			name: "unary, a->b->a->b",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, "third"))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "unary, a->b->b->a",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].HandleUnary(defaultCtx, "third"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "stream, c",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				for i := 0; i < 25; i++ {
					client := clients[getClientsIndex(i, clientsCount)]
					require.NoError(t, client.HandleStream(defaultCtx, 10))
				}
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/protobuf.Math/Max"},
					Method: http.MethodPost,
				}: 25,
			},
		},
		{
			name: "mixed, c->b->c->b",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleStream(defaultCtx, 10))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].HandleUnary(defaultCtx, "first"))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleStream(defaultCtx, 10))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].HandleUnary(defaultCtx, "second"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/protobuf.Math/Max"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "request with large body (50MB)",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				for i := 0; i < 5; i++ {
					longName := randStringRunes(50 * 1024 * 1024)
					require.NoError(t, clients[getClientsIndex(i, clientsCount)].HandleUnary(defaultCtx, longName))
				}
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 5,
			},
		},
		{
			name: "request with large body (5MB) -> b -> request with large body (5MB) -> b",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				longName := randStringRunes(5 * 1024 * 1024)

				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(defaultCtx, longName))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(defaultCtx, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(defaultCtx, longName))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(defaultCtx, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
			},
		},
		{
			name: "500 headers -> b -> 500 headers -> b",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 500)
				for i := 1; i <= 500; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value-%d", i)
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := randStringRunes(1024 * 1024)
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, clients[getClientsIndex(1, clientsCount)].GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(2, clientsCount)].HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, clients[getClientsIndex(3, clientsCount)].GetFeature(ctxWithoutHeaders, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
			},
			expectedError: true,
		},
		{
			name: "duplicated headers -> b -> duplicated headers -> b",
			runClients: func(t *testing.T, clientsCount int) {
				clients := getClientsArray(t, clientsCount, grpc.Options{})

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 20)
				for i := 1; i <= 20; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value")
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := randStringRunes(1024 * 1024)
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, clients[getClientsIndex(0, clientsCount)].GetFeature(ctxWithoutHeaders, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
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

				monitor, err := usm.NewMonitor(cfg, nil, nil, nil)
				require.NoError(t, err)
				require.NoError(t, monitor.Start())
				defer monitor.Stop()

				tt.runClients(t, clientCount)

				res := make(map[http.Key]int)
				require.Eventually(t, func() bool {
					stats := monitor.GetProtocolStats()
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
