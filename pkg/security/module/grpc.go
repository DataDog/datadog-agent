// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package module

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type GRPCServer struct {
	server      *grpc.Server
	netListener net.Listener
	wg          sync.WaitGroup
	socketPath  string
}

type info struct {
	credentials.CommonAuthInfo
}

// AuthType returns the type of info as a string.
func (info) AuthType() string {
	return "unix socket"
}

type grpcUnixSocketTransportCredential struct {
	sig string
}

func (gustc grpcUnixSocketTransportCredential) ClientHandshake(ctx context.Context, authority string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, info{credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity}}, nil
}

func (gustc grpcUnixSocketTransportCredential) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return conn, info{credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity}}, nil
	}
	valid, err := processnet.IsUnixNetConnValid(unixConn, gustc.sig)
	if err != nil || !valid {
		if err != nil {
			log.Errorf("unix socket %s -> %s closing connection, error %s", unixConn.LocalAddr(), unixConn.RemoteAddr(), err)
		} else if !valid {
			log.Errorf("unix socket %s -> %s closing connection, rejected. Client accessing this socket require a signed binary", unixConn.LocalAddr(), unixConn.RemoteAddr())
		}
		// reject the connection
		conn.Close()
	}
	if valid {
		log.Debugf("unix socket %s -> %s connection authenticated", unixConn.LocalAddr(), unixConn.RemoteAddr())
	}
	return conn, info{credentials.CommonAuthInfo{SecurityLevel: credentials.PrivacyAndIntegrity}}, nil
}

func (gustc grpcUnixSocketTransportCredential) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "unix socket"}
}

func (gustc grpcUnixSocketTransportCredential) Clone() credentials.TransportCredentials {
	return grpcUnixSocketTransportCredential{sig: gustc.sig}
}

func (gustc grpcUnixSocketTransportCredential) OverrideServerName(string) error {
	return nil
}

func GRPCWithCredOptions(sig string) grpc.ServerOption {
	return grpc.Creds(grpcUnixSocketTransportCredential{sig: sig})
}

func NewGRPCServer(socketPath string, opts ...grpc.ServerOption) *GRPCServer {
	// force socket cleanup of previous socket not cleanup
	_ = os.Remove(socketPath)

	return &GRPCServer{
		socketPath: socketPath,
		server:     grpc.NewServer(opts...),
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
