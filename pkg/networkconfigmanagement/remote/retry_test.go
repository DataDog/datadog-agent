// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package remote

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestRetryingSSHClient_InitialDialFails(t *testing.T) {
	errBoom := errors.New("dial boom")
	_, err := NewRetryingSSHClient(func() (*ssh.Client, error) {
		return nil, errBoom
	})
	require.ErrorIs(t, err, errBoom)
}

func TestRetryingSSHClient_NewSession_Success(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show version": Ok("Cisco IOS\n"),
	})
	hostfile := MakeKnownHostsFile(t, srv)

	var dials int
	reconnect := func() (*ssh.Client, error) {
		dials++
		return srv.Dial(hostfile)
	}

	r, err := NewRetryingSSHClient(reconnect)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	require.Equal(t, 1, dials, "constructor should dial once")

	sess, err := r.NewSession()
	require.NoError(t, err)
	defer sess.Close()

	out, err := sess.CombinedOutput("show version")
	require.NoError(t, err)
	assert.Equal(t, "Cisco IOS\n", string(out))
	assert.Equal(t, 1, dials, "no reconnect when first session works")
}

func TestRetryingSSHClient_NewSession_ReconnectsOnTransientError(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{
		"show version": Ok("from-srv1\n"),
	})
	hostfile := MakeKnownHostsFile(t, srv)
	config, err := srv.MakeConfig(hostfile)
	require.NoError(t, err)
	var dials int
	reconnect := func() (*ssh.Client, error) {
		dials++
		return ssh.Dial("tcp", srv.Addr(), config)
	}

	r, err := NewRetryingSSHClient(reconnect)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	// Stop the original server and wait until the client observes the
	// disconnect. Once Wait returns, the inner mux is closed and the next
	// NewSession returns io.EOF, which isTransientSSH recognises.
	srv.Stop()
	_ = r.Client.Wait()
	r.Client.Wait()

	// Stand up a fresh server and re-point the device at it so the reconnect
	// has somewhere to land rather than racing the now-dead address.
	srv = StartFakeSSHServer(t, map[string]FakeResponse{
		"show version": Ok("from-srv2\n"),
	})
	config, err = srv.MakeConfig(MakeKnownHostsFile(t, srv))
	require.NoError(t, err)

	sess, err := r.NewSession()
	require.NoError(t, err)
	defer sess.Close()

	out, err := sess.CombinedOutput("show version")
	require.NoError(t, err)
	assert.Equal(t, "from-srv2\n", string(out), "session should be served by the reconnect target")
	assert.Equal(t, 2, dials, "expected one reconnect after transient error")
}

func TestRetryingSSHClient_NewSession_ReconnectFails(t *testing.T) {
	srv := StartFakeSSHServer(t, map[string]FakeResponse{})
	hostfile := MakeKnownHostsFile(t, srv)

	errReconnect := errors.New("dial again boom")
	var dials int
	reconnect := func() (*ssh.Client, error) {
		dials++
		if dials == 1 {
			return srv.Dial(hostfile)
		}
		return nil, errReconnect
	}

	r, err := NewRetryingSSHClient(reconnect)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	// stop the server so that NewSession will get an EOF and retry
	srv.Stop()
	_ = r.Client.Wait()

	_, err = r.NewSession()
	require.Error(t, err)
	assert.ErrorIs(t, err, errReconnect)
	assert.Contains(t, err.Error(), "reconnection failed")
	assert.Equal(t, 2, dials)
}
