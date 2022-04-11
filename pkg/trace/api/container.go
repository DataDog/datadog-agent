// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.
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

func getContainerID(req *http.Request) string {
	return req.Header.Get(headerContainerID)
}
