// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import "net"

// TCPServer represents a basic TCP server configuration.
type TCPServer struct {
	address   string
	onMessage func(c net.Conn)
}

// NewTCPServer creates and initializes a new TCPServer instance with the provided address
// and callback function to handle incoming messages.
func NewTCPServer(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

// Run starts the TCPServer to listen on its configured address.
func (s *TCPServer) Run(done chan struct{}) error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.address = ln.Addr().String()

	go func() {
		<-done
		ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.onMessage(conn)
		}
	}()

	return nil
}
