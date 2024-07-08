// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package testutil

import "net"

type TCPServer struct {
	address   string
	Network   string
	onMessage func(c net.Conn)
	ln        net.Listener
}

func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return NewTCPServerOnAddress("127.0.0.1:0", onMessage)
}

func NewTCPServerOnAddress(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

func (t *TCPServer) Address() string {
	return t.address
}

func (t *TCPServer) Addr() net.Addr {
	return t.ln.Addr()
}

func (t *TCPServer) Run() error {
	networkType := "tcp"
	if t.Network != "" {
		networkType = t.Network
	}
	ln, err := net.Listen(networkType, t.address)
	if err != nil {
		return err
	}
	t.ln = ln
	t.address = ln.Addr().String()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go t.onMessage(conn)
		}
	}()

	return nil
}

func (t *TCPServer) Shutdown() {
	if t.ln != nil {
		_ = t.ln.Close()
		t.ln = nil
	}
}
