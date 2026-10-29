// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"bufio"
	"errors"
	"io"
	"sync/atomic"
	"testing"

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

func expectResponse(t testing.TB, r io.Reader, response string) {
	t.Helper()
	buf := make([]byte, len(response))
	n, err := r.Read(buf)
	require.NoError(t, err)
	if assert.Equal(t, n, len(buf)) {
		if response == string(buf) {
			return
		}
		rest, err := io.ReadAll(r)
		require.NoError(t, err)
		assert.Equal(t, response, string(buf)+string(rest))
		return
	}
}

func TestFakeSSHServer_ShellSession(t *testing.T) {
	var sessions atomic.Int32
	srv := StartFakeSSHServerWithFactory(t, iosLikeSession(&sessions))
	client := MustConnect(t, srv)

	session, err := client.NewSession()
	require.NoError(t, err)
	defer session.Close()

	require.NoError(t, session.RequestPty("vt100", 40, 80, ssh.TerminalModes{}))
	stdin, err := session.StdinPipe()
	require.NoError(t, err)

	stdoutPipe, err := session.StdoutPipe()
	require.NoError(t, err)
	stdout := bufio.NewReader(stdoutPipe)
	require.NoError(t, session.Shell())

	expectResponse(t, stdout, "R1#")

	_, err = io.WriteString(stdin, "configure terminal\n")
	require.NoError(t, err)
	expectResponse(t, stdout, "R1(config)#")

	_, err = io.WriteString(stdin, "end\r\n\nbogus\n")
	require.NoError(t, err)
	expectResponse(t, stdout, "R1#R1#% Invalid input\nR1#")

	require.NoError(t, stdin.Close())
	result, err := io.ReadAll(stdout)
	require.NoError(t, err)
	assert.Equal(t, "", string(result))

	err = session.Wait()
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
