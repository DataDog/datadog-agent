// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsutils offers tools to configure the secret feature of the Agent
package secretsutils

import (
	"fmt"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/components/os"
)

// Client is a client that can create and delete files containing secrets
type Client struct {
	t       *testing.T
	rootDir string
	host    *components.RemoteHost

	refreshInterval int
	agentBinary     string
	allowExecGroup  bool
}

// NewClient creates a new Client that can create and delete files containing secrets
func NewClient(t *testing.T, host *components.RemoteHost, rootDir string) *Client {
	t.Log("Creating secret client with root directory", rootDir)

	// WIP We're trying to detect the current OS for the test
	agentBinary := "/opt/datadog-agent/bin/agent/agent"
	if host.OSFamily == os.WindowsFamily {
		agentBinary = "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
	}

	return &Client{
		t:           t,
		rootDir:     rootDir,
		host:        host,
		agentBinary: agentBinary,
	}
}

// SetSecret creates or updates a file containing the secret value
func (c *Client) SetSecret(name string, value string) {
	c.t.Log("Setting secret", name)

	// Create the root directory if it doesn't exist
	err := c.host.MkdirAll(c.rootDir)
	require.NoError(c.t, err)

	fullpath := path.Join(c.rootDir, name)
	_, err = c.host.WriteFile(fullpath, []byte(value))
	require.NoError(c.t, err)
}

// RemoveSecret deletes the file containing the secret
func (c *Client) RemoveSecret(name string) error {
	c.t.Log("Removing secret", name)
	err := c.host.Remove(path.Join(c.rootDir, name))
	return err
}

// ConfigureRefreshInterval set a refresh interval for secrets. This has to be called before GetAgentConfiguration
func (c *Client) ConfigureRefreshInterval(interval int) {
	c.refreshInterval = interval
}

// AllowExecGroup set secret_backend_command_allow_group_exec_perm to true in the agent configuration. This has to be
// called before GetAgentConfiguration
func (c *Client) AllowExecGroup() {
	c.allowExecGroup = true
}

// GetAgentConfiguration returns the Agent configuration dedicated to secrets configured. The result
// is to be injected in the configuration before installing and configuring it.
func (c *Client) GetAgentConfiguration() string {
	conf := fmt.Sprintf(`
# Secret specific configuration

secret_backend_command: %s
secret_backend_arguments:
  - secret-helper
  - read
secret_backend_remove_trailing_line_break: true
secret_backend_command_allow_group_exec_perm: %t
`, c.agentBinary, c.allowExecGroup)
	if c.refreshInterval > 0 {
		conf += fmt.Sprintf("secret_refresh_interval: %d\n", c.refreshInterval)
	}
	return conf
}
