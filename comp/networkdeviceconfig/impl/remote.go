// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package networkdeviceconfigimpl

import "golang.org/x/crypto/ssh"

// RemoteClient defines the interface for a remote client that can create sessions to execute commands on a device
type RemoteClient interface {
	NewSession() (RemoteSession, error)
	Close() error
}

// RemoteSession defines the interface for a session that can execute commands on a remote device
type RemoteSession interface {
	CombinedOutput(cmd string) ([]byte, error)
	Close() error
}

// RemoteClientFactory defines the interface for creating remote clients
type RemoteClientFactory interface {
	Connect(ip string, auth AuthCredentials) (RemoteClient, error)
}

// SSHClient implements RemoteClient using SSH
type SSHClient struct {
	client *ssh.Client
}

// NewSession creates a new SSH session for the client (needed for every command execution)
func (r *SSHClient) NewSession() (RemoteSession, error) {
	sess, err := r.client.NewSession()
	if err != nil {
		return nil, err
	}
	return &SSHSession{sess}, nil
}

// Close closes the SSH client connection
func (r *SSHClient) Close() error {
	return r.client.Close()
}

// SSHSession implements RemoteSession using an SSH session
type SSHSession struct {
	session *ssh.Session
}

// CombinedOutput runs a command using the SSH session and returns its output
func (s *SSHSession) CombinedOutput(cmd string) ([]byte, error) {
	return s.session.CombinedOutput(cmd)
}

// Close closes the SSH session
func (s *SSHSession) Close() error {
	return s.session.Close()
}

// SSHClientFactory creates a new SSHClient for SSH connections
type SSHClientFactory struct{}

// Connect establishes a new SSH connection to the specified IP address using the provided authentication credentials
func (f *SSHClientFactory) Connect(ip string, auth AuthCredentials) (RemoteClient, error) {
	client, err := connectToHost(ip, auth)
	if err != nil {
		return nil, err
	}
	return &SSHClient{client: client}, nil
}
