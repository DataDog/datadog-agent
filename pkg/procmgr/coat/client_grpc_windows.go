// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package coat

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func dialProcmgrGRPC(socketPath string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(
		"passthrough:///procmgr",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return winio.DialPipe(socketPath, nil)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to dd-procmgrd: %w", err)
	}
	return conn, nil
}
