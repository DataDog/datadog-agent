// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)

// Package tracer contains implementation for NPM's tracer.
package tracer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

const (
	httpPort = "8080"
)

func testProtocolClassificationCrossOS(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T, tr *Tracer, clientHost, targetHost, serverHost string)
	}{
		{
			name:     "http",
			testFunc: testHTTPProtocolClassification,
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
