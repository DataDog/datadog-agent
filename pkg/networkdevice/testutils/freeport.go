// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package testutils

import (
	"net"
	"sync"
)

var (
	globalMutex sync.Mutex // Prevent the util function to be called concurrently
	usedPorts   = make(map[uint16]bool)
)

// GetFreePort finds a free port to use for testing.
// Borrowed from: https://github.com/phayes/freeport/blame/master/freeport.go#L8-L20
func GetFreePort() (uint16, error) {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	var lastError error
	for retries := 0; retries < 5; retries++ {
		port, err := getSingleFreePort()
		if err != nil {
			lastError = err
			continue
		}
		if _, ok := usedPorts[port]; !ok {
			usedPorts[port] = true
			return port, nil
		}
	}
	return 0, lastError
}

func getSingleFreePort() (uint16, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port), nil
}
