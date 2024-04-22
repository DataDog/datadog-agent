// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/require"
)

// SecretClient is a client that can create and delete files containing secrets
type SecretClient struct {
	t       *testing.T
	rootDir string
	host    *components.RemoteHost
}

// NewSecretClient creates a new SecretClient that can create and delete files containing secrets
func NewSecretClient(t *testing.T, host *components.RemoteHost, rootDir string) *SecretClient {
	return &SecretClient{
		t:       t,
		rootDir: rootDir,
		host:    host,
	}
}

// SetSecret creates a new file containing the secret value
func (c *SecretClient) SetSecret(name string, value string) int64 {
	fullpath := path.Join(c.rootDir, name)
	b, err := c.host.WriteFile(fullpath, []byte(value))
	require.NoError(c.t, err)
	return b
}

// RemoveSecret deletes the file containing the secret
func (c *SecretClient) RemoveSecret(name string) error {
	err := c.host.Remove(path.Join(c.rootDir, name))
	return err
}
