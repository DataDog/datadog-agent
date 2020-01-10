// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package mock

import (
	"io/ioutil"
	"net"
	"testing"
)

// NewMockLogsIntake creates a TCP server that mimics the logs backend and returns
// a Listener. It's the caller's responsibility to close the listener.
func NewMockLogsIntake(t *testing.T) net.Listener {
	// This needs to be an IPv4 because most of the code doesn't handle IPv6 yet.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer l.Close()
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			defer conn.Close()

			for {
				_, err := ioutil.ReadAll(conn)
				if err != nil {
					break
				}
			}
			return
		}
	}()
	return l
}
