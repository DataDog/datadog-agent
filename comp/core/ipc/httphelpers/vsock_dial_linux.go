// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package httphelpers

import (
	"context"
	"net"

	"github.com/mdlayher/socket"
	"golang.org/x/sys/unix"
)

func dialVSockContext(ctx context.Context, cid, port uint32) (net.Conn, error) {
	// Use socket directly so Connect observes the transport context, which carries
	// the server timeout. vsock.Dial connects with context.Background instead.
	conn, err := socket.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0, "vsock", nil)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Connect(ctx, &unix.SockaddrVM{CID: cid, Port: port}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return conn, nil
}
