// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package testutil provides utilities for testing the TLS package.
package testutil

import (
	"context"
	"github.com/stretchr/testify/require"
	"os/exec"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

// NewGoTLSServer starts a GoTLS HTTP server listening on `serverAddr`.
// Returns the `exec.Cmd` handle for the server.
func NewGoTLSServer(t *testing.T, serverAddr string) *exec.Cmd {
	serverBin := buildGoTLSServerBin(t)
	args := []string{serverBin, serverAddr}

	ctx, cancel := context.WithCancel(context.Background())
	commandLine := strings.Join(args, " ")

	c := exec.CommandContext(ctx, commandLine)
	require.NoError(t, c.Start())

	t.Cleanup(func() {
		cancel()
		require.NoError(t, c.Process.Kill())
		require.ErrorContains(t, c.Wait(), "killed")
	})

	return c
}

func buildGoTLSServerBin(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(curDir, "gotls_server")
	require.NoError(t, err)
	return serverBin
}
