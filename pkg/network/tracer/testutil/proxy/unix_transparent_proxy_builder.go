// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package proxy

import (
	"context"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
)

// NewExternalUnixTransparentProxyServer triggers an external unix transparent proxy (plaintext or TLS) server.
func NewExternalUnixTransparentProxyServer(t *testing.T, unixPath, remoteAddr string, useTLS bool) (*exec.Cmd, context.CancelFunc) {
	serverBin := buildUnixTransparentProxyServer(t)
	args := []string{serverBin, "-unix", unixPath, "-remote", remoteAddr}
	if useTLS {
		args = append(args, "-tls")
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

const (
	serverSrcPath = "external_unix_proxy_server"
)

// buildUnixTransparentProxyServer builds the unix transparent proxy server binary and returns the path to the binary.
// If the binary is already built (meanly in the CI), it returns the path to the binary.
func buildUnixTransparentProxyServer(t *testing.T) string {
	t.Helper()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	serverSrcDir := path.Join(cur, serverSrcPath)
	cachedServerBinaryPath := path.Join(serverSrcDir, serverSrcPath)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err = os.Stat(cachedServerBinaryPath); err == nil {
		return cachedServerBinaryPath
	}

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-tags=test", "-ldflags=-extldflags '-static'", "-o", cachedServerBinaryPath, serverSrcDir)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build grpc server test binary: %s\noutput: %s", err, string(out))

	return cachedServerBinaryPath
}
