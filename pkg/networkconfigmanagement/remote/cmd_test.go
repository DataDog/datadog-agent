// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

func TestCommand(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show version": Ok("Fakesco fOS\n"),
		"show venison": Fail("bad command", 1),
	})
	client := MustConnect(t, srv)

	for _, tc := range []struct {
		name      string
		cmd       *profile.PlainCommand
		expected  string
		expectErr string
	}{{
		name: "unchecked_command",
		cmd: &profile.PlainCommand{
			Command: "show version",
		},
		expected: "Fakesco fOS\n",
	}, {
		name: "valid_command",
		cmd: &profile.PlainCommand{
			Command: "show version",
			Validator: profile.Validator{
				Require: []*regexp.Regexp{regexp.MustCompile("Fakesco")},
				Reject:  []*regexp.Regexp{regexp.MustCompile("Realco")},
			},
		},
		expected: "Fakesco fOS\n",
	}, {
		name: "missing_req",
		cmd: &profile.PlainCommand{
			Command: "show version",
			Validator: profile.Validator{
				Require: []*regexp.Regexp{regexp.MustCompile("Realco")},
			},
		},
		expectErr: "does not match required regex",
	}, {
		name: "has_rejection",
		cmd: &profile.PlainCommand{
			Command: "show version",
			Validator: profile.Validator{
				Reject: []*regexp.Regexp{regexp.MustCompile("Fakesco")},
			},
		},
		expectErr: "matches failure regex",
	}, {
		name: "command_fails",
		cmd: &profile.PlainCommand{
			Command: "show venison",
			Validator: profile.Validator{
				Require: []*regexp.Regexp{regexp.MustCompile("Fakesco")},
				Reject:  []*regexp.Regexp{regexp.MustCompile("Realco")},
			},
		},
		expectErr: `Process exited with status 1: "bad command"`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExecuteCommand(context.Background(), client, tc.cmd)
			if tc.expectErr != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, result.FormattedError(), tc.expectErr)
			} else {
				require.NoError(t, err)
				assert.Empty(t, result.Error)
				assert.Equal(t, tc.expected, result.Output)
			}
		})
	}
}

func TestSCPCommand(t *testing.T) {
	for _, tc := range []struct {
		name      string
		op        ShellFunc
		validator profile.Validator
		expected  string
		expectErr string
	}{{
		name: "unchecked_command",
		op: func(shell *ShellContext) uint32 {
			shell.stdout.Write([]byte{0, 0})
			// wait for the other end to *finish* writing so we don't close stdin early
			io.Copy(io.Discard, shell.stdin)
			return 0
		},
		expected: "",
	}, {
		name: "failing_from_stdout",
		op: func(shell *ShellContext) uint32 {
			io.WriteString(shell.stdout, "\x01permission denied")
			shell.stdin.ReadByte()
			return 1
		},
		expectErr: "permission denied",
	}, {
		name: "failing_from_stderr",
		op: func(shell *ShellContext) uint32 {
			io.WriteString(shell.stderr, "Unknown command: scp")
			return 0
		},
		expectErr: "Unknown command: scp",
	}, {
		name: "failing_without_feedback",
		op: func(_ *ShellContext) uint32 {
			return 0
		},
		expectErr: "unexpected EOF",
	}, {
		name: "validate_response",
		op: func(shell *ShellContext) uint32 {
			io.WriteString(shell.stdout, "\x00\x00returning feedback")
			// wait for the other end to *finish* writing so we don't close shell.stdin early
			io.Copy(io.Discard, shell.stdin)
			return 0
		},
		validator: profile.Validator{
			Require: []*regexp.Regexp{
				regexp.MustCompile("feedback"),
			},
		},
		expected: "returning feedback",
	}, {
		name: "invalid_response",
		op: func(shell *ShellContext) uint32 {
			io.WriteString(shell.stdout, "\x00\x00incorrect response")
			// wait for the other end to *finish* writing so we don't close shell.stdin early
			io.Copy(io.Discard, shell.stdin)
			return 0
		},
		validator: profile.Validator{
			Require: []*regexp.Regexp{
				regexp.MustCompile("feedback"),
			},
		},
		expectErr: "does not match required regex",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := StartFakeSSHServerWithFunc(t, tc.op)
			client := MustConnect(t, srv)
			cmd := &profile.SCPCommand{
				RemoteCommand: "scp",
				Filepath:      "/mnt/flash/backup.txt",
				Validator:     tc.validator,
			}
			response, err := ExecuteSCP(context.Background(), client, cmd, "this is the data")
			if tc.expectErr != "" {
				assert.ErrorContains(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, response.Output)
			}
		})
	}
}

func TestSCPCommand_Timeout(t *testing.T) {
	srv := StartFakeSSHServerWithFunc(t, func(sc *ShellContext) uint32 {
		io.WriteString(sc.stdout, "\x01some random data but it never closes")
		time.Sleep(time.Second * 10)
		return 0
	})
	client := MustConnect(t, srv)
	ctx, cancel := context.WithTimeout(t.Context(), time.Microsecond)
	defer cancel()
	_, err := ExecuteSCP(ctx, client, &profile.SCPCommand{
		RemoteCommand: "scp",
		Filepath:      "/ignored.txt",
	}, "this is the data")
	assert.ErrorContains(t, err, "context deadline exceeded")
}
