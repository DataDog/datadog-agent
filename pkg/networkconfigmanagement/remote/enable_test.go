// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

// scriptedEnableShell simulates a device CLI: it reads the enable command,
// writes a password prompt, reads the password (accepting it only if it
// equals wantPassword), then either rejects it or reads and echoes the real
// command before printing cmdOutput and a trailing device prompt.
func scriptedEnableShell(wantPassword, cmdOutput string, sawCommand *bool, mu *sync.Mutex) InteractiveShellFunc {
	return func(stdin *bufio.Reader, stdout io.Writer) {
		if _, err := stdin.ReadString('\n'); err != nil { // "enable"
			return
		}
		io.WriteString(stdout, "Password: ")
		pwLine, err := stdin.ReadString('\n')
		if err != nil {
			return
		}
		pw := strings.TrimRight(pwLine, "\r\n")
		if pw != wantPassword {
			io.WriteString(stdout, "% Bad passwords\n")
		} else {
			io.WriteString(stdout, "\n")
		}
		// Real devices often stay reachable (just not privileged) after a
		// rejected enable - keep reading so callers that skip validation
		// (no Validator configured) still get to send the real command.
		cmdLine, err := stdin.ReadString('\n')
		if err != nil {
			return
		}
		if mu != nil {
			mu.Lock()
			*sawCommand = true
			mu.Unlock()
		}
		io.WriteString(stdout, cmdLine) // echo, as an interactive pty would
		io.WriteString(stdout, cmdOutput)
		io.WriteString(stdout, "Router#")
		_, _ = stdin.ReadString('\n') // "exit"
	}
}

func TestExecuteCommandWithEnable_Success(t *testing.T) {
	var sawCommand bool
	var mu sync.Mutex
	srv := StartFakeSSHServerWithFunc(t, nil, WithInteractiveShell(scriptedEnableShell("s3cret", "hostname Router1\n", &sawCommand, &mu)))
	client := MustConnect(t, srv)

	enableCmd := &profile.EnableCommand{Command: "enable"}
	cmd := &profile.PlainCommand{Command: "show running-config"}

	result, err := ExecuteCommandWithEnable(context.Background(), client, cmd, enableCmd, "s3cret")
	require.NoError(t, err)
	assert.Equal(t, "hostname Router1\n", result.Output)

	mu.Lock()
	assert.True(t, sawCommand, "real command should have been sent after successful enable")
	mu.Unlock()
}

func TestExecuteCommandWithEnable_PromptNeverAppears(t *testing.T) {
	var sawCommand bool
	var mu sync.Mutex
	// Shell func returns immediately after reading "enable", writing nothing -
	// the channel closes without ever producing a password prompt.
	srv := StartFakeSSHServerWithFunc(t, nil, WithInteractiveShell(func(stdin *bufio.Reader, _ io.Writer) {
		_, _ = stdin.ReadString('\n')
	}))
	client := MustConnect(t, srv)

	enableCmd := &profile.EnableCommand{Command: "enable"}
	cmd := &profile.PlainCommand{Command: "show running-config"}

	_, err := ExecuteCommandWithEnable(context.Background(), client, cmd, enableCmd, "s3cret")
	assert.ErrorContains(t, err, "password prompt not detected")

	mu.Lock()
	assert.False(t, sawCommand)
	mu.Unlock()
}

func TestExecuteCommandWithEnable_ValidatorRejectsWrongPassword(t *testing.T) {
	var sawCommand bool
	var mu sync.Mutex
	srv := StartFakeSSHServerWithFunc(t, nil, WithInteractiveShell(scriptedEnableShell("correct", "should not be seen\n", &sawCommand, &mu)))
	client := MustConnect(t, srv)

	enableCmd := profile.MkEnable("enable", profile.WithEnableReject(`(?i)% ?bad (password|secret)s?`))
	cmd := &profile.PlainCommand{Command: "show running-config"}

	_, err := ExecuteCommandWithEnable(context.Background(), client, cmd, enableCmd, "wrong-password")
	assert.ErrorContains(t, err, "matches failure regex")

	mu.Lock()
	assert.False(t, sawCommand, "real command must never be sent when enable is rejected")
	mu.Unlock()
}

func TestExecuteCommandWithEnable_NoValidatorProceedsAnyway(t *testing.T) {
	// Without a Validator configured, a wrong password can't be detected -
	// enable "succeeds" and the real command is still sent (documents the
	// intentional v1 behavior: configure a Validator to get hard failures).
	var sawCommand bool
	var mu sync.Mutex
	srv := StartFakeSSHServerWithFunc(t, nil, WithInteractiveShell(scriptedEnableShell("correct", "output\n", &sawCommand, &mu)))
	client := MustConnect(t, srv)

	enableCmd := &profile.EnableCommand{Command: "enable"}
	cmd := &profile.PlainCommand{Command: "show running-config"}

	_, err := ExecuteCommandWithEnable(context.Background(), client, cmd, enableCmd, "wrong-password")
	require.NoError(t, err)

	mu.Lock()
	assert.True(t, sawCommand)
	mu.Unlock()
}

func TestExecuteCommandWithEnable_Timeout(t *testing.T) {
	srv := StartFakeSSHServerWithFunc(t, nil, WithInteractiveShell(func(stdin *bufio.Reader, stdout io.Writer) {
		_, _ = stdin.ReadString('\n')
		io.WriteString(stdout, "Password: ")
		time.Sleep(time.Second * 10)
	}))
	client := MustConnect(t, srv)

	enableCmd := &profile.EnableCommand{Command: "enable"}
	cmd := &profile.PlainCommand{Command: "show running-config"}

	ctx, cancel := context.WithTimeout(t.Context(), time.Microsecond)
	defer cancel()
	_, err := ExecuteCommandWithEnable(ctx, client, cmd, enableCmd, "s3cret")
	assert.ErrorContains(t, err, "context deadline exceeded")
}

func TestSSHConnection_UsesEnable(t *testing.T) {
	var sawCommand bool
	var mu sync.Mutex
	srv := StartFakeSSHServerWithFunc(t, nil, WithInteractiveShell(scriptedEnableShell("s3cret", "hostname Router1\n", &sawCommand, &mu)))

	knownHosts := MakeKnownHostsFile(t, srv)
	port, err := strconv.Atoi(srv.Port())
	require.NoError(t, err)
	device := &ncmconfig.DeviceInstance{
		IPAddress: srv.Host(),
		Auth: ncmconfig.AuthCredentials{
			Username:       srv.User(),
			Password:       srv.Password(),
			Port:           strconv.Itoa(port),
			Protocol:       "tcp",
			Enable:         true,
			EnablePassword: "s3cret",
			SSH: &ncmconfig.SSHConfig{
				KnownHostsPath: knownHosts,
			},
		},
	}

	connector, err := NewSSHConnector(device)
	require.NoError(t, err)
	conn, err := connector.Connect()
	require.NoError(t, err)
	conn.SetProfile(&profile.NCMProfile{
		Name: "test-profile",
		Commands: profile.CommandSet{
			GetRunning:    profile.MkCommand("show running-config"),
			EnableCommand: &profile.EnableCommand{Command: "enable"},
		},
	})

	result, err := conn.RetrieveRunningConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "hostname Router1\n", result.Output)

	mu.Lock()
	assert.True(t, sawCommand)
	mu.Unlock()
}

func TestSSHConnection_NoEnableCommand_NoOp(t *testing.T) {
	// Auth.Enable is true, but the profile defines no EnableCommand - the
	// non-interactive ExecuteCommand path should run unaffected.
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show running-config": Ok("hostname Router1\n"),
	})
	device := makeDevice(t, srv)
	device.Auth.Enable = true

	connector, err := NewSSHConnector(device)
	require.NoError(t, err)
	conn, err := connector.Connect()
	require.NoError(t, err)
	conn.SetProfile(&profile.NCMProfile{
		Name: "test-profile",
		Commands: profile.CommandSet{
			GetRunning: profile.MkCommand("show running-config"),
		},
	})

	result, err := conn.RetrieveRunningConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "hostname Router1\n", result.Output)
}
