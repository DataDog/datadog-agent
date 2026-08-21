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
//
// Hazard: the port is closed before this function returns, leaving a window in
// which another process can take it before the caller binds. Only use this
// when a port number is needed before the listener can exist (e.g. writing it
// into a subprocess's config). When the code under test is created in-process,
// prefer TCPListener, which never releases the port.
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
//
// Hazard: see FindTCPPort. Prefer TCPListener when the server under test is
// created in-process.
func FreeTCPPort(t *testing.T) int {
	p, err := FindTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TCPListener returns a listener bound to 127.0.0.1 on a free port, closed at
// test cleanup. Prefer this over FreeTCPPort/FindTCPPort when the server under
// test is created in-process: the port is never released, so nothing else can
// take it before the code under test binds.
func TCPListener(t *testing.T) net.Listener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}
