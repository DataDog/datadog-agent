// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secrets offers tools to configure the secret feature of the Agent
package secrets

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/docker/docker/api/types/container"
)

var (
	linuxAgentBinary   = "/opt/datadog-agent/bin/agent/agent"
	windowsAgentBinary = "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
)

type Client interface {
	SetSecret(name string, value string) error
	ConfigureRefreshInterval(interval int)
	AllowExecGroup()
	GetAgentConfiguration() string
}

type hostClient struct {
	t       *testing.T
	rootDir string
	host    *components.RemoteHost

	refreshInterval int
	agentBinary     string
	allowExecGroup  bool
}

// NewHostClient creates a new hostClient that can create and delete files containing secrets
func NewHostClient(t *testing.T, host *components.RemoteHost, rootDir string) Client {
	t.Logf("Creating secret host client with root directory '%s'", rootDir)

	// WIP We're trying to detect the current OS for the test
	agentBinary := linuxAgentBinary
	if host.OSFamily == os.WindowsFamily {
		agentBinary = windowsAgentBinary
	}

	return &hostClient{
		t:           t,
		rootDir:     rootDir,
		host:        host,
		agentBinary: agentBinary,
	}
}

// SetSecret creates or updates a file containing the secret value
func (c *hostClient) SetSecret(name string, value string) error {
	c.t.Logf("Setting secret %s", name)

	c.t.Logf("creating directory '%s'", c.rootDir)
	err := c.host.MkdirAll(c.rootDir)
	if err != nil {
		return fmt.Errorf("could not create secret directory '%s': %s", c.rootDir, err)
	}

	filePath := filepath.Join(c.rootDir, name)
	_, err = c.host.WriteFile(filePath, []byte(value))
	return err
}

// ConfigureRefreshInterval set a refresh interval for secrets. This has to be called before GetAgentConfiguration
func (c *hostClient) ConfigureRefreshInterval(interval int) {
	c.t.Logf("Setting refresh interval %s", interval)
	c.refreshInterval = interval
}

// AllowExecGroup set secret_backend_command_allow_group_exec_perm to true in the agent configuration. This has to be
// called before GetAgentConfiguration
func (c *hostClient) AllowExecGroup() {
	c.t.Log("Setting AllowExecGroup to true")
	c.allowExecGroup = true
}

// GetAgentConfiguration returns the Agent configuration dedicated to secrets configured. The result
// is to be injected in the configuration before installing and configuring it.
func (c *hostClient) GetAgentConfiguration() string {
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

type k8sClient struct {
	t       *testing.T
	rootDir string
	k8s     *client.KubernetesClient

	refreshInterval int
	agentBinary     string
	allowExecGroup  bool
	pod             string
	namespace       string
}

// NewK8sClient creates a new k8sClient that can create and delete files containing secrets
func NewK8sClient(t *testing.T, k8s *client.KubernetesClient, rootDir string, namespace string, pod string) Client {
	t.Logf("Creating secret k8s client with root directory '%s', namespace %s and pod %s", rootDir, namespace, pod)
	return &k8sClient{
		t:           t,
		rootDir:     rootDir,
		k8s:         k8s,
		agentBinary: linuxAgentBinary,
		namespace:   namespace,
		pod:         pod,
	}
}

// SetSecret creates or updates a file containing the secret value
func (c *k8sClient) SetSecret(name string, value string) error {
	c.t.Logf("Setting secret %s", name)

	c.t.Logf("creating directory '%s'", c.rootDir)
	_, _, err := c.k8s.PodExec("namespace", "pod", "container", []string{"mkdir", "-p", c.rootDir})
	if err != nil {
		return fmt.Errorf("could not create secret directory '%s': %s", c.rootDir, err)
	}

	filePath := filepath.Join(c.rootDir, name)
	c.t.Logf("writing file '%s'", filePath)
	_, _, err = c.k8s.PodExec(
		"namespace",
		"pod",
		"container",
		[]string{"/bin/bash", "-c", fmt.Sprintf("echo '%s' | tee %s", value, filePath)},
	)
	return err
}

// ConfigureRefreshInterval set a refresh interval for secrets. This has to be called before GetAgentConfiguration
func (c *k8sClient) ConfigureRefreshInterval(interval int) {
	c.t.Logf("Setting refresh interval %s", interval)
	c.refreshInterval = interval
}

// AllowExecGroup set secret_backend_command_allow_group_exec_perm to true in the agent configuration. This has to be
// called before GetAgentConfiguration
func (c *k8sClient) AllowExecGroup() {
	c.t.Log("Setting AllowExecGroup to true")
	c.allowExecGroup = true
}

// GetAgentConfiguration returns the Agent configuration dedicated to secrets configured. The result
// is to be injected in the configuration before installing and configuring it.
func (c *k8sClient) GetAgentConfiguration() string {
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

type dockerClient struct {
	t       *testing.T
	rootDir string
	docker  *client.Docker

	refreshInterval int
	agentBinary     string
	allowExecGroup  bool
	containerName   string
}

// NewDockerClient creates a new dockerClient that can create and delete files containing secrets
func NewDockerClient(t *testing.T, d *client.Docker, rootDir string, containerName string) Client {
	t.Logf("Creating secret docker client with root directory '%s', containerName %s", rootDir, containerName)
	return &dockerClient{
		t:             t,
		rootDir:       rootDir,
		docker:        d,
		agentBinary:   linuxAgentBinary,
		containerName: containerName,
	}
}

// SetSecret creates or updates a file containing the secret value
func (c *dockerClient) SetSecret(name string, value string) error {
	c.t.Logf("Setting secret %s", name)

	c.t.Logf("creating directory '%s'", c.rootDir)
	_, err := c.docker.ExecuteCommandWithErr(c.containerName, "mkdir", "-p", c.rootDir)
	if err != nil {
		return fmt.Errorf("could not create secret directory '%s': %s", c.rootDir, err)
	}

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
func (c *dockerClient) ConfigureRefreshInterval(interval int) {
	c.t.Logf("Setting refresh interval %s", interval)
	c.refreshInterval = interval
}

// AllowExecGroup set secret_backend_command_allow_group_exec_perm to true in the agent configuration. This has to be
// called before GetAgentConfiguration
func (c *dockerClient) AllowExecGroup() {
	c.t.Log("Setting AllowExecGroup to true")
	c.allowExecGroup = true
}

// GetAgentConfiguration returns the Agent configuration dedicated to secrets configured. The result
// is to be injected in the configuration before installing and configuring it.
func (c *dockerClient) GetAgentConfiguration() string {
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
