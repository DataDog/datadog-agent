// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides utilities for testing the TLS package.
package testutil

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

// NewGoTLSClient triggers an external go tls client that runs `numRequests` HTTPs requests to `serverAddr`.
// Returns the command executed and a callback to start sending requests.
func NewGoTLSClient(t *testing.T, serverAddr string, numRequests int, enableHTTP2 bool) (*exec.Cmd, func()) {
	clientBin := buildGoTLSClientBin(t)
	args := []string{clientBin}
	if enableHTTP2 {
		args = append(args, "-http2")
	}
	// We're using the `flag` library, which requires the flags to be right after the binary name, and before positional arguments.
	args = append(args, serverAddr, strconv.Itoa(numRequests))

	timedCtx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	commandLine := strings.Join(args, " ")
	c, clientInput, err := nettestutil.StartCommandCtx(timedCtx, commandLine)

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
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(curDir, "gotls_client")
	require.NoError(t, err)
	return serverBin
}
