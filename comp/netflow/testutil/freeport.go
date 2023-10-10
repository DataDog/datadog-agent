// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package testutil

import (
	"net"
	"strconv"
)

// GetFreePort finds a free port to use for testing.
func GetFreePort() uint16 {
	var port uint16
	for i := 0; i < 5; i++ {
		conn, err := net.ListenPacket("udp", ":0")
		if err != nil {
			continue
		}
		conn.Close()
		port, err = parsePort(conn.LocalAddr().String())
		if err != nil {
			continue
		}
		return port
	}
	panic("unable to find free port for starting the trap listener")
}

func parsePort(addr string) (uint16, error) {
	_, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}

	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(port), nil
}
