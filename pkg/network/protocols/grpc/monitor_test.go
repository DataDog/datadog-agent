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
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
)

const (
	srvAddr = "127.0.0.1:5050"
)

// c is a stream endpoint
// a + b are unary endpoints
func TestGRPCScenarios(t *testing.T) {
	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableRuntimeCompiler = false
	cfg.EnableCORE = false
	cfg.BPFDebug = true

	s, err := grpc.NewServer(srvAddr)
	require.NoError(t, err)
	s.Run()
	t.Cleanup(s.Stop)

	tests := []struct {
		name              string
		runClients        func(t *testing.T, differentClients bool)
		expectedEndpoints map[http.Key]int
	}{
		{
			name: "aaaa simple unary",
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
				}: 1,
			},
		},
		{
			name: "guyunary, a->a->a",
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
		},
		{
			name: "amitrequest with large body (1MB)",
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
		},
	}
	for _, tt := range tests {
		for _, val := range []bool{false} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", val)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				monitor, err := http.NewMonitor(cfg, nil, nil, nil)
				require.NoError(t, err)
				require.NoError(t, monitor.Start())
				defer monitor.Stop()

				tt.runClients(t, val)

				res := make(map[http.Key]int)
				require.Eventually(t, func() bool {
					stats := monitor.GetHTTPStats()
					for key, stat := range stats {
						if key.DstPort == 5050 || key.SrcPort == 5050 {
							count := 0
							if stat.HasStats(200) {
								count += stat.Stats(200).Count
							}
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
