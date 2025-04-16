// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package testutil has utilities for testing the network tracer
package testutil

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// TCPServer is a simple TCP server for use in tests
type TCPServer struct {
	address   string
	Network   string
	onMessage func(c net.Conn)
	ln        net.Listener
}

// NewTCPServer creates a TCPServer using the provided function for newly accepted connections.
// It defaults to listening on an ephemeral port on 127.0.0.1
func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return NewTCPServerOnAddress("127.0.0.1:0", onMessage)
}

// NewTCPServerOnAddress creates a TCPServer using the provided address.
func NewTCPServerOnAddress(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

// Address returns the address of the server. This should be called after Run.
func (t *TCPServer) Address() string {
	return t.address
}

// Addr is the raw net.Addr of the listener
func (t *TCPServer) Addr() net.Addr {
	return t.ln.Addr()
}

// Run starts the TCP server
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
			err = SetTestDeadline(conn)
			if err != nil {
				return
			}
			go t.onMessage(conn)
		}
	}()

	return nil
}

// Dial creates a TCP connection to the server, and sets reasonable timeouts
func (t *TCPServer) Dial() (net.Conn, error) {
	return DialTCP("tcp", t.Address())
}

// DialTCP creates a connection to the specified address, and sets reasonable timeouts for TCP
func DialTCP(network, address string) (net.Conn, error) {
	conn, err := net.DialTimeout(network, address, time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", address, err)
	}
	err = SetTestDeadline(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Shutdown stops the TCP server
func (t *TCPServer) Shutdown() {
	if t.ln != nil {
		_ = t.ln.Close()
		t.ln = nil
	}
}

// SetTestDeadline prevents connection reads/writes from blocking the test indefinitely
func SetTestDeadline(conn net.Conn) error {
	// any test in the tracer suite should conclude in less than a minute (normally a couple seconds)
	return conn.SetDeadline(time.Now().Add(time.Minute))
}

// GracefulCloseTCP closes a connection after making sure all data has been sent/read
// It first shuts down the write end, then reads until EOF, then closes the connection
// https://blog.netherlabs.nl/articles/2009/01/18/the-ultimate-so_linger-page-or-why-is-my-tcp-not-reliable
func GracefulCloseTCP(conn net.Conn) error {
	tcpConn := conn.(*net.TCPConn)

	shutdownErr := tcpConn.CloseWrite()
	_, readErr := io.Copy(io.Discard, tcpConn)
	closeErr := tcpConn.Close()
	return errors.Join(shutdownErr, readErr, closeErr)
}
