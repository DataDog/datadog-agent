// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

const panOSPrompt = "cwadmin@PRDC-IF01> "

// curlyConfig is a trimmed PAN-OS `show config running` capture in the
// curly-brace (non-XML) format reported in AGENT-16721.
const curlyConfig = `config {
  mgt-config {
    users {
      admin {
        phash $1$ljjdxeva$.isIbumicIMfaHvG/EKqd.;
      }
    }
  }
}
`

// panOSRunningCmd returns the real built-in pan-os GetRunning command so the
// test exercises the actual profile wiring (interactive flag, setup command,
// validator, prompt).
func panOSRunningCmd(t *testing.T) *profile.PlainCommand {
	t.Helper()
	cmd := profile.DefaultProfiles[profile.ProfilePanOS].Commands.GetRunning
	require.NotNil(t, cmd)
	require.True(t, cmd.Interactive, "pan-os GetRunning should be interactive")
	return cmd
}

func TestInteractiveCommand_PanOSCurlyConfig(t *testing.T) {
	srv := StartFakeInteractiveSSHServer(t, panOSPrompt, map[string]FakeResponse{
		"show config running": Ok(curlyConfig),
		"set cli pager off":   Ok(""),
	})
	client := MustConnect(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ExecuteCommand(ctx, client, panOSRunningCmd(t))
	require.NoError(t, err)

	// Config content is present and validates (curly-brace format).
	assert.Contains(t, result.Output, "config {")
	assert.Contains(t, result.Output, "phash")
	// The echoed command line and trailing prompt are stripped.
	assert.NotContains(t, result.Output, "show config running")
	assert.NotContains(t, result.Output, panOSPrompt)

	// The pager was disabled before the command, and we exited cleanly.
	received := srv.Received()
	assert.Contains(t, received, "set cli pager off")
	assert.Contains(t, received, "show config running")
	assert.Contains(t, received, "exit")
	// Setup runs before the command.
	assert.Less(t, indexOf(received, "set cli pager off"), indexOf(received, "show config running"))
}

func TestInteractiveCommand_BannerOnlyFails(t *testing.T) {
	// The device returns to its prompt but produces no output for the command
	// (the AGENT-16721 symptom: only the banner/prompt, no config). The
	// validator should reject the empty output rather than silently passing.
	srv := StartFakeInteractiveSSHServer(t, panOSPrompt, map[string]FakeResponse{
		"show config running": Ok(""),
		"set cli pager off":   Ok(""),
	})
	client := MustConnect(t, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := ExecuteCommand(ctx, client, panOSRunningCmd(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match required regex")
}

func indexOf(s []string, target string) int {
	for i, v := range s {
		if v == target {
			return i
		}
	}
	return -1
}
