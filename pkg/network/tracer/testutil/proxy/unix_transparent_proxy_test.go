// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package proxy

import (
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

func TestUnixTransparentParentProxy(t *testing.T) {
	const remoteServerAddr = "127.0.0.1:5555"
	const unixPath = "/tmp/transparent.sock"
	const messageContent = "test"

	tests := []struct {
		name   string
		useTLS bool
	}{
		{
			name:   "plaintext",
			useTLS: false,
		},
		{
			name:   "TLS",
			useTLS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start the proxy server.
			_, cancel := NewExternalUnixTransparentProxyServer(t, unixPath, remoteServerAddr, tt.useTLS, false)
			t.Cleanup(cancel)

			// Start the remote server.
			tcpEchoServer := testutil.NewTCPServer(remoteServerAddr, func(c net.Conn) {
				defer c.Close()
				// Copy data from the connection's reader to its writer (echo)
				_, _ = io.Copy(c, c)
			}, tt.useTLS)
			done := make(chan struct{})
			t.Cleanup(func() { close(done) })
			require.NoError(t, tcpEchoServer.Run(done))

			require.NoError(t, WaitForConnectionReady(unixPath))

			// Start the proxy client.
			conn, err := net.DialTimeout("unix", unixPath, defaultDialTimeout)
			require.NoError(t, err)

			_, err = conn.Write([]byte(messageContent))
			require.NoError(t, err)
			output := make([]byte, len(messageContent))
			_, err = conn.Read(output)
			require.NoError(t, err)
			require.Equal(t, messageContent, string(output))
		})
	}
}
