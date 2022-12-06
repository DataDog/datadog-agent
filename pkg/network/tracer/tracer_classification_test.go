// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)
// +build linux_bpf windows,npm

package tracer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
	"github.com/stretchr/testify/require"
)

func testProtocolClassification(t *testing.T, cfg *config.Config, clientHost, targetHost, serverHost string) {
	tr, err := NewTracer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	initTracerState(t, tr)
	require.NoError(t, err)

	// Giving the tracer time to run
	time.Sleep(time.Second)

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   net.ParseIP(clientHost),
			Port: 0,
		},
	}

	tests := []struct {
		name      string
		clientRun func(t *testing.T, serverAddr string)
		serverRun func(t *testing.T, serverAddr string, done chan struct{}) string
		want      network.ProtocolType
	}{
		{
			name: "tcp client without sending data",
			clientRun: func(t *testing.T, serverAddr string) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				c, err := dialer.DialContext(timedContext, "tcp", serverAddr)
				cancel()
				require.NoError(t, err)
				defer c.Close()
			},
			serverRun: func(t *testing.T, serverAddr string, done chan struct{}) string {
				server := NewTCPServerOnAddress(serverAddr, func(c net.Conn) {
					c.Close()
				})
				require.NoError(t, server.Run(done))
				return server.address
			},
			want: network.ProtocolUnknown,
		},
		{
			name: "tcp client with sending random data",
			clientRun: func(t *testing.T, serverAddr string) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				c, err := dialer.DialContext(timedContext, "tcp", serverAddr)
				cancel()
				require.NoError(t, err)
				defer c.Close()
				c.Write([]byte("hello\n"))
				io.ReadAll(c)
			},
			serverRun: func(t *testing.T, serverAddr string, done chan struct{}) string {
				server := NewTCPServerOnAddress(serverAddr, func(c net.Conn) {
					r := bufio.NewReader(c)
					input, err := r.ReadBytes(byte('\n'))
					if err == nil {
						c.Write(input)
					}
					c.Close()
				})
				require.NoError(t, server.Run(done))
				return server.address
			},
			want: network.ProtocolUnknown,
		},
		{
			name: "tcp client with sending HTTP request",
			clientRun: func(t *testing.T, serverAddr string) {
				client := nethttp.Client{
					Transport: &nethttp.Transport{
						DialContext: dialer.DialContext,
					},
				}
				resp, err := client.Get("http://" + serverAddr + "/test")
				require.NoError(t, err)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			},
			serverRun: func(t *testing.T, serverAddr string, done chan struct{}) string {
				ln, err := net.Listen("tcp", serverAddr)
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
				srv.SetKeepAlivesEnabled(false)
				go func() {
					_ = srv.Serve(ln)
				}()
				go func() {
					<-done
					srv.Shutdown(context.Background())
				}()
				return srv.Addr
			},
			want: network.ProtocolHTTP,
		},
		{
			name: "gRPC traffic - unary call",
			clientRun: func(t *testing.T, serverAddr string) {
				c, err := grpc.NewClient(serverAddr, grpc.Options{
					CustomDialer: dialer,
				})
				require.NoError(t, err)
				defer c.Close()
				require.NoError(t, c.HandleUnary(context.Background(), "test"))
			},
			serverRun: func(t *testing.T, serverAddr string, done chan struct{}) string {
				server, err := grpc.NewServer(serverAddr)
				require.NoError(t, err)
				server.Run()
				go func() {
					<-done
					server.Stop()
				}()
				return server.Address
			},
			want: network.ProtocolHTTP2,
		},
		{
			name: "gRPC traffic - stream call",
			clientRun: func(t *testing.T, serverAddr string) {
				c, err := grpc.NewClient(serverAddr, grpc.Options{
					CustomDialer: dialer,
				})
				require.NoError(t, err)
				defer c.Close()
				require.NoError(t, c.HandleStream(context.Background(), 5))
			},
			serverRun: func(t *testing.T, serverAddr string, done chan struct{}) string {
				server, err := grpc.NewServer(serverAddr)
				require.NoError(t, err)
				server.Run()
				go func() {
					<-done
					server.Stop()
				}()
				return server.Address
			},
			want: network.ProtocolHTTP2,
		},
		{
			// A case where we see multiple protocols on the same socket. In that case, we expect to classify the connection
			// with the first protocol we've found.
			name: "mixed protocols",
			clientRun: func(t *testing.T, serverAddr string) {
				timedContext, cancel := context.WithTimeout(context.Background(), time.Second)
				c, err := dialer.DialContext(timedContext, "tcp", serverAddr)
				cancel()
				require.NoError(t, err)
				defer c.Close()
				c.Write([]byte("GET /200/foobar HTTP/1.1\n"))
				io.ReadAll(c)
				// http2 prefix.
				c.Write([]byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"))
				io.ReadAll(c)
			},
			serverRun: func(t *testing.T, serverAddr string, done chan struct{}) string {
				server := NewTCPServerOnAddress(serverAddr, func(c net.Conn) {
					r := bufio.NewReader(c)
					input, err := r.ReadBytes(byte('\n'))
					if err == nil {
						c.Write(input)
					}
					c.Close()
				})
				require.NoError(t, server.Run(done))
				return server.address
			},
			want: network.ProtocolHTTP,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			defer close(done)
			serverAddr := tt.serverRun(t, serverHost, done)
			_, port, err := net.SplitHostPort(serverAddr)
			require.NoError(t, err)
			targetAddr := net.JoinHostPort(targetHost, port)

			// Letting the server time to start
			time.Sleep(500 * time.Millisecond)
			tt.clientRun(t, targetAddr)

			waitForConnectionsWithProtocol(t, tr, targetAddr, serverAddr, tt.want)
		})
	}
}

func waitForConnectionsWithProtocol(t *testing.T, tr *Tracer, targetAddr, serverAddr string, expectedProtocol network.ProtocolType) {
	var incomingConns, outgoingConns []network.ConnectionStats

	foundIncomingWithProtocol := false
	foundOutgoingWithProtocol := false

	for start := time.Now(); time.Since(start) < 5*time.Second; {
		conns := getConnections(t, tr)
		newOutgoingConns := searchConnections(conns, func(cs network.ConnectionStats) bool {
			return cs.Type == network.TCP && fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == targetAddr
		})
		newIncomingConns := searchConnections(conns, func(cs network.ConnectionStats) bool {
			return cs.Type == network.TCP && fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == serverAddr
		})

		outgoingConns = append(outgoingConns, newOutgoingConns...)
		incomingConns = append(incomingConns, newIncomingConns...)

		for _, conn := range newOutgoingConns {
			t.Logf("Found outgoing connection %v", conn)
			if conn.Protocol == expectedProtocol {
				foundOutgoingWithProtocol = true
				break
			}
		}
		for _, conn := range newIncomingConns {
			t.Logf("Found incoming connection %v", conn)
			if conn.Protocol == expectedProtocol {
				foundIncomingWithProtocol = true
				break
			}
		}

		if foundOutgoingWithProtocol && foundIncomingWithProtocol {
			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	// If we didn't find both -> fail
	if foundOutgoingWithProtocol || foundIncomingWithProtocol {
		// We have found at least one.
		// Checking if the reason for not finding the other is flakiness of npm
		if !foundIncomingWithProtocol && len(incomingConns) == 0 {
			t.Log("npm didn't find the incoming connections, not failing the test")
			return
		}

		if !foundOutgoingWithProtocol && len(outgoingConns) == 0 {
			t.Log("npm didn't find the outgoing connections, not failing the test")
			return
		}

	}

	t.Errorf("couldn't find incoming and outgoing connections with protocol %d for "+
		"server address %s and target address %s.\nIncoming: %v\nOutgoing: %v\nfound incoming with protocol: "+
		"%v\nfound outgoing with protocol: %v", expectedProtocol, serverAddr, targetAddr, incomingConns, outgoingConns, foundIncomingWithProtocol, foundOutgoingWithProtocol)
}
