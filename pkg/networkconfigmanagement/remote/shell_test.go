// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// through returns a matcher that matches through the first occurrence of s.
func through(s string) func([]byte) int {
	return func(buf []byte) int {
		i := bytes.Index(buf, []byte(s))
		if i < 0 {
			return -1
		}
		return i + len(s)
	}
}

func startTestShell(t *testing.T, factory SessionFactory) Shell {
	t.Helper()
	return connectTestShell(t, StartFakeSSHServerWithFactory(t, factory))
}

func connectTestConn(t *testing.T, srv *FakeSSHServer) *SSHConnection {
	t.Helper()
	knownHosts := MakeKnownHostsFile(t, srv)
	client, err := NewRetryingSSHClient(func() (*ssh.Client, error) {
		return srv.Dial(knownHosts)
	})
	require.NoError(t, err)
	conn := &SSHConnection{client: client, ipAddress: srv.Host()}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func connectTestShell(t *testing.T, srv *FakeSSHServer) Shell {
	t.Helper()
	conn := connectTestConn(t, srv)
	sh, terr := conn.StartShell(t.Context())
	require.NoError(t, terr)
	t.Cleanup(func() { _ = sh.Close() })
	return sh
}

func TestShell_SendAndReadUntil(t *testing.T) {
	var sessions atomic.Int32
	sh := startTestShell(t, iosLikeSession(&sessions))
	ctx := t.Context()

	out, err := sh.ReadUntil(ctx, through("#"))
	require.NoError(t, err)
	assert.Equal(t, "R1#", string(out))

	require.NoError(t, sh.Send(ctx, "configure terminal\n"))
	out, err = sh.ReadUntil(ctx, through("#"))
	require.NoError(t, err)
	assert.Equal(t, "R1(config)#", string(out))

	require.NoError(t, sh.Send(ctx, "end\n"))
	out, err = sh.ReadUntil(ctx, through("#"))
	require.NoError(t, err)
	assert.Equal(t, "R1#", string(out))

	assert.EqualValues(t, 1, sessions.Load())
}

func TestShell_LeftoverOutputStaysBuffered(t *testing.T) {
	sh := startTestShell(t, func(sc *ShellContext) ShellFunc {
		_, _ = io.WriteString(sc.stdout, "banner\nR1#")
		return func(string, *ShellContext) uint32 { return 0 }
	})
	ctx := t.Context()

	out, err := sh.ReadUntil(ctx, through("\n"))
	require.NoError(t, err)
	assert.Equal(t, "banner\n", string(out))

	out, err = sh.ReadUntil(ctx, through("#"))
	require.NoError(t, err)
	assert.Equal(t, "R1#", string(out))
}

func TestShell_ReadUntilTimeout(t *testing.T) {
	var sessions atomic.Int32
	sh := startTestShell(t, iosLikeSession(&sessions))

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	out, err := sh.ReadUntil(ctx, through("never"))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "R1#", string(out))

	// The output returned with the timeout is still buffered.
	out, err = sh.ReadUntil(t.Context(), through("#"))
	require.NoError(t, err)
	assert.Equal(t, "R1#", string(out))
}

func TestShell_ReadUntilAfterServerExits(t *testing.T) {
	sh := startTestShell(t, func(sc *ShellContext) ShellFunc {
		_, _ = io.WriteString(sc.stdout, "R1#")
		return func(_ string, sc *ShellContext) uint32 {
			_, _ = io.WriteString(sc.stdout, "bye")
			return 0
		}
	})
	ctx := t.Context()

	_, err := sh.ReadUntil(ctx, through("#"))
	require.NoError(t, err)

	// Closing stdin makes the fake server end the session after the last
	// command.
	require.NoError(t, sh.Send(ctx, "exit\n"))
	require.NoError(t, sh.CloseWrite())

	out, err := sh.ReadUntil(ctx, through("never"))
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, "bye", string(out))
}

func TestShell_ReadUntilAfterClose(t *testing.T) {
	var sessions atomic.Int32
	sh := startTestShell(t, iosLikeSession(&sessions))

	require.NoError(t, sh.Close())
	require.NoError(t, sh.Close()) // idempotent

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := sh.ReadUntil(ctx, through("never"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestStartShell_CanceledContext(t *testing.T) {
	var requests atomic.Int32
	srv := StartFakeSSHServerWithFunc(t, FakeData(nil),
		WithRequestHook(func(string) { requests.Add(1) }))
	conn := connectTestConn(t, srv)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sh, err := conn.StartShell(ctx)
	assert.Nil(t, sh)
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, requests.Load(), "no session requests should reach the server")
}

func TestStartShell_ContextEndsDuringSetup(t *testing.T) {
	shellRequested := make(chan struct{})
	testEnded := make(chan struct{})
	// When the client sends the initial request for a pty, the server will
	// close shellRequested and then wait until testEnded is closed. This
	// ensures that the SSH session has been created but the handshake will
	// stall until after the context is canceled.
	srv := StartFakeSSHServerWithFunc(t, FakeData(nil),
		WithRequestHook(func(reqType string) {
			if reqType == "shell" {
				close(shellRequested)
				<-testEnded
			}
		}))
	// Registered after the server so it runs before the server's Stop.
	t.Cleanup(func() { close(testEnded) })
	conn := connectTestConn(t, srv)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*1)
	// defer this just to be safe; we'll be canceling it ourselves.
	defer cancel()
	// Wait for the shell request to be reached, then cancel the context.
	go func() {
		<-shellRequested
		cancel()
	}()
	sh, err := conn.StartShell(ctx)
	assert.Nil(t, sh)
	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-shellRequested:
	default:
		t.Fatal("cancellation should have happened while the pty request was pending")
	}
}
