// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

// These tests exercise the real SSHClient against the in-process fake SSH
// server defined in testserver.go. They cover paths that the previous
// mock-based tests skipped: SSH handshake, host key verification, password
// auth, real per-command session lifecycle, profile-driven command dispatch
// + output validation, and exit-status error propagation.

package remote

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// runningProfile builds a minimal profile with a single Running command list
// and an optional validation pattern.
func runningProfile(commands []string, validation *regexp.Regexp) *profile.NCMProfile {
	cmd := &profile.Commands{
		CommandType: profile.Running,
		Values:      commands,
	}
	if validation != nil {
		cmd.ProcessingRules = profile.ProcessingRules{
			ValidationRules: []profile.ValidationRule{{Pattern: validation}},
		}
	}
	return &profile.NCMProfile{
		BaseProfile: profile.BaseProfile{Name: "test"},
		Commands: map[profile.CommandType]*profile.Commands{
			profile.Running: cmd,
		},
	}
}

// startupProfile is the Startup-typed equivalent of runningProfile.
func startupProfile(commands []string) *profile.NCMProfile {
	return &profile.NCMProfile{
		BaseProfile: profile.BaseProfile{Name: "test"},
		Commands: map[profile.CommandType]*profile.Commands{
			profile.Startup: {
				CommandType: profile.Startup,
				Values:      commands,
			},
		},
	}
}

func TestSSHClient_RetrieveRunningConfig_AgainstFakeServer(t *testing.T) {
	const expected = `Building configuration...
hostname Router1
end
`
	srv := startFakeSSHServer(t, map[string]fakeResponse{
		"show running-config": ok(expected),
	})

	client, err := NewSSHClient(srv.DeviceInstance(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Connect())
	client.SetProfile(runningProfile(
		[]string{"show running-config"},
		regexp.MustCompile("Building configuration"),
	))

	got, err := client.RetrieveRunningConfig()
	require.NoError(t, err)

	// retrieveConfiguration appends a trailing '\n' between/after each command.
	assert.Equal(t, expected+"\n", string(got))
	assert.Equal(t, []string{"show running-config"}, srv.Received())
}

func TestSSHClient_RetrieveStartupConfig_AgainstFakeServer(t *testing.T) {
	const expected = `! Last configuration change at 10:00:00 UTC
hostname Router1
end
`
	srv := startFakeSSHServer(t, map[string]fakeResponse{
		"show startup-config": ok(expected),
	})

	client, err := NewSSHClient(srv.DeviceInstance(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Connect())
	client.SetProfile(startupProfile([]string{"show startup-config"}))

	got, err := client.RetrieveStartupConfig()
	require.NoError(t, err)

	// RetrieveStartupConfig currently calls retrieveConfiguration twice (once
	// for the inner result, once for the returned bytes); see ssh.go. The
	// fake server should record the command twice as a result.
	assert.Contains(t, string(got), "hostname Router1")
	assert.Equal(t, []string{"show startup-config", "show startup-config"}, srv.Received())
}

func TestSSHClient_RetrieveRunningConfig_MultiCommand(t *testing.T) {
	srv := startFakeSSHServer(t, map[string]fakeResponse{
		"show version":        ok("Cisco IOS Software, Version 15.1\n"),
		"show running-config": ok("Building configuration...\nhostname R1\nend\n"),
	})

	client, err := NewSSHClient(srv.DeviceInstance(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Connect())
	client.SetProfile(runningProfile(
		[]string{"show version", "show running-config"},
		nil,
	))

	got, err := client.RetrieveRunningConfig()
	require.NoError(t, err)

	assert.Contains(t, string(got), "Cisco IOS Software, Version 15.1")
	assert.Contains(t, string(got), "hostname R1")
	// Order is preserved and each command runs on its own session.
	assert.Equal(t, []string{"show version", "show running-config"}, srv.Received())
}

func TestSSHClient_RetrieveRunningConfig_FailsValidation(t *testing.T) {
	srv := startFakeSSHServer(t, map[string]fakeResponse{
		"show running-config": ok("this output does not contain the marker\n"),
	})

	client, err := NewSSHClient(srv.DeviceInstance(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Connect())
	client.SetProfile(runningProfile(
		[]string{"show running-config"},
		regexp.MustCompile("Building configuration"),
	))

	_, err = client.RetrieveRunningConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid output")
	// The command did execute on the wire — validation happens client-side.
	assert.Equal(t, []string{"show running-config"}, srv.Received())
}

// TestSSHClient_CommandExecutionFailure replaces the mock-based
// TestSSHClient_RetrieveConfig_CommandExecutionFailure: the server returns a
// non-zero exit status, which surfaces as an *ssh.ExitError on the client and
// is wrapped into "command %s failed: ..." by retrieveConfiguration.
func TestSSHClient_CommandExecutionFailure(t *testing.T) {
	srv := startFakeSSHServer(t, map[string]fakeResponse{
		"show running-config": fail("permission denied\n", 1),
	})

	client, err := NewSSHClient(srv.DeviceInstance(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Connect())
	client.SetProfile(runningProfile([]string{"show running-config"}, nil))

	_, err = client.RetrieveRunningConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command show running-config failed")
}

// TestSSHClient_SessionCreationFailure replaces the mock-based
// TestSSHClient_RetrieveConfig_SessionCreationFailure: stop the server
// post-Connect so the next NewSession fails (and redial fails because nothing
// is listening on the port any more).
func TestSSHClient_SessionCreationFailure(t *testing.T) {
	srv := startFakeSSHServer(t, map[string]fakeResponse{
		"show running-config": ok("ignored\n"),
	})

	client, err := NewSSHClient(srv.DeviceInstance(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Connect())
	client.SetProfile(runningProfile([]string{"show running-config"}, nil))

	// Tear the server down: closes the listener and every accepted conn.
	// NewSession will fail; redial will hit a refused connection.
	srv.Stop()

	_, err = client.RetrieveRunningConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create session")
}

func TestSSHClient_Connect_RejectsBadCredentials(t *testing.T) {
	srv := startFakeSSHServer(t, nil, withCredentials("expected-user", "expected-pass"))

	dev := srv.DeviceInstance(t)
	dev.Auth.Username = "wrong-user"
	dev.Auth.Password = "wrong-pass"

	client, err := NewSSHClient(dev)
	require.NoError(t, err)

	err = client.Connect()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}
