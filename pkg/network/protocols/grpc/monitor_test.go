// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package grpc

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
)

const (
	srvAddr = "127.0.0.1:5050"
)

func TestGRPCScenarios(t *testing.T) {
	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTP2Monitoring = true

	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http.MinimumKernelVersion {
		t.Skipf("USM can not run on kernel before %v", http.MinimumKernelVersion)
	}

	if currKernelVersion < http.HTTP2MinimumKernelVersion {
		t.Skipf("HTTP2 monitoring can not run on kernel before %v", http.HTTP2MinimumKernelVersion)
	}

	s, err := grpc.NewServer(srvAddr)
	require.NoError(t, err)
	s.Run()
	t.Cleanup(s.Stop)

	// c is a stream endpoint
	// a + b are unary endpoints
	tests := []struct {
		name              string
		runClients        func(t *testing.T, differentClients bool)
		expectedEndpoints map[http.Key]int
		expectedError     bool
	}{
		{
			name: "simple unary - multiple requests",
			runClients: func(t *testing.T, _ bool) {
				var client1 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)

				ctx := context.Background()
				for i := 0; i < 1000; i++ {
					require.NoError(t, client1.HandleUnary(ctx, "first"))
				}
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 1000,
			},
			expectedError: false,
		},
		{
			name: "unary, a->a->a",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, "first"))
				require.NoError(t, client2.HandleUnary(ctx, "second"))
				require.NoError(t, client1.HandleUnary(ctx, "third"))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 3,
			},
			expectedError: false,
		},
		{
			name: "unary, a->b->a",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, "first"))
				require.NoError(t, client2.GetFeature(ctx, -746143763, 407838351))
				require.NoError(t, client1.HandleUnary(ctx, "first"))
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
			expectedError: false,
		},
		{
			name: "unary, a->b->a->b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, "first"))
				require.NoError(t, client2.GetFeature(ctx, -746143763, 407838351))
				require.NoError(t, client1.HandleUnary(ctx, "third"))
				require.NoError(t, client2.GetFeature(ctx, -743999179, 408122808))
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
			expectedError: false,
		},
		{
			name: "unary, a->b->b->a",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, "first"))
				require.NoError(t, client2.GetFeature(ctx, -746143763, 407838351))
				require.NoError(t, client1.GetFeature(ctx, -743999179, 408122808))
				require.NoError(t, client2.HandleUnary(ctx, "third"))
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
			expectedError: false,
		},
		{
			name: "unary, a->b->b->a",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, "first"))
				require.NoError(t, client2.GetFeature(ctx, -746143763, 407838351))
				require.NoError(t, client2.GetFeature(ctx, -743999179, 408122808))
				require.NoError(t, client1.HandleUnary(ctx, "third"))
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
			expectedError: false,
		},
		{
			name: "stream, c",
			runClients: func(t *testing.T, differentClients bool) {
				var client1 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)

				ctx := context.Background()
				require.NoError(t, client1.HandleStream(ctx, 10))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/protobuf.Math/Max"},
					Method: http.MethodPost,
				}: 1,
			},
			expectedError: false,
		},
		{
			name: "stream, c->c->c",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleStream(ctx, 10))
				require.NoError(t, client2.HandleStream(ctx, 10))
				require.NoError(t, client1.HandleStream(ctx, 10))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/protobuf.Math/Max"},
					Method: http.MethodPost,
				}: 3,
			},
			expectedError: false,
		},
		{
			name: "mixed, c->b->c->b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleStream(ctx, 10))
				require.NoError(t, client2.HandleUnary(ctx, "first"))
				require.NoError(t, client1.HandleStream(ctx, 10))
				require.NoError(t, client2.HandleUnary(ctx, "second"))
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
			expectedError: false,
		},
		{
			name: "mixed, c->b->c->b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctx := context.Background()
				require.NoError(t, client1.HandleStream(ctx, 10))
				require.NoError(t, client1.HandleUnary(ctx, "first"))
				require.NoError(t, client2.HandleStream(ctx, 10))
				require.NoError(t, client2.HandleUnary(ctx, "second"))
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
			expectedError: false,
		},
		{
			name: "request with large body (1MB)",
			runClients: func(t *testing.T, differentClients bool) {
				var client1 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)

				longName := strings.Repeat("1", 1024*1024)
				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, longName))
			},
			expectedEndpoints: map[http.Key]int{
				{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 1,
			},
			expectedError: true,
		},

		{
			name: "request with large body (1MB) -> b -> request with large body (1MB) -> b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				longName := strings.Repeat("1", 1024*1024)
				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, longName))
				require.NoError(t, client2.GetFeature(ctx, -746143763, 407838351))
				require.NoError(t, client1.HandleUnary(ctx, longName))
				require.NoError(t, client2.GetFeature(ctx, -743999179, 408122808))
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
			name: "request with large body (1MB) -> b -> request with large body (1MB) -> b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				longName := strings.Repeat("1", 1024*1024)
				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, longName))
				require.NoError(t, client2.GetFeature(ctx, -746143763, 407838351))
				require.NoError(t, client2.HandleUnary(ctx, longName))
				require.NoError(t, client1.GetFeature(ctx, -743999179, 408122808))
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
			name: "500 headers -> b -> 500 headers -> b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 500)
				for i := 1; i <= 500; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value-%d", i)
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := strings.Repeat("1", 1024*1024)
				require.NoError(t, client1.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client2.GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, client1.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client2.GetFeature(ctxWithoutHeaders, -743999179, 408122808))
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
			name: "500 headers -> b -> 500 headers -> b",
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 500)
				for i := 1; i <= 500; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value-%d", i)
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := strings.Repeat("1", 1024*1024)
				require.NoError(t, client1.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client2.GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, client2.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client1.GetFeature(ctxWithoutHeaders, -743999179, 408122808))
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
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 20)
				for i := 1; i <= 20; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value")
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := strings.Repeat("1", 1024*1024)
				require.NoError(t, client1.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client2.GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, client1.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client2.GetFeature(ctxWithoutHeaders, -743999179, 408122808))
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
			runClients: func(t *testing.T, differentClients bool) {
				var client1, client2 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)
				client2 = client1
				if differentClients {
					client2, err = grpc.NewClient(srvAddr, grpc.Options{})
					require.NoError(t, err)
				}

				ctxWithoutHeaders := context.Background()
				ctxWithHeaders := context.Background()
				headers := make(map[string]string, 20)
				for i := 1; i <= 20; i++ {
					headers[fmt.Sprintf("header-%d", i)] = fmt.Sprintf("value")
				}
				md := metadata.New(headers)
				ctxWithHeaders = metadata.NewOutgoingContext(ctxWithHeaders, md)
				longName := strings.Repeat("1", 1024*1024)
				require.NoError(t, client1.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client2.GetFeature(ctxWithoutHeaders, -746143763, 407838351))
				require.NoError(t, client2.HandleUnary(ctxWithHeaders, longName))
				require.NoError(t, client1.GetFeature(ctxWithoutHeaders, -743999179, 408122808))
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
		for _, val := range []bool{false, true} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", val)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				// we are currently not supporting some edge cases:
				// https://datadoghq.atlassian.net/browse/USMO-222
				if tt.expectedError {
					t.Skip("Skipping test due to known issue")
				}

				monitor, err := http.NewMonitor(cfg, nil, nil, nil)
				require.NoError(t, err)
				require.NoError(t, monitor.Start())
				defer monitor.Stop()

				tt.runClients(t, val)

				res := make(map[http.Key]int)
				require.Eventually(t, func() bool {
					stats := monitor.GetHTTP2Stats()
					for key, stat := range stats {
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
