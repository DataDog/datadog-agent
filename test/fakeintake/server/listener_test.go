// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package server

import (
	"bytes"
	"compress/gzip"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type observedListener struct {
	net.Listener
	wire *observedWire
}

type observedWire struct {
	sync.Mutex
	bytes.Buffer
}

type observedConnection struct {
	net.Conn
	wire *observedWire
}

func (l observedListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return observedConnection{Conn: conn, wire: l.wire}, nil
}

func (c observedConnection) Read(data []byte) (int, error) {
	n, err := c.Conn.Read(data)
	c.wire.Lock()
	_, _ = c.wire.Write(data[:n])
	c.wire.Unlock()
	return n, err
}

func TestListenerFactoryObservesOriginalRequest(t *testing.T) {
	var wire observedWire
	fi, _ := InitialiseForTests(t,
		WithAddress("127.0.0.1:0"),
		WithListenerFactory(func(network, address string) (net.Listener, error) {
			listener, err := net.Listen(network, address)
			if err != nil {
				return nil, err
			}
			return observedListener{Listener: listener, wire: &wire}, nil
		}),
	)
	t.Cleanup(func() { require.NoError(t, fi.Stop()) })

	var body bytes.Buffer
	compressed := gzip.NewWriter(&body)
	_, err := compressed.Write([]byte(`[{"message":"observed"}]`))
	require.NoError(t, err)
	require.NoError(t, compressed.Close())
	request, err := http.NewRequest(http.MethodPost, fi.URL()+"/api/v2/logs?test=wire", bytes.NewReader(body.Bytes()))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	request.Header.Set("X-Observation", "original")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)

	payloads := fi.store.GetRawPayloads("/api/v2/logs")
	require.Len(t, payloads, 1)
	require.Equal(t, body.Bytes(), payloads[0].Data)
	wire.Lock()
	defer wire.Unlock()
	require.Contains(t, wire.String(), "POST /api/v2/logs?test=wire HTTP/1.1\r\n")
	require.Contains(t, wire.String(), "X-Observation: original\r\n")
	require.True(t, bytes.Contains(wire.Bytes(), body.Bytes()), "listener must observe compressed bytes unchanged")
}
