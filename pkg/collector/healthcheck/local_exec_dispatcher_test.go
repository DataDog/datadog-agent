// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package healthcheck

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func TestLocalExecDispatcherSuccess(t *testing.T) {
	cfg := healthConfig()
	cfg.Remediation.Steps[0].Command = "echo remediated"
	out := make(chan event.Event, 2)
	dispatcher := NewLocalExecDispatcher(out, "test-host")
	dispatcher.Dispatch(context.Background(), "local-check", cfg.ServiceCheck, "unhealthy", cfg)
	require.Len(t, out, 2)
	require.Contains(t, (<-out).Tags, "remediation:detected")
	require.Contains(t, (<-out).Tags, "remediation:remediated")
	exitCode, output, err := dispatcher.run(context.Background(), "echo remediated")
	require.NoError(t, err)
	require.Zero(t, exitCode)
	require.Equal(t, "remediated", strings.TrimSpace(output))
}

func TestLocalOutputBuffer(t *testing.T) {
	var output localOutputBuffer
	for _, chunk := range []string{"prefix", strings.Repeat("x", localOutputLimit*10), "discarded"} {
		n, err := output.Write([]byte(chunk))
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
		require.LessOrEqual(t, output.size, localOutputLimit)
	}
	require.Equal(t, "prefix"+strings.Repeat("x", localOutputLimit-len("prefix")), string(output.data[:output.size]))
}

func TestLocalOutputBufferScrubsTruncatedSecret(t *testing.T) {
	secret := "-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("a", localOutputLimit*2) + "\n-----END PRIVATE KEY-----"
	var output localOutputBuffer
	_, err := output.Write([]byte(secret))
	require.NoError(t, err)
	require.Equal(t, "[redacted: output reached capture limit]", scrubForLog(string(output.data[:output.size])))
	require.Equal(t, "remediated", scrubForLog("remediated\n"))
}
