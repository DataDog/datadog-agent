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
	return ctx
}

// GetContainerID attempts first to read the container ID set by the client in the request header.
// If no such header is present or the value is empty, it will look in the container ID cache. If
// there is a valid (not stale) container ID for the given pid, that is returned. Otherwise the
// container ID is parsed using readContainerID. If none of these methods succeed, getContainerID
// returns an empty string.
func GetContainerID(h http.Header) string {
	return h.Get(headerContainerID)
}
