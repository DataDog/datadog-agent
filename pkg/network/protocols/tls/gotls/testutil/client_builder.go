// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/stretchr/testify/require"
)

// NewGoTLSClient triggers an external go tls client that runs `numRequests` HTTPs requests to `serverAddr`.
// Returns the command executed and a callback to start sending requests.
func NewGoTLSClient(t *testing.T, serverAddr string, numRequests int) (*exec.Cmd, func()) {
	clientBin := buildGoTLSClientBin(t)
	clientCmd := fmt.Sprintf("%s %s %d", clientBin, serverAddr, numRequests)

	timedCtx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	c, clientInput, err := nettestutil.StartCommandCtx(timedCtx, clientCmd)

	require.NoError(t, err)
	return c, func() {
		defer cancel()
		_, err = clientInput.Write([]byte{1})
		require.NoError(t, err)
		err = c.Wait()
		require.NoError(t, err)
	}
}

func buildGoTLSClientBin(t *testing.T) string {
	const ClientSrcPath = "gotls_client"
	const ClientBinaryPath = "gotls_client/gotls_client"

	t.Helper()

	cur, err := testutil.CurDir()
	require.NoError(t, err)

	clientBinary := fmt.Sprintf("%s/%s", cur, ClientBinaryPath)

	// If there is a compiled binary already, skip the compilation.
	// Meant for the CI.
	if _, err = os.Stat(clientBinary); err == nil {
		return clientBinary
	}

	clientSrcDir := fmt.Sprintf("%s/%s", cur, ClientSrcPath)
	clientBuildDir, err := os.MkdirTemp("", "gotls_client_build-")
	require.NoError(t, err)

	t.Cleanup(func() {
		os.RemoveAll(clientBuildDir)
	})

	clientBinPath := fmt.Sprintf("%s/gotls_client", clientBuildDir)

	c := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", clientBinPath, clientSrcDir)
	out, err := c.CombinedOutput()
	require.NoError(t, err, "could not build client test binary: %s\noutput: %s", err, string(out))

	return clientBinPath
}
