// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/stretchr/testify/require"
)

// NewGRPCTLSServer triggers an external go tls client that runs `numRequests` HTTPs requests to `serverAddr`.
// Returns the command executed and a callback to start sending requests.
func NewGRPCTLSServer(t *testing.T) (*exec.Cmd, context.CancelFunc) {
	serverBin := buildGRPCServerBin(t)
	serverCmd := fmt.Sprintf("%s", serverBin)

	cancelCtx, cancel := context.WithCancel(context.Background())
	c, _, err := nettestutil.StartCommandCtx(cancelCtx, serverCmd)

	require.NoError(t, err)
	return c, cancel
}

func buildGRPCServerBin(t *testing.T) string {
	const ServerSrcPath = "grpc_tls_server"
	const ServerBinaryPath = "grpc_tls_server/server"

	t.Helper()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	serverBinary := fmt.Sprintf("%s/%s", cur, ServerBinaryPath)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err = os.Stat(serverBinary); err == nil {
		return serverBinary
	}

	serverSrcDir := fmt.Sprintf("%s/%s", cur, ServerSrcPath)
	serverBuildDir, err := os.MkdirTemp("", "grpc_tls_server_build-")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(serverBuildDir)
	})

	serverBinPath := fmt.Sprintf("%s/server", serverBuildDir)

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", serverBinPath, serverSrcDir)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build grpc server test binary: %s\noutput: %s", err, string(out))

	return serverBinPath
}
