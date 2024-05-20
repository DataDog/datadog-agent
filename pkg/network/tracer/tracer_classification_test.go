// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)

// Package tracer contains implementation for NPM's tracer.
package tracer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/amqp"
	usmhttp2 "github.com/DataDog/datadog-agent/pkg/network/protocols/http2"
	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil/grpc"
)

const (
	defaultTimeout      = 30 * time.Second
	http2DefaultTimeout = 3 * time.Second
)

// testContext shares the context of a given test.
// It contains common variable used by all tests, and allows extending the context dynamically by setting more
// attributes to the `extras` map.
type testContext struct {
	// The address of the server to listen on.
	serverAddress string
	// The port to listen on.
	serverPort string
	// The address for the client to communicate with.
	targetAddress string
	// A dynamic map that allows extending the context easily between phases of the test.
	extras map[string]interface{}
}

// protocolClassificationAttributes holds all attributes a single protocol classification test should have.
type protocolClassificationAttributes struct {
	// The name of the test.
	name string
	// Specific test context, allows to share states among different phases of the test.
	context testContext
	// Allows to decide on runtime if we should skip the test or not.
	skipCallback func(t *testing.T, ctx testContext)
	// Allows to do any preparation without traffic being captured by the tracer.
	preTracerSetup func(t *testing.T, ctx testContext)
	// All traffic here will be captured by the tracer.
	postTracerSetup func(t *testing.T, ctx testContext)
	// A validation method ensure the test succeeded.
	validation func(t *testing.T, ctx testContext, tr *Tracer)
	// Cleaning test resources if needed.
	teardown func(t *testing.T, ctx testContext)
}

func validateProtocolConnection(expectedStack *protocols.Stack) func(t *testing.T, ctx testContext, tr *Tracer) {
	return func(t *testing.T, ctx testContext, tr *Tracer) {
		waitForConnectionsWithProtocol(t, tr, ctx.targetAddress, ctx.serverAddress, expectedStack)
	}
}

// skipIfNotLinux skips the test if we are not on a linux machine
func skipIfNotLinux(t *testing.T, _ testContext) {
	if runtime.GOOS != "linux" {
		t.Skip("test is supported on linux machine only")
	}
}

// skipIfUsingNAT skips the test if we have a NAT rules applied.
func skipIfUsingNAT(t *testing.T, ctx testContext) {
	if ctx.targetAddress != ctx.serverAddress {
		t.Skip("test is not supported when NAT is applied")
	}
}

// composeSkips skips if one of the given filters is matched.
func composeSkips(skippers ...func(t *testing.T, ctx testContext)) func(t *testing.T, ctx testContext) {
	return func(t *testing.T, ctx testContext) {
		for _, skipFunction := range skippers {
			skipFunction(t, ctx)
		}
	}
}

const (
	amqpPort       = "5672"
	httpPort       = "8080"
	httpsPort      = "8443"
	http2Port      = "9090"
	grpcPort       = "9091"
	rawTrafficPort = "9093"
)

func testProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string)
	}{
		{
			name:     "kafka",
			testFunc: testKafkaProtocolClassification,
		},
		{
			name:     "mysql",
			testFunc: testMySQLProtocolClassification,
		},
		{
			name:     "postgres",
			testFunc: testPostgresProtocolClassification,
		},
		{
			name:     "mongo",
			testFunc: testMongoProtocolClassification,
		},
		{
			name:     "redis",
			testFunc: testRedisProtocolClassification,
		},
		{
			name:     "amqp",
			testFunc: testAMQPProtocolClassification,
		},
		{
			name:     "http",
			testFunc: testHTTPProtocolClassification,
		},
		{
			name:     "http2",
			testFunc: testHTTP2ProtocolClassification,
		},
		{
			name:     "edge cases",
			testFunc: testEdgeCasesProtocolClassification,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.testFunc(t, tr, clientHost, targetHost, serverHost)
		})
	}
}

func testAMQPProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipFunc := composeSkips(skipIfNotLinux, skipIfUsingNAT)
	skipFunc(t, testContext{
		serverAddress: serverHost,
		serverPort:    amqpPort,
		targetAddress: targetHost,
	})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	amqpTeardown := func(t *testing.T, ctx testContext) {
		client := ctx.extras["client"].(*amqp.Client)
		defer client.Terminate()

		require.NoError(t, client.DeleteQueues())
	}

	// Setting one instance of amqp server for all tests.
	serverAddress := net.JoinHostPort(serverHost, amqpPort)
	targetAddress := net.JoinHostPort(targetHost, amqpPort)
	require.NoError(t, amqp.RunServer(t, serverHost, amqpPort))

	tests := []protocolClassificationAttributes{
		{
			name: "connect",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.AMQP}),
		},
		{
			name: "declare channel",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(&protocols.Stack{}),
		},
		{
			name: "publish",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				require.NoError(t, client.Publish("test", "my msg"))
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.AMQP}),
		},
		{
			name: "consume",
			context: testContext{
				serverPort:    amqpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				client, err := amqp.NewClient(amqp.Options{
					ServerAddress: ctx.serverAddress,
					Dialer:        defaultDialer,
				})
				require.NoError(t, err)
				require.NoError(t, client.DeclareQueue("test", client.PublishChannel))
				require.NoError(t, client.DeclareQueue("test", client.ConsumeChannel))
				require.NoError(t, client.Publish("test", "my msg"))
				ctx.extras["client"] = client
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := ctx.extras["client"].(*amqp.Client)
				res, err := client.Consume("test", 1)
				require.NoError(t, err)
				require.Equal(t, []string{"my msg"}, res)
			},
			teardown:   amqpTeardown,
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.AMQP}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testHTTPProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	serverAddress := net.JoinHostPort(serverHost, httpPort)
	targetAddress := net.JoinHostPort(targetHost, httpPort)
	tests := []protocolClassificationAttributes{
		{
			name: "tcp client with sending HTTP request",
			context: testContext{
				serverPort:    httpPort,
				serverAddress: serverAddress,
				targetAddress: targetAddress,
				extras:        make(map[string]interface{}),
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				ln, err := net.Listen("tcp", ctx.serverAddress)
				require.NoError(t, err)

				srv := &nethttp.Server{
					Addr: ln.Addr().String(),
					Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
						io.Copy(io.Discard, req.Body)
						w.WriteHeader(200)
					}),
					ReadTimeout:  time.Second,
					WriteTimeout: time.Second,
				}
				// Temporary change, without it the protocol classification is flaky.
				srv.SetKeepAlivesEnabled(true)
				go func() {
					_ = srv.Serve(ln)
				}()

				ctx.extras["server"] = srv
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := nethttp.Client{
					Transport: &nethttp.Transport{
						DialContext: defaultDialer.DialContext,
					},
				}
				resp, err := client.Get("http://" + ctx.targetAddress + "/test")
				require.NoError(t, err)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			},
			teardown: func(t *testing.T, ctx testContext) {
				srv := ctx.extras["server"].(*nethttp.Server)
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				_ = srv.Shutdown(timedContext)
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testHTTP2ProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	skipIfNotLinux(t, testContext{})

	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP(clientHost),
			Port: 0,
		},
	}

	// http2 server init
	http2ServerAddress := net.JoinHostPort(serverHost, http2Port)
	http2TargetAddress := net.JoinHostPort(targetHost, http2Port)
	http2Server := &nethttp.Server{
		Addr: ":" + http2Port,
		Handler: h2c.NewHandler(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			w.WriteHeader(200)
			w.Write([]byte("test"))
		}), &http2.Server{}),
	}

	go func() {
		if err := http2Server.ListenAndServe(); err != nethttp.ErrServerClosed {
			require.NoError(t, err, "could not serve")
		}
	}()
	t.Cleanup(func() {
		http2Server.Close()
	})

	// gRPC server init
	grpcServerAddress := net.JoinHostPort(serverHost, grpcPort)
	grpcTargetAddress := net.JoinHostPort(targetHost, grpcPort)

	grpcServer, err := grpc.NewServer(grpcServerAddress, false)
	require.NoError(t, err)
	grpcServer.Run()
	t.Cleanup(grpcServer.Stop)

	grpcContext := testContext{
		serverPort:    grpcPort,
		serverAddress: grpcServerAddress,
		targetAddress: grpcTargetAddress,
	}

	tests := []protocolClassificationAttributes{
		{
			name: "http2 traffic without grpc",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: http2ServerAddress,
				targetAddress: http2TargetAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := &nethttp.Client{
					Transport: &http2.Transport{
						AllowHTTP: true,
						DialTLSContext: func(_ context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
							return net.Dial(network, addr)
						},
					},
				}

				resp, err := client.Post("http://"+ctx.targetAddress, "application/json", bytes.NewReader([]byte("test")))
				require.NoError(t, err)

				resp.Body.Close()
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2}),
		},
		{
			name:    "http2 traffic using gRPC - unary call",
			context: grpcContext,
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := grpc.NewClient(ctx.targetAddress, grpc.Options{
					CustomDialer: defaultDialer,
				}, false)
				require.NoError(t, err)
				defer c.Close()
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, c.HandleUnary(timedContext, "test"))
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
		{
			name:    "http2 traffic using gRPC - stream call",
			context: grpcContext,
			postTracerSetup: func(t *testing.T, ctx testContext) {
				c, err := grpc.NewClient(ctx.targetAddress, grpc.Options{
					CustomDialer: defaultDialer,
				}, false)
				require.NoError(t, err)
				defer c.Close()
				timedContext, cancel := context.WithTimeout(context.Background(), defaultTimeout)
				defer cancel()
				require.NoError(t, c.HandleStream(timedContext, 5))
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
		{
			// This test checks if the classifier can properly skip literal
			// headers that are not useful to determine if gRPC is used.
			name: "http2 traffic using gRPC - irrelevant literal headers",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: http2ServerAddress,
				targetAddress: http2TargetAddress,
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				client := &nethttp.Client{
					Transport: &http2.Transport{
						AllowHTTP: true,
						DialTLSContext: func(_ context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
							return net.Dial(network, addr)
						},
					},
				}

				req, err := nethttp.NewRequest("POST", "http://"+ctx.targetAddress, bytes.NewReader([]byte("test")))
				require.NoError(t, err)

				// Add some literal headers that needs to be skipped by the
				// classifier. Also adding a grpc content-type to emulate grpc
				// traffic
				req.Header.Add("someheader", "somevalue")
				req.Header.Add("Content-type", "application/grpc")
				req.Header.Add("someotherheader", "someothervalue")

				resp, err := client.Do(req)
				require.NoError(t, err)

				resp.Body.Close()
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
		{
			// This test checks that we are not classifying a connection as
			// gRPC traffic without a prior classification as HTTP2.
			name: "GRPC without prior HTTP2 classification",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: net.JoinHostPort(serverHost, rawTrafficPort),
				targetAddress: net.JoinHostPort(serverHost, rawTrafficPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				skipIfNotLinux(t, ctx)

				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					io.Copy(c, c)
					c.Close()
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// The gRPC classification is based on having only POST requests,
				// and having "application/grpc" as a content-type.
				var testHeaderFields = []hpack.HeaderField{
					{Name: ":authority", Value: "127.0.0.0.1:" + rawTrafficPort},
					{Name: ":method", Value: "POST"},
					{Name: ":path", Value: "/aaa"},
					{Name: ":scheme", Value: "http"},
					{Name: "content-type", Value: "application/grpc"},
					{Name: "content-length", Value: "0"},
					{Name: "accept-encoding", Value: "gzip"},
					{Name: "user-agent", Value: "Go-http-client/2.0"},
				}

				buf := new(bytes.Buffer)
				framer := http2.NewFramer(buf, nil)
				rawHdrs, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{Headers: testHeaderFields})
				require.NoError(t, err)

				// Writing the header frames to the buffer using the Framer.
				require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      uint32(1),
					BlockFragment: rawHdrs,
					EndStream:     true,
					EndHeaders:    true,
				}))

				c, err := net.Dial("tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
				_, err = c.Write(buf.Bytes())
				require.NoError(t, err)
			},
			teardown: func(t *testing.T, ctx testContext) {
				ctx.extras["server"].(*TCPServer).Shutdown()
			},
			validation: validateProtocolConnection(&protocols.Stack{}),
		},
		{
			name: "GRPC with prior HTTP2 classification",
			context: testContext{
				serverPort:    http2Port,
				serverAddress: net.JoinHostPort(serverHost, rawTrafficPort),
				targetAddress: net.JoinHostPort(targetHost, rawTrafficPort),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				skipIfNotLinux(t, ctx)

				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					io.Copy(c, c)
					c.Close()
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				// The gRPC classification is based on having only POST requests,
				// and having "application/grpc" as a content-type.
				var testHeaderFields = []hpack.HeaderField{
					{Name: ":authority", Value: "127.0.0.0.1:" + rawTrafficPort},
					{Name: ":method", Value: "POST"},
					{Name: ":path", Value: "/aaa"},
					{Name: ":scheme", Value: "http"},
					{Name: "content-type", Value: "application/grpc"},
					{Name: "content-length", Value: "0"},
					{Name: "accept-encoding", Value: "gzip"},
					{Name: "user-agent", Value: "Go-http-client/2.0"},
				}

				buf := new(bytes.Buffer)
				framer := http2.NewFramer(buf, nil)

				// Initiate a connection to the TCP server.
				c, err := net.Dial("tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()

				// Writing a magic and the settings in the same packet to socket.
				_, err = c.Write(usmhttp2.ComposeMessage([]byte(http2.ClientPreface), buf.Bytes()))
				require.NoError(t, err)
				buf.Reset()
				c.SetReadDeadline(time.Now().Add(http2DefaultTimeout))
				frameReader := http2.NewFramer(nil, c)
				for {
					_, err := frameReader.ReadFrame()
					if err != nil {
						break
					}
				}

				rawHdrs, err := usmhttp2.NewHeadersFrameMessage(usmhttp2.HeadersFrameOptions{Headers: testHeaderFields})
				require.NoError(t, err)

				// Writing the header frames to the buffer using the Framer.
				require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{
					StreamID:      uint32(1),
					BlockFragment: rawHdrs,
					EndStream:     true,
					EndHeaders:    true,
				}))

				_, err = c.Write(buf.Bytes())
				require.NoError(t, err)
				c.SetReadDeadline(time.Now().Add(http2DefaultTimeout))
				frameReader = http2.NewFramer(nil, c)
				for {
					_, err := frameReader.ReadFrame()
					if err != nil {
						break
					}
				}
			},
			teardown: func(t *testing.T, ctx testContext) {
				ctx.extras["server"].(*TCPServer).Shutdown()
			},
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP2, API: protocols.GRPC}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func testEdgeCasesProtocolClassification(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	defaultDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: net.ParseIP(clientHost),
		},
	}

	teardown := func(t *testing.T, ctx testContext) {
		server, ok := ctx.extras["server"].(*TCPServer)
		if ok {
			server.Shutdown()
		}
	}

	tests := []protocolClassificationAttributes{
		{
			name: "tcp client without sending data",
			context: testContext{
				serverPort:    "10001",
				serverAddress: net.JoinHostPort(serverHost, "10001"),
				targetAddress: net.JoinHostPort(targetHost, "10001"),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					c.Close()
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
			},
			teardown:   teardown,
			validation: validateProtocolConnection(&protocols.Stack{}),
		},
		{
			name: "tcp client with sending random data",
			context: testContext{
				serverPort:    "10002",
				serverAddress: net.JoinHostPort(serverHost, "10002"),
				targetAddress: net.JoinHostPort(targetHost, "10002"),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					defer c.Close()
					r := bufio.NewReader(c)
					input, err := r.ReadBytes(byte('\n'))
					if err == nil {
						c.Write(input)
					}
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
				c.Write([]byte("hello\n"))
				io.Copy(io.Discard, c)
			},
			teardown:   teardown,
			validation: validateProtocolConnection(&protocols.Stack{}),
		},
		{
			// A case where we see multiple protocols on the same socket. In that case, we expect to classify the connection
			// with the first protocol we've found.
			name: "mixed protocols",
			context: testContext{
				serverPort:    "10003",
				serverAddress: net.JoinHostPort(serverHost, "10003"),
				targetAddress: net.JoinHostPort(targetHost, "10003"),
				extras:        map[string]interface{}{},
			},
			preTracerSetup: func(t *testing.T, ctx testContext) {
				server := NewTCPServerOnAddress(ctx.serverAddress, func(c net.Conn) {
					defer c.Close()

					r := bufio.NewReader(c)
					for {
						// The server reads up to a marker, in our case `$`.
						input, err := r.ReadBytes(byte('$'))
						if err != nil {
							return
						}
						c.Write(input)
					}
				})
				ctx.extras["server"] = server
				require.NoError(t, server.Run())
			},
			postTracerSetup: func(t *testing.T, ctx testContext) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, err := defaultDialer.DialContext(timedContext, "tcp", ctx.targetAddress)
				require.NoError(t, err)
				defer c.Close()
				// The server reads up to a marker, in our case `$`.
				httpInput := []byte("GET /200/foobar HTTP/1.1\n$")
				http2Input := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n$")
				for _, input := range [][]byte{httpInput, http2Input} {
					// Calling write multiple times to increase chances for classification.
					_, err = c.Write(input)
					require.NoError(t, err)

					// This is an echo server, we expect to get the same buffer as we sent, so the output buffer
					// has the same length of the input.
					output := make([]byte, len(input))
					_, err = c.Read(output)
					require.NoError(t, err)
					require.Equal(t, input, output)
				}
			},
			teardown:   teardown,
			validation: validateProtocolConnection(&protocols.Stack{Application: protocols.HTTP}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProtocolClassificationInner(t, tt, tr)
		})
	}
}

func waitForConnectionsWithProtocol(t *testing.T, tr *Tracer, targetAddr, serverAddr string, expectedStack *protocols.Stack) {
	t.Logf("looking for target addr %s", targetAddr)
	t.Logf("looking for server addr %s", serverAddr)
	var outgoing, incoming *network.ConnectionStats
	failed := !assert.Eventually(t, func() bool {
		conns := getConnections(t, tr)
		if outgoing == nil {
			for _, c := range searchConnections(conns, func(cs network.ConnectionStats) bool {
				return cs.Direction == network.OUTGOING && cs.Type == network.TCP && fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == targetAddr
			}) {
				t.Logf("found potential outgoing connection %+v", c)
				if assertProtocolStack(t, &c.ProtocolStack, expectedStack) {
					t.Logf("found outgoing connection %+v", c)
					outgoing = &c
					break
				}
			}
		}

		if incoming == nil {
			for _, c := range searchConnections(conns, func(cs network.ConnectionStats) bool {
				return cs.Direction == network.INCOMING && cs.Type == network.TCP && fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == serverAddr
			}) {
				t.Logf("found potential incoming connection %+v", c)
				if assertProtocolStack(t, &c.ProtocolStack, expectedStack) {
					t.Logf("found incoming connection %+v", c)
					incoming = &c
					break
				}
			}
		}

		failed := incoming == nil || outgoing == nil
		if failed {
			t.Log(conns)
		}
		return !failed
	}, 5*time.Second, 100*time.Millisecond, "could not find incoming or outgoing connections")
	if failed {
		t.Logf("incoming=%+v outgoing=%+v", incoming, outgoing)
	}
}

func assertProtocolStack(t *testing.T, stack, expectedStack *protocols.Stack) bool {
	t.Helper()

	return reflect.DeepEqual(stack, expectedStack)
}
