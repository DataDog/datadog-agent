// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package httphelpers

import (
	"context"
	"net"

	"github.com/mdlayher/vsock"
)

func dialVSockContext(_ context.Context, cid, port uint32) (net.Conn, error) {
	return vsock.Dial(cid, port, &vsock.Config{})
}
