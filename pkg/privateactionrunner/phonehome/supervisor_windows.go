// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package phonehome

import (
	"context"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Match dd-procmgrd's Windows default in pkg/procmgr/rust/client/src/lib.rs.
const defaultSupervisorSocket = `\\.\pipe\datadog-procmgrd`

func validSupervisorSocket(path string) bool {
	const prefix = `\\.\pipe\`
	return strings.HasPrefix(strings.ToLower(path), prefix) && len(path) > len(prefix)
}

func dialSupervisor(path string) (*grpc.ClientConn, error) {
	return grpc.NewClient("passthrough:///procmgr",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			// Match COAT and AGENT_PIPE_CLIENT_ACCESS_MASK; GENERIC_WRITE includes FILE_CREATE_PIPE_INSTANCE.
			return winio.DialPipeAccessImpLevel(ctx, path,
				uint32(windows.GENERIC_READ|windows.FILE_WRITE_DATA), winio.PipeImpLevelIdentification)
		}))
}
