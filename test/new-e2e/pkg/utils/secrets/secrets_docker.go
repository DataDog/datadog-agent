// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/docker/docker/api/types/container"
)

type DockerClient struct {
	t       *testing.T
	rootDir string
	docker  *client.Docker

	refreshInterval int
	agentBinary     string
	allowExecGroup  bool
	containerName   string
}

// NewDockerClient creates a new DockerClient that can create and delete files containing secrets
func NewDockerClient(t *testing.T, d *client.Docker, rootDir string, containerName string) DockerClient {
	t.Logf("Creating secret docker client with root directory '%s', containerName %s", rootDir, containerName)

	c.t.Logf("creating directory '%s'", c.rootDir)
	_, err := c.docker.ExecuteCommandWithErr(c.containerName, "mkdir", "-p", c.rootDir)
	if err != nil {
		return fmt.Errorf("could not create secret directory '%s': %s", c.rootDir, err)
	}

	return &DockerClient{
		t:             t,
		rootDir:       rootDir,
		docker:        d,
		agentBinary:   linuxAgentBinary,
		containerName: containerName,
	}
}

// SetSecret creates or updates a file containing the secret value
func (c *DockerClient) SetSecret(name string, value string) error {
	c.t.Logf("Setting secret %s", name)

	c.t.Log("creating TAR for docker")
	// CopyToContainer only works on a tar reader to copy files Create and add some files to the archive.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: name,
		Mode: 0600,
		Size: int64(len(value)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("could not create temporary tar to send file to container: %s", err)
	}
	if _, err := tw.Write([]byte(value)); err != nil {
		return fmt.Errorf("could not create temporary tar to send file to container: %s", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("could not create temporary tar to send file to container: %s", err)
	}

	option := container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	}

	c.t.Logf("CopyToContainer memory TAR to container '%s' in '%s'", c.containerName, c.rootDir)
	return c.docker.CopyToContainer(context.Background(), c.containerName, c.rootDir, tar.NewReader(&buf), option)
}

// ConfigureRefreshInterval set a refresh interval for secrets. This has to be called before GetAgentConfiguration
func (c *DockerClient) ConfigureRefreshInterval(interval int) {
	c.t.Logf("Setting refresh interval %s", interval)
	c.refreshInterval = interval
}

// AllowExecGroup set secret_backend_command_allow_group_exec_perm to true in the agent configuration. This has to be
// called before GetAgentConfiguration
func (c *DockerClient) AllowExecGroup() {
	c.t.Log("Setting AllowExecGroup to true")
	c.allowExecGroup = true
}

// GetAgentConfiguration returns the Agent configuration dedicated to secrets configured. The result
// is to be injected in the configuration before installing and configuring it.
func (c *DockerClient) GetAgentConfiguration() string {
	c.t.Log("Generating secret configuration")
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
