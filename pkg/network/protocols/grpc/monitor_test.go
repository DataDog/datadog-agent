// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package grpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	nethttp "net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	srvAddr = "127.0.0.1:5050"
)

func skipTestIfKernelNotSupported(t *testing.T) {
	currKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if currKernelVersion < http.MinimumKernelVersion {
		t.Skip(fmt.Sprintf("gRPC feature not available on pre %s kernels", http.MinimumKernelVersion.String()))
	}
}

// c is a stream endpoint
// a + b are unary endpoints
func TestGRPCScenarios(t *testing.T) {
	tests := []struct {
		name              string
		runClients        func(t *testing.T, differentClients bool)
		expectedEndpoints map[http.Key]int
	}{
		{
			name: "simple unary",
			runClients: func(t *testing.T, _ bool) {
				var client1 grpc.Client
				var err error
				client1, err = grpc.NewClient(srvAddr, grpc.Options{})
				require.NoError(t, err)

				ctx := context.Background()
				require.NoError(t, client1.HandleUnary(ctx, "first"))
			},
			expectedEndpoints: map[http.Key]int{
				http.Key{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 1,
			},
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
				http.Key{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 3,
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
				http.Key{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
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
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/protobuf.Math/Max"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/protobuf.Math/Max"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				require.NoError(t, client1.HandleUnary(ctx, longName))
				require.NoError(t, client2.GetFeature(ctx, -743999179, 408122808))
			},
			expectedEndpoints: map[http.Key]int{
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
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
				http.Key{
					Path:   http.Path{Content: "/routeguide.RouteGuide/GetFeature"},
					Method: http.MethodPost,
				}: 2,
				http.Key{
					Path:   http.Path{Content: "/helloworld.Greeter/SayHello"},
					Method: http.MethodPost,
				}: 2,
			},
		},
	}
	cfg := config.New()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableRuntimeCompiler = true
	for _, tt := range tests {
		for _, val := range []bool{false} {
			testNameSuffix := fmt.Sprintf("-different clients - %v", val)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				s, err := grpc.NewServer(srvAddr)
				require.NoError(t, err)
				s.Run()
				t.Cleanup(s.Stop)

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

func assertAllRequestsExists(t *testing.T, monitor *http.Monitor, requests []*nethttp.Request) {
	requestsExist := make([]bool, len(requests))
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		stats := monitor.GetHTTPStats()
		for reqIndex, req := range requests {
			included, err := isRequestIncludedOnce(stats, req)
			require.NoError(t, err)
			requestsExist[reqIndex] = requestsExist[reqIndex] || included
		}
	}

	for reqIndex, exists := range requestsExist {
		require.Truef(t, exists, "request %d was not found (req %v)", reqIndex, requests[reqIndex])
	}
}

var (
	httpMethods         = []string{nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete, nethttp.MethodOptions}
	httpMethodsWithBody = []string{nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete}
	statusCodes         = []int{nethttp.StatusOK, nethttp.StatusMultipleChoices, nethttp.StatusBadRequest, nethttp.StatusInternalServerError}
)

func requestGenerator(t *testing.T, targetAddr string, reqBody []byte) func() *nethttp.Request {
	var (
		random = rand.New(rand.NewSource(time.Now().Unix()))
		idx    = 0
		client = new(nethttp.Client)
	)

	return func() *nethttp.Request {
		idx++
		var method string
		var body io.Reader
		var finalBody []byte
		if len(reqBody) > 0 {
			finalBody = append([]byte(strings.Repeat(" ", idx)), reqBody...)
			body = bytes.NewReader(finalBody)
			method = httpMethodsWithBody[random.Intn(len(httpMethodsWithBody))]
		} else {
			method = httpMethods[random.Intn(len(httpMethods))]
		}
		status := statusCodes[random.Intn(len(statusCodes))]
		url := fmt.Sprintf("http://%s/%d/request-%d", targetAddr, status, idx)
		req, err := nethttp.NewRequest(method, url, body)
		require.NoError(t, err)

		resp, err := client.Do(req)
		if strings.Contains(targetAddr, "ignore") {
			return req
		}
		require.NoError(t, err)
		if len(reqBody) > 0 {
			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, finalBody, respBody)
		}
		return req
	}
}

func includesRequest(t *testing.T, allStats map[http.Key]*http.RequestStats, req *nethttp.Request) {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	included, err := isRequestIncludedOnce(allStats, req)
	require.NoError(t, err)
	if !included {
		t.Errorf(
			"could not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
			req.URL.Path,
			req.Method,
			expectedStatus,
		)
	}
}

func requestNotIncluded(t *testing.T, allStats map[http.Key]*http.RequestStats, req *nethttp.Request) {
	included, err := isRequestIncludedOnce(allStats, req)
	require.NoError(t, err)
	if included {
		expectedStatus := testutil.StatusFromPath(req.URL.Path)
		t.Errorf(
			"should not find HTTP transaction matching the following criteria:\n path=%s method=%s status=%d",
			req.URL.Path,
			req.Method,
			expectedStatus,
		)
	}
}

func isRequestIncludedOnce(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) (bool, error) {
	occurrences := countRequestOccurrences(allStats, req)

	if occurrences == 1 {
		return true, nil
	} else if occurrences == 0 {
		return false, nil
	}
	return false, fmt.Errorf("expected to find 1 occurrence of %v, but found %d instead", req, occurrences)
}

func countRequestOccurrences(allStats map[http.Key]*http.RequestStats, req *nethttp.Request) int {
	expectedStatus := testutil.StatusFromPath(req.URL.Path)
	occurrences := 0
	for key, stats := range allStats {
		if key.Path.Content == req.URL.Path && stats.HasStats(expectedStatus) {
			occurrences++
		}
	}

	return occurrences
}
