// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package remote

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SSHClient implements Client using SSH
type SSHClient struct {
	client *ssh.Client
}

// SSHSession implements Session using an SSH session
type SSHSession struct {
	session *ssh.Session
}

// SSHClientConfig holds configuration for SSH client
type SSHClientConfig struct {
	Timeout         time.Duration
	HostKeyCallback ssh.HostKeyCallback
}

// SSHClientFactory creates a new SSHClient for SSH connections
type SSHClientFactory struct {
	config *SSHClientConfig
}

// Connect establishes a new SSH connection to the specified IP address using the provided authentication credentials
func (f *SSHClientFactory) Connect(ip string, auth ncmconfig.AuthCredentials) (Client, error) {
	client, err := connectToHost(ip, auth, f.config)
	if err != nil {
		return nil, err
	}
	return &SSHClient{client: client}, nil
}

// DefaultSSHClientConfig returns a default SSH client configuration
func DefaultSSHClientConfig() *SSHClientConfig {
	return &SSHClientConfig{
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // ⚠️ TODO: Use proper host key validation in production
	}
}

// NewSession creates a new SSH session for the client (needed for every command execution)
func (c *SSHClient) NewSession() (Session, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	return &SSHSession{session: sess}, nil
}

// CombinedOutput runs a command using the SSH session and returns its output
func (s *SSHSession) CombinedOutput(cmd string) ([]byte, error) {
	if s.session == nil {
		return nil, fmt.Errorf("SSH session is nil")
	}
	return s.session.CombinedOutput(cmd)
}

// RetrieveRunningConfig retrieves the running configuration for the device connected via SSH
func (c *SSHClient) RetrieveRunningConfig() (string, error) {
	commands := []string{"show running-config"}
	return c.retrieveConfiguration(commands)
}

// RetrieveStartupConfig retrieves the startup configuration for the device connected via SSH
func (c *SSHClient) RetrieveStartupConfig() (string, error) {
	commands := []string{"show startup-config"}
	return c.retrieveConfiguration(commands)
}

// retrieveConfiguration retrieves the configuration for a given network device using multiple commands
func (c *SSHClient) retrieveConfiguration(commands []string) (string, error) {
	var results []string

	for _, cmd := range commands {
		session, err := c.NewSession()
		if err != nil {
			return "", fmt.Errorf("failed to create session for command %s: %w", cmd, err)
		}

		log.Debugf("Executing command: %s", cmd)
		output, err := session.CombinedOutput(cmd)
		session.Close()

		if err != nil {
			return "", fmt.Errorf("command %s failed: %w", cmd, err)
		}

		results = append(results, string(output))
	}

	return strings.Join(results, "\n"), nil
}

// Close closes the SSH client connection
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Close closes the SSH session
func (s *SSHSession) Close() error {
	return s.session.Close()
}

// connectToHost establishes an SSH connection to the specified IP address using the provided authentication credentials
func connectToHost(ipAddress string, auth ncmconfig.AuthCredentials, config *SSHClientConfig) (*ssh.Client, error) {
	if config == nil {
		config = DefaultSSHClientConfig()
	}

	sshConfig := &ssh.ClientConfig{
		User:            auth.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(auth.Password)},
		HostKeyCallback: config.HostKeyCallback,
		Timeout:         config.Timeout,
	}

	// TODO: Add support for SSH key authentication

	host := fmt.Sprintf("%s:%s", ipAddress, auth.Port)
	client, err := ssh.Dial(auth.Protocol, host, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", host, err)
	}

	return client, nil
}
