// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

func TestTLSHandshakeTimeoutReleasesWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cm := newConnectionManagerForHostPort("localhost", 443)
		for i := 0; i < 3; i++ {
			conn, peer := net.Pipe()
			ssl := tls.Client(conn, &tls.Config{ServerName: "localhost"})
			err := cm.handshakeWithTimeout(t.Context(), ssl, time.Second)
			require.ErrorAs(t, err, new(tlsTimeoutError))
			ssl.Close()
			_, err = peer.Write([]byte("closed"))
			require.Error(t, err)
			peer.Close()
		}
		// Every timed-out handshake must leave the bubble.
		synctest.Wait()
	})
}

func tcpTestListener(t *testing.T) *net.TCPListener {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	require.NoError(t, listener.SetDeadline(time.Now().Add(5*time.Second)))
	return listener
}

func TestConnectionNegotiationCanceled(t *testing.T) {
	for _, socks := range []bool{false, true} {
		t.Run(map[bool]string{false: "tls", true: "socks"}[socks], func(t *testing.T) {
			listener := tcpTestListener(t)
			host, port := AddrToHostPort(listener.Addr())
			endpoint := config.NewEndpoint("", "", host, port, config.EmptyPathPrefix, !socks)
			if socks {
				endpoint.ProxyAddress = listener.Addr().String()
			}
			cm := NewConnectionManager(endpoint, statusinterface.NewNoopStatusProvider())
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				conn, err := cm.NewConnection(ctx)
				if conn != nil {
					conn.Close()
				}
				done <- err
			}()
			peer, err := listener.Accept()
			require.NoError(t, err)
			defer peer.Close()
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(2 * time.Second):
				t.Fatal("negotiation ignored cancellation")
			}
		})
	}
}

func TestConnectionClosesOnCancellation(t *testing.T) {
	listener := tcpTestListener(t)
	cm := newConnectionManagerForAddr(listener.Addr())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	conn, err := cm.NewConnection(ctx)
	require.NoError(t, err)
	defer conn.Close()
	peer, err := listener.Accept()
	require.NoError(t, err)
	defer peer.Close()
	require.NoError(t, peer.SetReadDeadline(time.Now().Add(2*time.Second)))
	cancel()
	_, err = io.Copy(io.Discard, peer)
	require.NoError(t, err)
}

func TestConnectivityDiagnoseClosesConnection(t *testing.T) {
	listener := tcpTestListener(t)
	host, port := AddrToHostPort(listener.Addr())
	endpoint := config.NewEndpoint("", "", host, port, config.EmptyPathPrefix, false)
	_, err := CheckConnectivityDiagnose(endpoint, 2)
	require.NoError(t, err)
	peer, err := listener.Accept()
	require.NoError(t, err)
	defer peer.Close()
	require.NoError(t, peer.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = io.Copy(io.Discard, peer)
	require.NoError(t, err)
}

func TestDestinationWriteHasDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("key", "", "localhost", 1234, config.EmptyPathPrefix, false)
		destination := NewDestination(endpoint, false, client.NewDestinationsContext(), false, statusinterface.NewNoopStatusProvider())
		conn, peer := net.Pipe()
		defer peer.Close()
		destination.conn = conn
		output := make(chan *message.Payload, 1)
		start := time.Now()
		destination.sendAndRetry(message.NewPayload(nil, []byte("logs"), "source", 1), output, nil)
		require.Equal(t, connectionTimeout, time.Since(start))
		require.Nil(t, destination.conn)
		require.Empty(t, output)
	})
}

func TestDestinationClosesConnectionOnDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("key", "", "localhost", 1234, config.EmptyPathPrefix, false)
		destination := NewDestination(endpoint, false, client.NewDestinationsContext(), false, statusinterface.NewNoopStatusProvider())
		conn, peer := net.Pipe()
		defer peer.Close()
		destination.conn = conn
		input := make(chan *message.Payload)
		close(input)
		<-destination.Start(input, nil, nil)
		_, err := peer.Read(make([]byte, 1))
		require.ErrorIs(t, err, io.EOF)
		require.Nil(t, destination.conn)
	})
}
