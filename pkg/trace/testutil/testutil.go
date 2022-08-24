// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides easy ways to generate some random
// or deterministic data that can be use for tests or benchmarks.
//
// All the publicly shared trace agent model is available.
//
// It avoids the cumbersome step of having to redefine complicated
// structs in every test case and maintain common methods for quick
// access to almost all kind of stub data needed.
// It should NEVER be imported in a program, most likely in one-off
// projects or fuzz modes or test suites.
package testutil

import (
	"fmt"
	"net"
	"testing"
)

// FindTCPPort finds a free TCP port and returns it. If it fails, error will be non-nil.
func FindTCPPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, fmt.Errorf("resolve: %v", err)
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// FreeTCPPort returns a free TCP port. Upon encountering an error, it uses t to fail
// the test and report it.
func FreeTCPPort(t *testing.T) int {
	p, err := FindTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
