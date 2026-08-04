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
package main

import (
	"errors"
	"flag"
	"log"
	"net"
	"os"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/fdhandoff"
)

const (
	defaultSocketPath  = "/var/run/datadog/dsd.socket"
	defaultHandoffPath = "/var/run/datadog/dsd-handoff.sock"
	// The handoff socket gives away a file descriptor on the DogStatsD socket,
	// which allows reading everything clients send to it. It is therefore
	// restricted to the user running the holder by default: widen it with
	// --handoff-mode when the Agent runs as another user.
	defaultHandoffMode = 0700
)

func main() {
	socketPath := flag.String("socket", defaultSocketPath, "path of the DogStatsD datagram socket to bind and hold")
	handoffPath := flag.String("handoff", defaultHandoffPath, "path of the unix stream socket the file descriptor is handed off on")
	handoffMode := flag.Int("handoff-mode", defaultHandoffMode, "permissions of the handoff socket, as an octal value")
	rcvbuf := flag.Int("so-rcvbuf", 0, "size of the DogStatsD socket receive buffer in bytes, 0 to leave the system default")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)
	log.SetPrefix("dogstatsd-socket-holder: ")

	socket, err := fdhandoff.BindDatagramSocket(*socketPath)
	if err != nil {
		log.Fatalf("failed to bind %s: %v", *socketPath, err)
	}
	// The socket is deliberately never closed nor unlinked: it must outlive every
	// Agent restart.

	if *rcvbuf != 0 {
		if err := socket.SetReadBuffer(*rcvbuf); err != nil {
			log.Printf("could not set socket rcvbuf: %v", err)
		}
	}

	server, err := fdhandoff.NewServer(*handoffPath, os.FileMode(*handoffMode), socket)
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
