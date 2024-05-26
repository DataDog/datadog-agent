// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package proxy

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

const (
	serverSrcPath = "external_unix_proxy_server"
)

func newExternalUnixTransparentProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS, useControl bool) (*exec.Cmd, context.CancelFunc) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildUnixTransparentProxyServer(curDir, serverSrcPath)
	require.NoError(t, err)

	args := []string{serverBin, "-unix", unixPath, "-remote", remoteAddr}
	if useTLS {
		args = append(args, "-tls")
	}
	if useControl {
		args = append(args, "-control")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	commandLine := strings.Join(args, " ")
	c, _, err := nettestutil.StartCommandCtx(cancelCtx, commandLine)

	require.NoError(t, err)
	return c, func() {
		cancel()
		if c.Process != nil {
			_, _ = c.Process.Wait()
		}
	}
}

// NewExternalUnixTransparentProxyServer triggers an external unix transparent proxy (plaintext or TLS) server.
func NewExternalUnixTransparentProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS bool) (*exec.Cmd, context.CancelFunc) {
	return newExternalUnixTransparentProxyServer(t, unixPath, remoteAddr, useTLS, false)
}

// NewExternalUnixControlProxyServer triggers an external unix proxy (plaintext or TLS) server with control
// messages.
func NewExternalUnixControlProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS bool) (*exec.Cmd, context.CancelFunc) {
	return newExternalUnixTransparentProxyServer(t, unixPath, remoteAddr, useTLS, true)
}
