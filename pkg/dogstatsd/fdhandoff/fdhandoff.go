// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fdhandoff implements the handoff of a bound DogStatsD datagram socket
// between a long-lived socket-holder process and the Agent.
//
// The holder binds the DogStatsD "unixgram" socket once and never closes nor
// unlinks it. It then listens on a separate unix stream socket (the "handoff"
// socket) and, for every connection, sends the datagram socket's file
// descriptor over it using SCM_RIGHTS. The Agent connects to that handoff
// socket on startup and adopts the file descriptor instead of binding the
// socket itself, so clients keep sending to the same socket inode across Agent
// restarts and no datagram is lost while the Agent is down.
//
// This is the datagram counterpart of the APM socket-activation mechanism in
// pkg/trace/api/loader, which passes file descriptors through the exec syscall
// instead.
package fdhandoff

import (
	"errors"
	"fmt"
	"net"
)

// ErrUnsupported is returned on platforms that cannot pass file descriptors
// over a unix socket.
var ErrUnsupported = errors.New("dogstatsd socket handoff is only implemented on Linux hosts")

// ReceivePacketConn connects to the handoff socket at handoffPath, receives the
// DogStatsD datagram socket file descriptor from the holder and wraps it in a
// *net.UnixConn ready to be read from.
//
// The received file descriptor is duplicated by the Go runtime, so closing the
// returned connection does not affect the holder's copy and does not unlink the
// socket path.
func ReceivePacketConn(handoffPath string) (*net.UnixConn, error) {
	f, err := receiveFile(handoffPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// FilePacketConn (and not FileConn) is required here: the file descriptor
	// refers to an unbound-peer datagram socket, so the connection must be
	// usable with ReadFrom/ReadMsg.
	packetConn, err := net.FilePacketConn(f)
	if err != nil {
		return nil, fmt.Errorf("could not create a packet connection from the received file descriptor: %w", err)
	}

	conn, ok := packetConn.(*net.UnixConn)
	if !ok {
		packetConn.Close()
		return nil, fmt.Errorf("unexpected return type from FilePacketConn, expected UnixConn: %#v", packetConn)
	}

	return conn, nil
}
