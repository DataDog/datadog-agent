// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// startTCPServer starts a command server on a random local TCP port and returns its address
func startTCPServer(t *testing.T, ipcComp *ipcmock.IPCMock) string {
	opts, err := serverOptions("tcp", ipcComp)
	require.NoError(t, err)

	server := grpc.NewServer(opts...)
	// the unimplemented server answers codes.Unimplemented once the call went through authentication
	api.RegisterSecurityModuleCmdServer(server, api.UnimplementedSecurityModuleCmdServer{})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = server.Serve(ln) }()
	t.Cleanup(server.Stop)

	return ln.Addr().String()
}

func getStatusCode(t *testing.T, addr string, opts ...grpc.DialOption) codes.Code {
	conn, err := grpc.NewClient(addr, opts...)
	require.NoError(t, err)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = api.NewSecurityModuleCmdClient(conn).GetStatus(ctx, &api.GetStatusParams{})
	return status.Code(err)
}

func TestTCPServerRequiresAuth(t *testing.T) {
	ipcComp := ipcmock.New(t)
	addr := startTCPServer(t, ipcComp)

	t.Run("authenticated", func(t *testing.T) {
		opts, err := ClientOptions("tcp", ipcComp)
		require.NoError(t, err)
		assert.Equal(t, codes.Unimplemented, getStatusCode(t, addr, opts...))
	})

	t.Run("plaintext", func(t *testing.T) {
		code := getStatusCode(t, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		assert.Equal(t, codes.Unavailable, code)
	})

	t.Run("no-token", func(t *testing.T) {
		code := getStatusCode(t, addr, grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())))
		assert.Equal(t, codes.Unauthenticated, code)
	})

	t.Run("invalid-token", func(t *testing.T) {
		code := getStatusCode(t, addr,
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
			grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth("invalid")),
		)
		assert.Equal(t, codes.Unauthenticated, code)
	})
}

func TestTCPRequiresIPC(t *testing.T) {
	_, err := serverOptions("tcp", nil)
	assert.Error(t, err)

	_, err = ClientOptions("tcp", nil)
	assert.Error(t, err)

	_, err = NewServer("tcp", "127.0.0.1:0", nil)
	assert.Error(t, err)
}

func TestUnauthenticatedFamilies(t *testing.T) {
	for _, family := range []string{"unix", "vsock"} {
		t.Run(family, func(t *testing.T) {
			_, err := serverOptions(family, nil)
			assert.NoError(t, err)

			opts, err := ClientOptions(family, nil)
			assert.NoError(t, err)
			assert.Len(t, opts, 1)
		})
	}
}
