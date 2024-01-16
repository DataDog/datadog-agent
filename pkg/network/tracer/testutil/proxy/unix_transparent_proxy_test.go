// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func TestUnixTransparentParentProxy(t *testing.T) {
	const remoteServerAddr = "127.0.0.1:5555"
	const unixPath = "/tmp/transparent.sock"

	// Start the proxy server.
	_, cancel := NewExternalUnixTransparentProxyServer(t, unixPath, remoteServerAddr, false)
	t.Cleanup(cancel)

	// Start the remote server.
	tcpEchoServer := testutil.NewTCPServer(remoteServerAddr, func(c net.Conn) {
		defer c.Close()
		// Copy data from the connection's reader to its writer (echo)
		_, _ = io.Copy(c, c)
	})
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	require.NoError(t, tcpEchoServer.Run(done))

	require.NoError(t, WaitForConnectionReady(unixPath))

	// Start the proxy client.
	conn, err := net.DialTimeout("unix", unixPath, defaultDialTimeout)
	require.NoError(t, err)

	_, err = conn.Write([]byte("test"))
	require.NoError(t, err)
	output := make([]byte, 4)
	_, err = conn.Read(output)
	require.NoError(t, err)
	require.Equal(t, "test", string(output))
}

func TestTLSUnixTransparentParentProxy(t *testing.T) {
	const remoteServerAddr = "127.0.0.1:5556"
	const unixPath = "/tmp/transparent-tls.sock"

	// Start the proxy server.
	_, cancel := NewExternalUnixTransparentProxyServer(t, unixPath, remoteServerAddr, true)
	t.Cleanup(cancel)

	// Start the remote server.
	tlsServerCancel := testutil.HTTPServer(t, remoteServerAddr, testutil.Options{EnableTLS: true})
	t.Cleanup(tlsServerCancel)

	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", unixPath)
			},
		},
	}

	require.NoError(t, WaitForConnectionReady(unixPath))
	response, err := client.Get("http://unix/status/201")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 201, response.StatusCode)
}
