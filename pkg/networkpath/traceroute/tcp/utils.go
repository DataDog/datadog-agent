// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"fmt"
	"net"
)

// reserveLocalPort reserves an ephemeral TCP port
// and returns both the listener and port because the
// listener should be held until the port is no longer
// in use
func reserveLocalPort() (uint16, net.Listener, error) {
	// Create a TCP listener with port 0 to get a random port from the OS
	// and reserve it for the duration of the traceroute
	tcpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}
	tcpAddr := tcpListener.Addr().(*net.TCPAddr)

	return uint16(tcpAddr.Port), tcpListener, nil
}
