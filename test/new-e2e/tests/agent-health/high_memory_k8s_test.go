// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenthealth

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

const (
	// highMemoryK8sIssuePrefix is the IssueIDPrefix of comp/healthplatform/issues/highmemoryk8s; the full id is
	// suffixed with a hash of the reporting Agent's discriminator and the source container.
	highMemoryK8sIssuePrefix = "high-memory-k8s:"

	nodeAgentNamespace = "datadog"
	nodeAgentContainer = "agent"

	// highMemoryK8sDetectionTimeout covers the 5 minute evaluation window filling with high samples, the 1 minute
	// evaluation interval and the 30s forwarder interval.
	highMemoryK8sDetectionTimeout = 10 * time.Minute
	// highMemoryK8sResolutionTimeout covers one low sample pulling the 5 minute average under the threshold.
	highMemoryK8sResolutionTimeout = 5 * time.Minute
)

type highMemoryK8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestHighMemoryK8sSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &highMemoryK8sSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(highMemoryK8sHelmValues),
				),
			),
		)),
	)
}

// TestHighMemoryK8sIssueLifecycle verifies that holding the node Agent container above 90% of its memory limit is
// reported to fakeintake as ACTIVE, and that releasing the memory reports it as RESOLVED. Both phases run in one
// test body: resolution needs the issue the detection phase raised.
//
// Cross-restart persistence is tested separately in TestResilienceSuite.
func (suite *highMemoryK8sSuite) TestHighMemoryK8sIssueLifecycle() {
	t := suite.T()
	fakeIntake := suite.Env().FakeIntake.Client()
	pod := suite.nodeAgentPod()

	// Leave the Agent container as provisioned so the test can be retried on the same infra.
	t.Cleanup(func() {
		_, _, _ = suite.execInNodeAgent(pod, stopMemHogCommand)
	})

	// Detection: allocate memory up to 93% of the Agent container limit.
	_, stderr, err := suite.execInNodeAgent(pod, startMemHogCommand)
	require.NoError(t, err, "failed to start the memory allocation: %s", stderr)

	reached := assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		_, _, err := suite.execInNodeAgent(pod, "test -f "+memHogPIDFile)
		assert.NoError(ct, err, "memory allocation has not reached its target yet")
	}, 2*time.Minute, 5*time.Second, "memory allocation did not reach its target")
	if !reached {
		t.Fatalf("diagnostics: %s", suite.memHogDiagnostics(pod))
	}

	var issues []*healthplatform.Issue
	detected := assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		issues = nil
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, highMemoryK8sIssuePrefix) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_ACTIVE {
					issues = append(issues, iss)
				}
			}
		}
		assert.NotEmpty(ct, issues, "issue not found as ACTIVE in fakeintake")
	}, highMemoryK8sDetectionTimeout, defaultIssuePollInterval, "issue not detected as ACTIVE in fakeintake")
	if !detected {
		t.Fatalf("diagnostics: %s", suite.memHogDiagnostics(pod))
	}

	require.NotEmpty(t, issues)
	issue := issues[0]
	assert.Equal(t, "High Memory K8s", issue.IssueName)
	assert.Equal(t, "high_memory_k8s", issue.IssueType)
	assert.Equal(t, "resources", issue.Category)
	assert.Equal(t, "agent", issue.Source)
	assert.Equal(t, "agent", issue.Location)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Contains(t, issue.Tags, "memory")
	assert.Contains(t, issue.Tags, "kubernetes")
	require.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Summary)
	assert.NotEmpty(t, issue.Remediation.Steps)

	// Resolution: release the memory.
	_, stderr, err = suite.execInNodeAgent(pod, stopMemHogCommand)
	require.NoError(t, err, "failed to release the memory allocation: %s", stderr)
	require.NoError(t, fakeIntake.FlushServerAndResetAggregators())

	resolved := assert.EventuallyWithT(t, func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		assert.NoError(ct, err)
		var found bool
		for _, p := range payloads {
			for _, iss := range findIssuesByPrefix(p, highMemoryK8sIssuePrefix) {
				if iss.PersistedIssue != nil && iss.PersistedIssue.State == healthplatform.IssueState_ISSUE_STATE_RESOLVED {
					found = true
				}
			}
		}
		assert.True(ct, found, "issue not found as RESOLVED in fakeintake")
	}, highMemoryK8sResolutionTimeout, defaultIssuePollInterval, "issue not resolved after releasing the memory")
	if !resolved {
		t.Fatalf("diagnostics: %s", suite.memHogDiagnostics(pod))
	}
}

// nodeAgentPod returns the name of the node Agent pod. The kind cluster has a single node, so there is one.
func (suite *highMemoryK8sSuite) nodeAgentPod() string {
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(nodeAgentNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	require.NoError(suite.T(), err)
	require.Len(suite.T(), pods.Items, 1, "expected exactly one node Agent pod")
	return pods.Items[0].Name
}

// execInNodeAgent runs cmd with sh -c in the node Agent container.
func (suite *highMemoryK8sSuite) execInNodeAgent(pod, cmd string) (stdout, stderr string, err error) {
	return suite.Env().KubernetesCluster.KubernetesClient.PodExec(nodeAgentNamespace, pod, nodeAgentContainer, []string{"sh", "-c", cmd})
}

// memHogDiagnostics returns what is needed to tell apart the ways the issue can fail to be reported: the memory
// allocation output and the container memory accounting (allocation failed), the agent_performance series the
// monitor reads (series not recorded), the issues known to the Agent (issue not forwarded), the pod labels the
// series recording is gated on, and the Agent log lines of the monitor and its evaluator.
func (suite *highMemoryK8sSuite) memHogDiagnostics(pod string) string {
	var b strings.Builder
	section := func(title, content string) {
		b.WriteString("\n--- " + title + " ---\n" + content + "\n")
	}
	exec := func(cmd string) string {
		stdout, stderr, err := suite.execInNodeAgent(pod, cmd)
		if err != nil {
			return "error: " + err.Error() + " " + stderr
		}
		return stdout
	}

	section("memory accounting", exec("cat "+memHogLogFile+"; echo; "+
		"echo memory.max: $(cat /sys/fs/cgroup/memory.max); "+
		"echo memory.current: $(cat /sys/fs/cgroup/memory.current); "+
		"grep -E '^(anon|inactive_file) ' /sys/fs/cgroup/memory.stat"))
	section("agent_performance memory series", exec("agent diagnose show-metadata agent-full-telemetry | "+
		"grep -E '^agent_performance__(container_)?memory_(usage|limit)' || echo 'no agent_performance memory series'"))
	section("health issues", exec("agent diagnose show-metadata health-issues"))

	if p, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(nodeAgentNamespace).Get(context.Background(), pod, metav1.GetOptions{}); err == nil {
		section("pod labels", fmt.Sprint(p.Labels))
	} else {
		section("pod labels", "error: "+err.Error())
	}

	logs, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(nodeAgentNamespace).
		GetLogs(pod, &corev1.PodLogOptions{Container: nodeAgentContainer}).DoRaw(context.Background())
	if err != nil {
		section("agent logs", "error: "+err.Error())
	} else {
		var matched []string
		for _, line := range strings.Split(string(logs), "\n") {
			if agentLogPattern.MatchString(line) {
				matched = append(matched, line)
			}
		}
		section("agent logs", strings.Join(matched, "\n"))
	}
	return b.String()
}

// agentLogPattern matches the Agent log lines of the high memory monitor, the metrics evaluator and the container
// check that records the agent_performance series.
var agentLogPattern = regexp.MustCompile(`(?i)metrics evaluator|high memory|high_memory|health platform|check:container`)
