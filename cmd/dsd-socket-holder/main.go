// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// The DogStatsD socket holder binds the DogStatsD datagram socket once and hands
// its file descriptor over to the Agent on demand.
//
// The Agent normally unlinks and rebinds the socket every time it starts, which
// destroys the socket inode: clients keep writing to the old one and their
// datagrams are lost until they reconnect. When the holder owns the socket
// instead, the inode outlives the Agent and datagrams sent while the Agent is
// restarting simply queue up in the socket receive buffer.
//
// The holder is expected to be supervised (systemd, an init container, ...) and
// to run for the lifetime of the host. Set dogstatsd_socket_fd_from in the Agent
// configuration to the value of --handoff to make the Agent adopt the socket.
//
// The holder refuses to start while another process is bound to --socket, so an
// Agent that binds the socket itself has to be stopped, or reconfigured with
// dogstatsd_socket_fd_from, before the holder takes over.
package main

import (
	"errors"
	"flag"
	"log"
	"net"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/fdhandoff"
)

const (
	defaultSocketPath  = "/var/run/datadog/dsd.socket"
	defaultHandoffPath = "/var/run/datadog/dsd-handoff.sock"
	// The handoff socket gives away a file descriptor on the DogStatsD socket,
	// which allows reading everything clients send to it. It is therefore
	// restricted to the user running the holder by default: widen it with
	// --handoff-mode when the Agent runs as another user.
	defaultHandoffMode = "0700"
)

func main() {
	socketPath := flag.String("socket", defaultSocketPath, "path of the DogStatsD datagram socket to bind and hold")
	handoffPath := flag.String("handoff", defaultHandoffPath, "path of the unix stream socket the file descriptor is handed off on")
	handoffMode := flag.String("handoff-mode", defaultHandoffMode, "permissions of the handoff socket, as an octal value")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)
	log.SetPrefix("dogstatsd-socket-holder: ")

	mode, err := fdhandoff.ParseMode(*handoffMode)
	if err != nil {
		log.Fatalf("invalid --handoff-mode: %v", err)
	}

	socket, err := fdhandoff.BindDatagramSocket(*socketPath)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", *socketPath, err)
	}
	// The socket is deliberately never closed nor unlinked: it must outlive every
	// Agent restart.

	// Deliberately no receive-buffer option here. For AF_UNIX datagrams the
	// sender allocates the skb from its own send buffer, so how much survives an
	// Agent restart is bounded by the *client's* SO_SNDBUF, not by anything the
	// holder can set. Measured on Linux 6.8 with 100-byte datagrams: raising the
	// holder's SO_RCVBUF to 64 MB left the queue at 278 datagrams, while raising
	// the client's SO_SNDBUF took it to 555. Both are then clamped by
	// net.core.{r,w}mem_max, which is not network-namespaced and so cannot be set
	// from a pod. A knob here would only look like it tuned something.

	server, err := fdhandoff.NewServer(*handoffPath, mode, socket)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *handoffPath, err)
	}
	server.ErrorHandler = func(err error) {
		log.Printf("handoff failed: %v", err)
	}

	log.Printf("holding %s, handing its file descriptor off on %s", socket.Path(), server.Addr())

	if err := server.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Fatalf("failed to serve %s: %v", *handoffPath, err)
	}
}
