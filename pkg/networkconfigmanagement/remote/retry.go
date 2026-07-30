// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/ssh"
)

// isTransientSSH checks if the error is transient and can be retried (devices that may only accept a limited number of connections)
func isTransientSSH(err error) bool {
	if err == io.EOF {
		return true
	}
	s := err.Error()
	// TODO where does this list of strings come from?
	return strings.Contains(s, "unexpected packet in response to channel open") ||
		strings.Contains(s, "channel open") ||
		strings.Contains(s, "connection reset by peer")
}

// RetryingSSHClient wraps an ssh.Client, and on NewSession will reconnect if
// the connection appears to have failed. In general this shouldn't happen
// often, but we see it sometimes with devices that take a very long time to
// report their config (especially true when testing against public sandboxes).
// We will only attempt to reconnect once per NewSession call, on the grounds
// that if the other end closes the connection immediately after making it then
// retrying probably won't help.
type RetryingSSHClient struct {
	*ssh.Client
	Reconnect func() (*ssh.Client, error)
}

// NewRetryingSSHClient creates a RetryingSSHClient from a connection function.
// The function will be called immediately, and will be called again if
// NewSession ever returns EOF.
func NewRetryingSSHClient(reconnect func() (*ssh.Client, error)) (*RetryingSSHClient, error) {
	client, err := reconnect()
	if err != nil {
		return nil, err
	}
	return &RetryingSSHClient{
		Client:    client,
		Reconnect: reconnect,
	}, nil
}

// NewSession opens a new Session for this client. (A session is a remote
// execution of a program). If the first attempt to open a session fails because
// the other end has closed the connection, this will attempt to reconnect and
// then will retry.
func (r *RetryingSSHClient) NewSession() (*ssh.Session, error) {
	sess, sessErr := r.Client.NewSession()
	if sessErr != nil {
		if !isTransientSSH(sessErr) {
			return nil, sessErr
		}
		// Try to reconnect
		newClient, clientErr := r.Reconnect()
		if clientErr != nil {
			return nil, fmt.Errorf("new session failed with [%w] and reconnection failed with [%w]", sessErr, clientErr)
		}
		// new connection succeeded, so clean up the old one and try the new one
		_ = r.Client.Close()
		r.Client = newClient
		sess, sessErr = r.Client.NewSession()
		if sessErr != nil {
			// if we have an error immediately after reconnecting then this is
			// probably not a transient thing.
			return nil, sessErr
		}
	}
	return sess, nil
}
