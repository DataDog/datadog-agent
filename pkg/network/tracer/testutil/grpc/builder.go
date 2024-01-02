// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

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

// NewGRPCTLSServer triggers an external go tls client that runs `numRequests` HTTPs requests to `serverAddr`.
// Returns the command executed and a callback to start sending requests.
func NewGRPCTLSServer(t *testing.T, addr string, useTLS bool) (*exec.Cmd, context.CancelFunc) {
	serverBin := buildGRPCServerBin(t)
	args := []string{serverBin, "-addr", addr}
	if useTLS {
		args = append(args, "-tls")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	commandLine := strings.Join(args, " ")
	c, _, err := nettestutil.StartCommandCtx(cancelCtx, commandLine)

	require.NoError(t, err)
	return c, cancel
}

const (
	serverSrcPath = "grpc_external_server"
)

// buildGRPCServerBin builds the grpc server binary and returns the path to the binary.
// If the binary is already built (meanly in the CI), it returns the path to the binary.
func buildGRPCServerBin(t *testing.T) string {
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

	tempFile, err := os.CreateTemp("", "grpc_tls_server_build")
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	t.Cleanup(func() {
		_ = os.Remove(tempFile.Name())
	})

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", tempFile.Name(), serverSrcDir)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build grpc server test binary: %s\noutput: %s", err, string(out))

	return tempFile.Name()
}
