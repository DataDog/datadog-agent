// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

type GRPCServer struct {
	server      *grpc.Server
	netListener net.Listener
	wg          sync.WaitGroup
	socketPath  string
}

func NewGRPCServer(socketPath string) *GRPCServer {
	// force socket cleanup of previous socket not cleanup
	_ = os.Remove(socketPath)

	return &GRPCServer{
		socketPath: socketPath,
		server:     grpc.NewServer(),
	}
}

func (g *GRPCServer) Start() error {
	ln, err := net.Listen("unix", g.socketPath)
	if err != nil {
		return fmt.Errorf("unable to create runtime security socket: %w", err)
	}

	if err := os.Chmod(g.socketPath, 0700); err != nil {
		return fmt.Errorf("unable to create runtime security socket: %w", err)
	}

	g.netListener = ln

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()

		if err := g.server.Serve(ln); err != nil {
			seclog.Errorf("error launching the grpc server: %v", err)
		}
	}()

	return nil
}

func (g *GRPCServer) Stop() {
	if g.server != nil {
		g.server.Stop()
	}

	if g.netListener != nil {
		g.netListener.Close()
		os.Remove(g.socketPath)
	}

	g.wg.Wait()
}
