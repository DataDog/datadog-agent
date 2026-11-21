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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/profile"
)

func TestCommand(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show version": Ok("Fakesco fOS\n"),
		"show venison": Fail("bad command", 1),
	})
	conn := connectTestConn(t, srv)

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
		expectErr: `Process exited with status 1`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ExecuteCommand(t.Context(), conn, tc.cmd)
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
		op: func(_ string, shell *ShellContext) uint32 {
			shell.stdout.Write([]byte{0, 0})
			// wait for the other end to *finish* writing so we don't close stdin early
			io.Copy(io.Discard, shell.stdin)
			return 0
		},
		expected: "",
	}, {
		name: "failing_from_stdout",
		op: func(_ string, shell *ShellContext) uint32 {
			io.WriteString(shell.stdout, "\x01permission denied")
			shell.stdin.ReadByte()
			return 1
		},
		expectErr: "permission denied",
	}, {
		name: "failing_from_stderr",
		op: func(_ string, shell *ShellContext) uint32 {
			io.WriteString(shell.stderr, "Unknown command: scp")
			return 0
		},
		expectErr: "Unknown command: scp",
	}, {
		name: "failing_without_feedback",
		op: func(_ string, _ *ShellContext) uint32 {
			return 0
		},
		expectErr: "unexpected EOF",
	}, {
		name: "validate_response",
		op: func(_ string, shell *ShellContext) uint32 {
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
		op: func(_ string, shell *ShellContext) uint32 {
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
			response, err := ExecuteSCP(t.Context(), client, cmd, "this is the data")
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
	// The handler holds the session open until the test is done, so only ctx
	// can end the copy. The deferred close runs before the server's cleanup.
	done := make(chan struct{})
	defer close(done)
	srv := StartFakeSSHServerWithFunc(t, func(_ string, sc *ShellContext) uint32 {
		io.WriteString(sc.stdout, "\x01some random data but it never closes")
		<-done
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

func TestPagerCommand(t *testing.T) {
	config := "ASA Version 9.24(1)\nhostname FW01\n"
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"terminal pager 0":           Ok(""),
		"more system:running-config": Ok(config),
	}, WithPrompt("fw01# "), WithEcho())
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{
		Command:     "more system:running-config",
		Interactive: true,
		Validator: profile.Validator{
			Require: []*regexp.Regexp{
				regexp.MustCompile(`ASA Version \d+\.\d+\(\d+\)`),
			},
			Reject: []*regexp.Regexp{
				regexp.MustCompile(`(?i)(<---\s*More\s*--->|--More--)`),
			},
		},
	}

	result, err := ExecuteCommand(t.Context(), conn, cmd)
	require.NoError(t, err)
	assert.Empty(t, result.Error)
	assert.Equal(t, strings.TrimRight(config, "\n"), result.Output)
	assert.Equal(t, []string{"terminal pager 0", "more system:running-config", "exit"}, srv.Received())
}

func TestPagerCommand_SetupAppliesToMainCommand(t *testing.T) {
	config := "ASA Version 9.24(1)\n: Saved\nhostname FW01\ninterface G0/0\n: end\n"
	srv := StartFakeSSHServerWithFactory(t, FakeASA(config, 2),
		WithBanner("User datadog logged in to cml-asav\n"),
		WithPrompt("cml-asav# "),
		WithEcho(),
	)
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{
		Command:     "more system:running-config",
		Interactive: true,
		Validator: profile.Validator{
			Require: []*regexp.Regexp{regexp.MustCompile(`ASA Version \d+\.\d+\(\d+\)`)},
			Reject:  []*regexp.Regexp{regexp.MustCompile(`(?i)(<---\s*More\s*--->|--More--)`)},
		},
	}

	result, err := ExecuteCommand(t.Context(), conn, cmd)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimRight(config, "\n"), result.Output)
	assert.NotContains(t, result.Output, "<--- More --->")
	assert.NotContains(t, result.Output, "logged in", "banner must not land in the config")
	assert.NotContains(t, result.Output, "terminal pager 0", "setup echo must not land in the config")
}

func TestPagerCommand_SeparateSessionsTruncate(t *testing.T) {
	config := "ASA Version 9.24(1)\n: Saved\nhostname FW01\n: end\n"
	srv := StartFakeSSHServerWithFactory(t, FakeASA(config, 2))
	conn := connectTestConn(t, srv)

	pager := &profile.PlainCommand{Command: "terminal pager 0"}
	_, err := ExecuteCommand(t.Context(), conn, pager)
	require.NoError(t, err)

	running := &profile.PlainCommand{Command: "more system:running-config"}
	result, err := ExecuteCommand(t.Context(), conn, running)
	require.NoError(t, err)
	assert.Contains(t, result.Output, "<--- More --->")
}

func TestPagerCommand_ClosedContextRetainsOutput(t *testing.T) {
	// The handler never prints the closing prompt while the test runs, so only
	// ctx can end the read. The deferred close runs before the server's cleanup.
	done := make(chan struct{})
	defer close(done)
	ctx, cancel := context.WithCancel(t.Context())
	srv := StartFakeSSHServerWithFunc(t, func(cmd string, sc *ShellContext) uint32 {
		if cmd == "terminal pager 0" {
			return 0
		}
		io.WriteString(sc.stdout, "some output")
		// give the monitor a fraction of a second to reach the output
		time.Sleep(time.Millisecond)
		cancel()
		<-done
		return 0
	}, WithPrompt("fw01# "), WithEcho())
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{
		Command:     "more system:running-config",
		Interactive: true,
	}
	result, err := ExecuteCommand(ctx, conn, cmd)
	assert.Equal(t, result.Output, "some output")
	assert.ErrorContains(t, err, "context canceled")
}

// TestPagerCommand_ReconnectRerunsSetup ensures that when the connection is
// replaced mid-collection, the setup commands are re-applied on the new
// connection rather than being stranded on the old one.
func TestPagerCommand_ReconnectRerunsSetup(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"terminal pager 0":           Ok(""),
		"more system:running-config": Ok("from-srv1\n"),
	}, WithPrompt("fw01# "), WithEcho())
	config, err := srv.MakeConfig(MakeKnownHostsFile(t, srv))
	require.NoError(t, err)
	reconnect := func() (*ssh.Client, error) {
		return ssh.Dial("tcp", srv.Addr(), config)
	}

	r, err := NewRetryingSSHClient(reconnect)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	// Kill the first connection so the initial attempt fails transiently and
	// Pinned must reconnect.
	srv.Stop()
	_ = r.Client.Wait()

	// The reconnect lands on a fresh server: the pager setup must be re-applied
	// here, on the same connection that serves the config.
	srv = StartFakeSSHServer(t, map[string]FakeResponse{
		"terminal pager 0":           Ok(""),
		"more system:running-config": Ok("from-srv2\n"),
	}, WithPrompt("fw01# "), WithEcho())
	config, err = srv.MakeConfig(MakeKnownHostsFile(t, srv))
	require.NoError(t, err)

	cmd := &profile.PlainCommand{
		Command:     "more system:running-config",
		Interactive: true,
	}

	result, err := ExecuteCommand(t.Context(), &SSHConnection{client: r, ipAddress: srv.Host(), prof: asaProfile}, cmd)
	require.NoError(t, err)
	assert.Equal(t, "from-srv2", result.Output)
	assert.Equal(t, []string{"terminal pager 0", "more system:running-config", "exit"}, srv.Received())
}

func TestPagerCommand_EagerEcho(t *testing.T) {
	config := "ASA Version 9.24(1)\n: Saved\nhostname FW01\ninterface G0/0\n: end\n"
	srv := StartFakeSSHServerWithFactory(t, FakeASA(config, 2),
		WithBanner("User datadog logged in to cml-asav\n"),
		WithPrompt("cml-asav# "),
		WithEagerEcho(),
	)
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{
		Command:     "more system:running-config",
		Interactive: true,
		Validator: profile.Validator{
			Require: []*regexp.Regexp{regexp.MustCompile(`ASA Version \d+\.\d+\(\d+\)`)},
			Reject:  []*regexp.Regexp{regexp.MustCompile(`(?i)(<---\s*More\s*--->|--More--)`)},
		},
	}

	result, err := ExecuteCommand(t.Context(), conn, cmd)
	require.NoError(t, err)
	assert.Equal(t, strings.TrimRight(config, "\n"), result.Output)
	assert.NotContains(t, result.Output, "exit", "the session-end echo must not reach the capture")
	assert.Equal(t, []string{"terminal pager 0", "more system:running-config", "exit"}, srv.Received())
}

var asaProfile = &profile.NCMProfile{Name: profile.ProfileCiscoASA}

func connectASAConn(t *testing.T, srv *FakeSSHServer) *SSHConnection {
	t.Helper()
	conn := connectTestConn(t, srv)
	conn.SetProfile(asaProfile)
	return conn
}

func TestInteractiveCommand_NoDialectForProfile(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show running-config": Ok("hostname sw01\n"),
	}, WithPrompt("sw01# "), WithEcho())
	conn := connectTestConn(t, srv)
	conn.SetProfile(&profile.NCMProfile{Name: profile.ProfileEOS})

	cmd := &profile.PlainCommand{Command: "show running-config", Interactive: true}
	_, err := ExecuteCommand(t.Context(), conn, cmd)
	assert.ErrorContains(t, err, `no interactive shell defined for profile "eos"`)
	assert.Empty(t, srv.Received(), "nothing should be sent without a dialect")
}

func TestInteractiveCommand_NoProfile(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{}, WithPrompt("fw01# "), WithEcho())
	conn := connectTestConn(t, srv)

	cmd := &profile.PlainCommand{Command: "more system:running-config", Interactive: true}
	_, err := ExecuteCommand(t.Context(), conn, cmd)
	assert.ErrorContains(t, err, "no device type provided")
}

func TestPagerCommand_RejectedSetupFails(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"terminal pager 0":           Fail("ERROR: % Invalid input detected at '^' marker.\n", 1),
		"more system:running-config": Ok("ASA Version 9.24(1)\n"),
	}, WithPrompt("fw01# "), WithEcho())
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{Command: "more system:running-config", Interactive: true}
	_, err := ExecuteCommand(t.Context(), conn, cmd)
	assert.ErrorContains(t, err, `running "terminal pager 0": ERROR: % Invalid input`)
	assert.Equal(t, []string{"terminal pager 0"}, srv.Received())
}

func TestPagerCommand_RejectedCommandFails(t *testing.T) {
	srv := StartFakeSSHServerWithFactory(t, FakeASA("ASA Version 9.24(1)\n", 0),
		WithPrompt("fw01# "), WithEcho())
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{Command: "show venison", Interactive: true}
	result, err := ExecuteCommand(t.Context(), conn, cmd)
	assert.ErrorContains(t, err, "ERROR: % Invalid input")
	require.NotNil(t, result)
	assert.Contains(t, result.Error, "ERROR: % Invalid input")
}

func TestPagerCommand_ShellClosesBeforePrompt(t *testing.T) {
	srv := StartFakeSSHServerWithFactory(t, func(*ShellContext) ShellFunc {
		return func(command string, shell *ShellContext) uint32 {
			if strings.TrimSpace(command) == "more system:running-config" {
				// Hang up partway through the config, before the next prompt.
				_, _ = io.WriteString(shell.stdout, "ASA Version 9.24(1)\nhostname FW01\n")
				_ = shell.stdout.(io.Closer).Close()
			}
			return 0
		}
	}, WithPrompt("fw01# "), WithEcho())
	conn := connectASAConn(t, srv)

	cmd := &profile.PlainCommand{
		Command:     "more system:running-config",
		Interactive: true,
		Validator: profile.Validator{
			Require: []*regexp.Regexp{regexp.MustCompile(`ASA Version \d+\.\d+\(\d+\)`)},
		},
	}
	result, err := ExecuteCommand(t.Context(), conn, cmd)
	assert.ErrorContains(t, err, `no cisco-asa prompt after "more system:running-config", output may be incomplete`)
	require.NotNil(t, result)
	assert.Equal(t, "ASA Version 9.24(1)\nhostname FW01", result.Output)
}
