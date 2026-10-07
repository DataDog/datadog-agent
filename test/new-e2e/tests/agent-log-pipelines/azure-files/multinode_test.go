// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package azurefiles

import (
	"sort"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// TestMultiNodeScenario runs the Agent DaemonSet on AZURE_FILES_E2E_NODES
// nodes, so every Agent pod has the smb cell's source, and requires what the
// SMB source does today: each Agent reads the whole share, since nothing
// elects a single reader, so every record arrives once from each of them.
// Once the source elects one reader, the rule becomes exactly once in all.
func (suite *azureFilesSuite) TestMultiNodeScenario() {
	c := suite.scenarioCell(multiNodeScenario)
	// Registered first, so it is the last line the test prints.
	defer suite.remindOfMultiNodeStack()
	suite.installAgent()
	defer suite.captureEvidence()
	suite.requireAgentReady()
	agents := suite.agentHostnames()
	hosts := make([]string, 0, len(agents))
	for host := range agents {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	suite.writeScenarioEvidence("agents", agents)
	suite.T().Logf("%s: %d Agents run the source, one per node (hostname: pod): %v", c.name, len(agents), agents)
	suite.checkCell(c, recordRules{hosts: hosts})
}

// remindOfMultiNodeStack says, last, that the run used a stack of its own. A
// failed run keeps it (WithSkipDeleteOnFailure), and so does E2E_DEV_MODE:
// an AKS cluster of N nodes, its Fakeintake VM and its storage accounts, which
// bill until that stack is destroyed, next to the one-node stack the other
// runs reuse.
func (suite *azureFilesSuite) remindOfMultiNodeStack() {
	suite.T().Logf("REMINDER: this multi-node run used its own stack %s (%d AKS nodes, a Fakeintake VM and storage accounts), separate from any one-node stack. "+
		"If it was kept (failed run or E2E_DEV_MODE), destroy it once done: run `dda inv new-e2e-tests.clean -s` and select %s",
		suite.spec.stackName, suite.spec.nodes, suite.spec.stackName)
}

// agentHostnames requires one ready Agent pod on each node, and returns the
// pods by the hostname each Agent reports, which its logs carry.
func (suite *azureFilesSuite) agentHostnames() map[string]string {
	t := suite.T()
	var pods []corev1.Pod
	suite.EventuallyWithT(func(collect *assert.CollectT) {
		var err error
		pods, err = suite.agentPods()
		require.NoError(collect, err)
		assert.Len(collect, pods, suite.spec.nodes, "the Agent DaemonSet must run one pod on each of the %d nodes", suite.spec.nodes)
	}, 5*time.Minute, 10*time.Second)

	nodes := make(map[string]string, len(pods))
	agents := make(map[string]string, len(pods))
	for _, pod := range pods {
		require.NotEmpty(t, pod.Spec.NodeName, "the Agent pod %s is not scheduled", pod.Name)
		other, twice := nodes[pod.Spec.NodeName]
		require.False(t, twice, "the Agent pods %s and %s both run on node %s", other, pod.Name, pod.Spec.NodeName)
		nodes[pod.Spec.NodeName] = pod.Name

		stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(agentNamespace, pod.Name, agentContainer, []string{"agent", "hostname"})
		require.NoError(t, err, "agent hostname on %s (stderr: %s)", pod.Name, strings.TrimSpace(stderr))
		hostname := lastLine(stdout)
		require.NotEmpty(t, hostname, "agent hostname printed nothing on %s", pod.Name)
		other, twice = agents[hostname]
		require.False(t, twice, "the Agents of %s and %s both report hostname %s, so their logs cannot be told apart", other, pod.Name, hostname)
		agents[hostname] = pod.Name
	}
	return agents
}

// lastLine returns the last line of output that is not blank: a command can
// print warnings before its result.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
