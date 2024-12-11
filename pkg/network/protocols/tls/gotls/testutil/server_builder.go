// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package testutil provides utilities for testing the TLS package.
package testutil

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	nettestutil "github.com/DataDog/datadog-agent/pkg/network/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

// NewGoTLSServer starts a GoTLS HTTP server listening on `serverAddr`.
// Returns the `exec.Cmd` handle for the server.
func NewGoTLSServer(t *testing.T, serverAddr string) *exec.Cmd {
	serverBin := buildGoTLSServerBin(t)
	args := []string{serverBin, serverAddr}

	timedCtx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	commandLine := strings.Join(args, " ")

	c, _, err := nettestutil.StartCommandCtx(timedCtx, commandLine)
	require.NoError(t, err)

	t.Cleanup(func() {
		defer cancel()
		err := c.Process.Kill()
		require.NoError(t, err)
		err = c.Wait()
		require.ErrorContains(t, err, "killed")
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
