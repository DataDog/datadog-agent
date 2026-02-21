// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc holds grpc related files
package grpc

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"

	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// Server defines a gRPC server
type Server struct {
	server  *grpc.Server
	wg      sync.WaitGroup
	family  string
	address string
}

// getDDAgentIDs returns the UID and GID of the dd-agent user if it exists
// Returns °, 0 if the user doesn't exist or on error
func getDDAgentIDs() (int, int) {
	var uid, gid int

	ddUser, err := user.Lookup("dd-agent")
	if err == nil {
		if uid, err = strconv.Atoi(ddUser.Uid); err != nil {
			uid = -1
			seclog.Warnf("failed to parse dd-agent UID: %v", err)
		}
	}

	ddGroup, err := user.LookupGroup("dd-agent")
	if err == nil {
		if gid, err = strconv.Atoi(ddGroup.Gid); err != nil {
			gid = -1
			seclog.Warnf("failed to parse dd-agent GID: %v", err)
		}
	}

	return uid, gid
}

// NewServer returns a new gRPC server
func NewServer(family string, address string) *Server {
	// force socket cleanup of previous socket not cleanup
	if family == "unix" {
		if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
			seclog.Errorf("error removing the previous runtime security socket: %v", err)
		}
	}

	// Add gRPC metrics interceptors
	opts := grpcutil.ServerOptionsWithMetrics()

	return &Server{
		family:  family,
		address: address,
		server:  grpc.NewServer(opts...),
	}
}

// ServiceRegistrar returns the gRPC server
func (g *Server) ServiceRegistrar() grpc.ServiceRegistrar {
	return g.server
}

// Start the server
func (g *Server) Start() error {
	var ln net.Listener
	var err error

	if g.family == "vsock" {
		port, parseErr := strconv.Atoi(g.address)
		if parseErr != nil {
			return parseErr
		}

		if port <= 0 {
			return fmt.Errorf("invalid port '%s' for vsock", g.address)
		}

		seclog.Infof("starting runtime security agent gRPC server on vsock port %d with host context", port)
		ln, err = vsock.ListenContextID(vsock.Host, uint32(port), &vsock.Config{})
	} else {
		ln, err = net.Listen(g.family, g.address)
	}

	if err != nil {
		return fmt.Errorf("unable to create runtime security socket: %w", err)
	}

	if g.family == "unix" {
		if err := os.Chmod(g.address, 0770); err != nil {
			return fmt.Errorf("unable to update permissions of runtime security socket: %w", err)
		}

		// Set ownership to dd-agent user/group if it exists
		// This allows the agent running as dd-agent to access the socket
		uid, gid := getDDAgentIDs()
		if uid != -1 && gid != -1 {
			if err := syscall.Chown(g.address, uid, gid); err != nil {
				seclog.Warnf("unable to set dd-agent ownership for runtime security socket: %v", err)
				// Don't return error - this is not critical if running as root
			} else {
				seclog.Debugf("set runtime security socket ownership to dd-agent (uid=%d, gid=%d)", uid, gid)
			}
		}
	}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()

		if err := g.server.Serve(ln); err != nil {
			seclog.Errorf("error launching the grpc server: %v", err)
		}
	}()

	return nil
}

// Stop the server
func (g *Server) Stop() {
	if g.server != nil {
		g.server.Stop()
	}

	if g.family == "unix" {
		if err := os.Remove(g.address); err != nil && !os.IsNotExist(err) {
			seclog.Errorf("error removing the runtime security socket: %v", err)
		}
	}

	g.wg.Wait()
}
