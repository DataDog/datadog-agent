// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// complianceValues enables the compliance module in the Helm chart.
const complianceValues = `
datadog:
  securityAgent:
    compliance:
      enabled: true
`

// fileMeta is the ownership and permissions of a file as the compliance
// loader reports them.
type fileMeta struct {
	Path  string `json:"path"`
	User  string `json:"user"`
	Group string `json:"group"`
	Mode  uint32 `json:"mode"`
}

// configDirMeta is the kubelet drop-in directory as the compliance loader
// reports it.
type configDirMeta struct {
	fileMeta
	Files []*fileMeta `json:"files"`
}

// nodeConfig holds the fields of the Kubernetes node configuration that the
// tests check.
type nodeConfig struct {
	AzureJSON  *fileMeta `json:"azureJson"`
	Components struct {
		Kubelet *struct {
			ConfigDir *configDirMeta `json:"config-dir"`
		} `json:"kubelet"`
	} `json:"components"`
}

var resourceTypeLine = regexp.MustCompile(`Loaded config with resource type "([^"]+)"`)

// nodeAgentPod returns the node agent pod running on node, or the first one
// when node is empty, once its security-agent container is ready.
func nodeAgentPod(t *testing.T, env *environments.Kubernetes, node string) string {
	t.Helper()
	opts := metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", env.Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	}
	if node != "" {
		opts.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", node).String()
	}
	pods, err := env.KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), opts)
	require.NoError(t, err)
	require.NotEmpty(t, pods.Items, "no node agent pod on node %q", node)
	return waitForSecurityAgentPodReady(t, env.KubernetesCluster, "datadog", pods.Items[0].Name)
}

// loadNodeConfig runs the compliance loader in the security-agent container of
// pod. It returns the resource type the loader reports and the node
// configuration it prints.
func loadNodeConfig(t *testing.T, env *environments.Kubernetes, pod string) (string, string) {
	t.Helper()
	// The chart sets DD_LOG_LEVEL=INFO, and the command logs to stdout,
	// which would mix log lines into the JSON.
	stdout, stderr, err := env.KubernetesCluster.KubernetesClient.PodExec("datadog", pod, "security-agent",
		[]string{"env", "DD_LOG_LEVEL=off", "security-agent", "compliance", "load", "k8s"})
	require.NoError(t, err, stderr)
	t.Logf("node configuration of pod %s:\n%s", pod, stdout)
	m := resourceTypeLine.FindStringSubmatch(stderr)
	require.NotNil(t, m, "no resource type in the loader output: %s", stderr)
	return m[1], stdout
}

// statFile returns the ownership and permission bits of name on the node, as
// stat(1) reports them under HOST_ROOT in the security-agent container of pod.
// The setuid, setgid and sticky bits are left out, as in assertFileMeta.
func statFile(t *testing.T, env *environments.Kubernetes, pod, name string) fileMeta {
	t.Helper()
	stdout, stderr, err := env.KubernetesCluster.KubernetesClient.PodExec("datadog", pod, "security-agent",
		[]string{"sh", "-c", `exec stat -c "%a %U %G" "$HOST_ROOT$1"`, "sh", name})
	require.NoError(t, err, stderr)
	f := strings.Fields(stdout)
	require.Len(t, f, 3, "stat output: %q", stdout)
	mode, err := strconv.ParseUint(f[0], 8, 32)
	require.NoError(t, err)
	return fileMeta{Path: name, User: f[1], Group: f[2], Mode: uint32(mode) & 0o777}
}

// assertFileMeta checks a file reported by the loader against stat(1) on the
// node.
func assertFileMeta(t *testing.T, want fileMeta, got *fileMeta) {
	t.Helper()
	if !assert.NotNil(t, got, "%s missing from the node configuration", want.Path) {
		return
	}
	reported := *got
	// The loader reports Go's file mode, which carries the type bits of a
	// directory.
	reported.Mode &= 0o777
	assert.Equal(t, want, reported)
}
