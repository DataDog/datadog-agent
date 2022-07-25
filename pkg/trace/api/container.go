// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !linux
// +build !linux

package api

import (
	"context"
	"net"
	"net/http"
)

func connContext(ctx context.Context, c net.Conn) context.Context {
	// unimplemented for non-linux builds.
	return ctx
}

// GetContainerID returns the container ID set by the client in the request header, or the empty
// string if none is present.
func GetContainerID(_ context.Context, h http.Header) string {
	return h.Get(headerContainerID)
}
