// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"bufio"
	"context"
	"net"
	nethttp "net/http"
	"syscall"
	"testing"
	"time"

	netlink "github.com/DataDog/datadog-agent/pkg/network/netlink/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestProtocolClassification(t *testing.T) {
	cfg := testConfig()
	if !classificationSupported(cfg) {
		t.Skip("Classification is not supported")
	}

	t.Run("with dnat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 2.2.2.2 to 1.1.1.1
		netlink.SetupDNAT(t)
		testProtocolClassification(t, cfg, "localhost", "2.2.2.2", "1.1.1.1:0")
		testProtocolClassificationMapCleanup(t, cfg, "localhost", "2.2.2.2", "1.1.1.1:0")
	})

	t.Run("with snat", func(t *testing.T) {
		// SetupDNAT sets up a NAT translation from 6.6.6.6 to 7.7.7.7
		netlink.SetupSNAT(t)
		testProtocolClassification(t, cfg, "6.6.6.6", "127.0.0.1", "127.0.0.1:0")
		testProtocolClassificationMapCleanup(t, cfg, "6.6.6.6", "127.0.0.1", "127.0.0.1:0")
	})

	t.Run("without nat", func(t *testing.T) {
		testProtocolClassification(t, cfg, "localhost", "127.0.0.1", "127.0.0.1:0")
		testProtocolClassificationMapCleanup(t, cfg, "localhost", "127.0.0.1", "127.0.0.1:0")
	})
}

func testProtocolClassificationMapCleanup(t *testing.T, cfg *config.Config, clientHost, targetHost, serverHost string) {
	t.Run("protocol cleanup", func(t *testing.T) {
		dialer := &net.Dialer{
			LocalAddr: &net.TCPAddr{
				IP:   net.ParseIP(clientHost),
				Port: 0,
			},
			Control: func(network, address string, c syscall.RawConn) error {
				var opErr error
				err := c.Control(func(fd uintptr) {
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
					opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				})
				if err != nil {
					return err
				}
				return opErr
			},
		}

		tr, err := NewTracer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer tr.Stop()

		initTracerState(t, tr)
		require.NoError(t, err)
		done := make(chan struct{})
		defer close(done)
		server := NewTCPServerOnAddress(serverHost, func(c net.Conn) {
			r := bufio.NewReader(c)
			input, err := r.ReadBytes(byte('\n'))
			if err == nil {
				c.Write(input)
			}
			c.Close()
		})
		require.NoError(t, server.Run(done))
		_, port, err := net.SplitHostPort(server.address)
		require.NoError(t, err)
		targetAddr := net.JoinHostPort(targetHost, port)

		// Letting the server time to start
		time.Sleep(500 * time.Millisecond)

		// Running a HTTP client
		client := nethttp.Client{
			Transport: &nethttp.Transport{
				DialContext: dialer.DialContext,
			},
		}
		resp, err := client.Get("http://" + targetAddr + "/test")
		if err == nil {
			resp.Body.Close()
		}

		client.CloseIdleConnections()
		waitForConnectionsWithProtocol(t, tr, targetAddr, server.address, network.ProtocolHTTP)

		time.Sleep(2 * time.Second)
		grpcClient, err := grpc.NewClient(targetAddr, grpc.Options{
			CustomDialer: dialer,
		})
		require.NoError(t, err)
		defer grpcClient.Close()
		_ = grpcClient.HandleUnary(context.Background(), "test")
		waitForConnectionsWithProtocol(t, tr, targetAddr, server.address, network.ProtocolHTTP2)
	})
}
