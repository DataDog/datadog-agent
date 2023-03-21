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

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	utilnet "github.com/DataDog/datadog-agent/pkg/util/net"
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
	allowedUsrID int
	allowedGrpID int
}

func (gustc grpcUnixSocketTransportCredential) ClientHandshake(ctx context.Context, authority string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return conn, info{credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity}}, nil
}

func (gustc grpcUnixSocketTransportCredential) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return conn, info{credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity}}, nil
	}
	valid, err := utilnet.IsUnixNetConnValid(unixConn, gustc.allowedUsrID, gustc.allowedGrpID)
	if err != nil || !valid {
		if err != nil {
			log.Errorf("unix socket %s -> %s closing connection, error %s", unixConn.LocalAddr(), unixConn.RemoteAddr(), err)
		}
		if !valid {
			log.Debugf("unix socket %s -> %s closing connection, rejected", unixConn.LocalAddr(), unixConn.RemoteAddr())
		}
		// reject the connection
		conn.Close()
	}
	return conn, info{credentials.CommonAuthInfo{SecurityLevel: credentials.PrivacyAndIntegrity}}, nil
}

func (gustc grpcUnixSocketTransportCredential) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "unix socket"}
}

func (gustc grpcUnixSocketTransportCredential) Clone() credentials.TransportCredentials {
	return grpcUnixSocketTransportCredential{gustc.allowedUsrID, gustc.allowedGrpID}
}

func (gustc grpcUnixSocketTransportCredential) OverrideServerName(string) error {
	return nil
}

func NewGRPCServer(socketPath string) *GRPCServer {
	// force socket cleanup of previous socket not cleanup
	_ = os.Remove(socketPath)

	found, allowedUsrID, allowedGrpID, err := filesystem.UserDDAgent()
	if err != nil || !found {
		// if user dd-agent doesn't exist, map to root
		allowedUsrID = 0
		allowedGrpID = 0
	}

	return &GRPCServer{
		socketPath: socketPath,
		server: grpc.NewServer(grpc.Creds(grpcUnixSocketTransportCredential{
			allowedUsrID: allowedUsrID,
			allowedGrpID: allowedGrpID})),
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
