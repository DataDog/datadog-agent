// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package localapiclientimpl

import (
	"context"
	"net"
	"net/http"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(paths.RunPath, "installer.sock"))
			},
		},
	}
}
