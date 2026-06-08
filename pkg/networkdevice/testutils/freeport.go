// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package testutils

import (
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
	"strings"
)

// GetFreePort finds a free port to use for testing.
func GetFreePort() (uint16, error) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return 0, fmt.Errorf("can't find an available udp port: %s", err)
	}
	defer conn.Close()

	_, portString, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return 0, fmt.Errorf("can't find an available udp port: %s", err)
	}
	portInt, err := strconv.Atoi(portString)
	if err != nil {
		return 0, fmt.Errorf("can't convert udp port: %s", err)
	}

	return uint16(portInt), nil
}

// UniqueTestPort returns a deterministic UDP port derived from inputs,
// reducing the chance of collisions across concurrent tests without TOCTOU races.
func UniqueTestPort(keys ...string) uint16 {
	h := fnv.New32a()
	h.Write([]byte(strings.Join(keys, "|")))
	// Choose from 20000-29999. This range sits below the default ephemeral port
	// ranges on every supported platform (Linux: 32768-60999, macOS/Windows:
	// 49152-65535) and above the privileged range (<1024), so the OS will not
	// hand out one of these ports as an ephemeral source port for another
	// socket and steal it from the listener under test.
	return uint16(20000 + (h.Sum32() % 10000))
}
