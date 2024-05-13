// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"crypto/tls"
	"net"
)

// TCPServer represents a basic TCP server configuration.
type TCPServer struct {
	address   string
	onMessage func(c net.Conn)
	isTLS     bool
}

// NewTCPServer creates and initializes a new TCPServer instance with the provided address
// and callback function to handle incoming messages.
func NewTCPServer(addr string, onMessage func(c net.Conn), isTLS bool) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
		isTLS:     isTLS,
	}
}

// Run starts the TCPServer to listen on its configured address.
func (s *TCPServer) Run(done chan struct{}) error {
	var ln net.Listener
	var lnErr error

	if s.isTLS {
		crtPath, keyPath, err := GetCertsPaths()
		if err != nil {
			return err
		}
		cert, err := tls.LoadX509KeyPair(crtPath, keyPath)
		if err != nil {
			return err
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		ln, lnErr = tls.Listen("tcp", s.address, tlsConfig)
	} else {
		ln, lnErr = net.Listen("tcp", s.address)
	}
	if lnErr != nil {
		return lnErr
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
