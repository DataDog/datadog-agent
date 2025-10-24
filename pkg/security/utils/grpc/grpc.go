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
	"sync"

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
	ln, err := net.Listen(g.family, g.address)
	if err != nil {
		return fmt.Errorf("unable to create runtime security socket: %w", err)
	}

	if g.family == "unix" {
		if err := os.Chmod(g.address, 0700); err != nil {
			return fmt.Errorf("unable to update permissions of runtime security socket: %w", err)
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
