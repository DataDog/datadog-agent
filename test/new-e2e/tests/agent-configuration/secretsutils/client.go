// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsutils

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// Client is a client that can create and delete files containing secrets
type Client struct {
	t       *testing.T
	rootDir string
	host    *components.RemoteHost
}

// NewClient creates a new Client that can create and delete files containing secrets
func NewClient(t *testing.T, host *components.RemoteHost, rootDir string) *Client {
	t.Log("Creating secret client with root directory", rootDir)
	return &Client{
		t:       t,
		rootDir: rootDir,
		host:    host,
	}
}

// SetSecret creates or updates a file containing the secret value
func (c *Client) SetSecret(name string, value string) int64 {
	c.t.Log("Setting secret", name)

	// Create the root directory if it doesn't exist
	err := c.host.MkdirAll(c.rootDir)
	require.NoError(c.t, err)

	fullpath := path.Join(c.rootDir, name)
	b, err := c.host.WriteFile(fullpath, []byte(value))
	require.NoError(c.t, err)
	return b
}

// RemoveSecret deletes the file containing the secret
func (c *Client) RemoveSecret(name string) error {
	c.t.Log("Removing secret", name)
	err := c.host.Remove(path.Join(c.rootDir, name))
	return err
}
