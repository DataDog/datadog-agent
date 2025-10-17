// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package proxy

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

const (
	serverSrcPath = "external_unix_proxy_server"
)

func newExternalUnixTransparentProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS, useControl, useIPv6 bool) (*exec.Cmd, context.CancelFunc) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(curDir, serverSrcPath)
	require.NoError(t, err)

	args := []string{"-unix", unixPath, "-remote", remoteAddr}
	if useTLS {
		args = append(args, "-tls")
	}
	if useControl {
		args = append(args, "-control")
	}
	if useIPv6 {
		args = append(args, "-ipv6")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	c := exec.CommandContext(cancelCtx, serverBin, args...)
	require.NoError(t, c.Start())
	return c, func() {
		cancel()
		if c.Process != nil {
			_, _ = c.Process.Wait()
		}
	}
}

// NewExternalUnixTransparentProxyServer triggers an external unix transparent proxy (plaintext or TLS) server.
func NewExternalUnixTransparentProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS, useIPv6 bool) (*exec.Cmd, context.CancelFunc) {
	return newExternalUnixTransparentProxyServer(t, unixPath, remoteAddr, useTLS, false, useIPv6)
}

// NewExternalUnixControlProxyServer triggers an external unix proxy (plaintext or TLS) server with control
// messages.
func NewExternalUnixControlProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS, useIPv6 bool) (*exec.Cmd, context.CancelFunc) {
	return newExternalUnixTransparentProxyServer(t, unixPath, remoteAddr, useTLS, true, useIPv6)
}
