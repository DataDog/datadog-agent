// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// iosLikeSession is a SessionFactory whose sessions track config mode across
// commands and print a prompt after each one in shell mode.
func iosLikeSession(sessions *atomic.Int32) SessionFactory {
	return func(sc *ShellContext) ShellFunc {
		sessions.Add(1)
		prompt := "R1#"
		if sc.mode == modeShell {
			_, _ = io.WriteString(sc.stdout, prompt)
		}
		return func(command string, sc *ShellContext) uint32 {
			var status uint32
			switch command {
			case "", "show version":
			case "configure terminal":
				prompt = "R1(config)#"
			case "end":
				prompt = "R1#"
			default:
				_, _ = io.WriteString(sc.stdout, "% Invalid input\n")
				status = 1
			}
			if sc.mode == modeShell {
				_, _ = io.WriteString(sc.stdout, prompt)
			} else {
				_, _ = io.WriteString(sc.stdout, "ok:"+command)
			}
			return status
		}
	}
}

// expectResponse reads from sh until response arrives, and fails if anything
// else arrived before it.
func expectResponse(t testing.TB, sh Shell, response string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := sh.ReadUntil(ctx, through(response))
	require.NoError(t, err, "got %q so far", out)
	assert.Equal(t, response, string(out))
}

func TestFakeSSHServer_ShellSession(t *testing.T) {
	var sessions atomic.Int32
	srv := StartFakeSSHServerWithFactory(t, iosLikeSession(&sessions))
	sh := connectTestShell(t, srv)
	ctx := t.Context()

	expectResponse(t, sh, "R1#")

	require.NoError(t, sh.Send(ctx, "configure terminal\n"))
	expectResponse(t, sh, "R1(config)#")

	require.NoError(t, sh.Send(ctx, "end\r\n\nbogus\n"))
	expectResponse(t, sh, "R1#R1#% Invalid input\nR1#")

	// Closing stdin ends the session; nothing else should be output.
	require.NoError(t, sh.CloseWrite())
	result, err := sh.ReadUntil(ctx, through("never"))
	assert.Equal(t, "", string(result))

	var exitErr *ssh.ExitError
	require.True(t, errors.As(err, &exitErr), "expected exit error, got %v", err)
	assert.Equal(t, 1, exitErr.ExitStatus())

	assert.Equal(t, []string{"configure terminal", "end", "bogus"}, srv.Received())
	assert.EqualValues(t, 1, sessions.Load())
}

func TestFakeSSHServer_FactoryCalledPerSession(t *testing.T) {
	var sessions atomic.Int32
	srv := StartFakeSSHServerWithFactory(t, iosLikeSession(&sessions))
	client := MustConnect(t, srv)

	for range 2 {
		session, err := client.NewSession()
		require.NoError(t, err)
		out, err := session.Output("show version")
		require.NoError(t, err)
		assert.Equal(t, "ok:show version", string(out))
		session.Close()
	}

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()
	stdin, err := session.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, session.Shell())
	require.NoError(t, stdin.Close())
	require.NoError(t, session.Wait())

	assert.EqualValues(t, 3, sessions.Load())
}
