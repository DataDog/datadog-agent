// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"fmt"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// GRPCServer defines a gRPC server
type GRPCServer struct {
	server  *grpc.Server
	wg      sync.WaitGroup
	family  string
	address string
}

// NewGRPCServer returns a new gRPC server
func NewGRPCServer(family string, address string) *GRPCServer {
	// force socket cleanup of previous socket not cleanup
	if family == "unix" {
		if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
			seclog.Errorf("error removing the previous runtime security socket: %v", err)
		}
	}

	return &GRPCServer{
		family:  family,
		address: address,
		server:  grpc.NewServer(),
	}
}

// Start the server
func (g *GRPCServer) Start() error {
	ln, err := net.Listen(g.family, g.address)
	if err != nil {
		return fmt.Errorf("unable to create runtime security socket: %w", err)
	}

	if g.family == "unix" {
		if err := os.Chmod(g.address, 0700); err != nil {
			return fmt.Errorf("unable to create runtime security socket: %w", err)
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
func (g *GRPCServer) Stop() {
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
