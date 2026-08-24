// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// startFailingSSHServer starts an in-memory SSH server that accepts
// connections and completes (no-auth) handshakes, but makes every command
// execution fail with a non-zero exit status. It returns the listen address.
func startFailingSSHServer(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)

	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed on cleanup
			}
			go serveFailingConn(conn, cfg)
		}
	}()

	return ln.Addr().String()
}

func serveFailingConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	sConn, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return
	}
	defer sConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func() {
			for req := range chReqs {
				switch req.Type {
				case "exec", "shell":
					if req.WantReply {
						_ = req.Reply(true, nil)
					}
					// Report a non-zero exit status so the client's
					// CombinedOutput returns an *ssh.ExitError.
					_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 1}))
					_ = ch.Close()
				default:
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
				}
			}
		}()
	}
}

func dialSSHClient(t *testing.T, addr string) *ssh.Client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)
	client := ssh.NewClient(c, chans, reqs)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestSshRunCommandReturnsErrorAfterRetries guards against the err
// variable-shadowing regression that made SshRunCommand return (nil, nil)
// when a command failed on every retry (see PR #48951 / commit 7b455c21).
func TestSshRunCommandReturnsErrorAfterRetries(t *testing.T) {
	addr := startFailingSSHServer(t)
	client := dialSSHClient(t, addr)

	output, err := SshRunCommand(client, "false", io.Discard)

	require.Error(t, err, "SshRunCommand must return the command error after exhausting retries, not (nil, nil)")
	require.Nil(t, output)
}
