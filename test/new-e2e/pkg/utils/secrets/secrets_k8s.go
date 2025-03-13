// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsutils

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

const linuxAgentBinary = "/opt/datadog-agent/bin/agent/agent"

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

	c.t.Logf("creating directory '%s'", rootDir)
	_, _, err := c.k8s.PodExec("namespace", "pod", "container", []string{"mkdir", "-p", rootDir})
	if err != nil {
		return fmt.Errorf("could not create secret directory '%s': %s", rootDir, err)
	}

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
