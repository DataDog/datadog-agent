// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package localapiclientimpl

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/Microsoft/go-winio"
)

const defaultPipeDialTimeout = 2 * time.Second

func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				if _, ok := ctx.Deadline(); ok {
					return winio.DialPipeContext(ctx, `\\.\pipe\DD_INSTALLER`)
				}
				dialCtx, cancel := context.WithTimeout(ctx, defaultPipeDialTimeout)
				defer cancel()
				return winio.DialPipeContext(dialCtx, `\\.\pipe\DD_INSTALLER`)
			},
		},
	}
}
