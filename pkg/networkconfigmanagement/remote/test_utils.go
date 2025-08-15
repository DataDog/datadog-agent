// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import "fmt"

// Test/mock implementations

type mockSSHSession struct {
	outputs map[string]string
	closed  bool
	err     error
}

func (m *mockSSHSession) CombinedOutput(cmd string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if output, ok := m.outputs[cmd]; ok {
		return []byte(output), nil
	}
	return nil, fmt.Errorf("command not found: %s", cmd)
}

func (m *mockSSHSession) Close() error {
	m.closed = true
	return nil
}

// MockSSHClient is a mock implementation of the SSHClient interface for testing purposes.
type MockSSHClient struct {
	session      *mockSSHSession
	sessionError error
	closed       bool
}

// NewSession creates a new mock SSH session for testing.
func (t *MockSSHClient) NewSession() (Session, error) {
	if t.sessionError != nil {
		return nil, t.sessionError
	}
	return t.session, nil
}

// RetrieveRunningConfig retrieves the running configuration using mock commands.
func (t *MockSSHClient) RetrieveRunningConfig() (string, error) {
	commands := []string{"show running-config"}
	results, err := t.retrieveConfiguration(commands)
	if err != nil {
		return "", err
	}
	return results[0], nil
}

// RetrieveStartupConfig retrieves the startup configuration using mock commands.
func (t *MockSSHClient) RetrieveStartupConfig() (string, error) {
	commands := []string{"show startup-config"}
	results, err := t.retrieveConfiguration(commands)
	if err != nil {
		return "", err
	}
	return results[0], nil
}

// retrieveConfiguration retrieves the configuration using the provided commands.
func (t *MockSSHClient) retrieveConfiguration(commands []string) ([]string, error) {
	var results []string

	for _, cmd := range commands {
		session, err := t.NewSession()
		if err != nil {
			return nil, fmt.Errorf("failed to create SSH session: %w", err)
		}

		output, err := session.CombinedOutput(cmd)
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("command %s failed: %w", cmd, err)
		}

		results = append(results, string(output))
		session.Close()
	}

	return results, nil
}

// Close closes the mock SSH client.
func (t *MockSSHClient) Close() error {
	t.closed = true
	return nil
}
