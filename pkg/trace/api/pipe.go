// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package api

import (
	"net"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/Microsoft/go-winio"
)

// listenPipe returns a listener on the given Windows Pipe, using the provided security
// descriptor and buffer size.
func listenPipe(path string, secdec string, bufferSize int, maxconn int, statsd statsd.ClientInterface) (net.Listener, error) {
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: secdec,
		InputBufferSize:    int32(bufferSize),
	})
	return NewMeasuredListener(ln, "pipe_connections", maxconn, statsd), err
}

// probeExistingPipe attempts to connect to a named pipe as a client.
// Returns true if a server is already listening on the pipe (orphan or prior owner),
// false if no server exists or the probe times out.
// Used by the [aas-repro] diagnostic path only.
func probeExistingPipe(path string) bool {
	t := 100 * time.Millisecond
	conn, err := winio.DialPipe(path, &t)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
