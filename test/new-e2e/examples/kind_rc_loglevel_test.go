// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

// This example shows how to use fakeintake's Remote Config backend to change
// the agent's log level at runtime on a Kind (Kubernetes-in-Docker) cluster.
//
// Remote Config is wired automatically: every awskubernetes.Provisioner() run
// starts fakeintake with a fixed TUF signing key and configures the agent to
// point at fakeintake's RC endpoint — no extra provisioner options needed.
//
// Flow:
//  1. Agent pods start at the default (info) log level — no DEBUG lines in the log.
//  2. Two AGENT_CONFIG payloads are pushed via the fakeintake RC API:
//     - a named layer that sets log_level to "debug"
//     - a configuration_order that activates the layer
//  3. The agent polls fakeintake (every 5 s), receives the signed config, and
//     starts writing DEBUG lines to its log file.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type rcLogLevelKindExampleSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestRCLogLevelKindExampleSuite is the entry point for the Kind Remote Config log-level example.
// Run locally with:
//
//	dda inv new-e2e-tests.run --targets=./examples/... -run TestRCLogLevelKindExampleSuite
func TestRCLogLevelKindExampleSuite(t *testing.T) {
	// RC is wired automatically — no special provisioner option is required.
	e2e.Run(t, &rcLogLevelKindExampleSuite{},
		e2e.WithProvisioner(awskubernetes.Provisioner()),
	)
}

// TestLogLevelViaRCKind verifies that the agent honours an AGENT_CONFIG payload
// delivered through Remote Config on a Kind cluster:
//
//  1. No DEBUG lines exist at startup (default log_level is info).
//  2. After pushing an AGENT_CONFIG layer + configuration_order, DEBUG lines appear.
func (s *rcLogLevelKindExampleSuite) TestLogLevelViaRCKind() {
	fi := s.Env().FakeIntake.Client()

	// Step 1 — wait for an agent pod to be running and confirm no DEBUG logs.
	var agentPodName, agentPodNamespace string
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").
			List(context.Background(), v1.ListOptions{LabelSelector: "app=dda-linux-datadog"})
		if !assert.NoError(c, err) {
			return
		}
		for _, pod := range res.Items {
			if pod.DeletionTimestamp != nil {
				continue
			}
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == "agent" && cs.Ready {
					agentPodName = pod.Name
					agentPodNamespace = pod.Namespace
					return
				}
			}
		}
		assert.Fail(c, "no agent pod with a ready agent container found")
	}, 3*time.Minute, 10*time.Second, "agent pod did not become ready")

	agentLog, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		agentPodNamespace, agentPodName, "agent",
		[]string{"cat", "/var/log/datadog/agent.log"})
	require.NoError(s.T(), err)
	require.False(s.T(), strings.Contains(agentLog, "| DEBUG |"),
		"expected no DEBUG logs at default log level")

	// Step 2 — push an AGENT_CONFIG layer that sets log_level to "debug".
	err = fi.RCAddConfig("", "AGENT_CONFIG", "layer1", "log_level_debug",
		[]byte(`{"name":"layer1","config":{"log_level":"debug"}}`))
	require.NoError(s.T(), err)

	// Step 3 — push a configuration_order to activate the layer.
	// MergeRCAgentConfig requires a non-empty Order or InternalOrder to apply.
	err = fi.RCAddConfig("", "AGENT_CONFIG", "configuration_order", "order",
		[]byte(`{"order":["layer1"],"internal_order":[]}`))
	require.NoError(s.T(), err)

	// Step 4 — wait for DEBUG lines to appear.
	// The agent polls RC every 5 s (set by remote_configuration.refresh_interval).
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, _, execErr := s.Env().KubernetesCluster.KubernetesClient.PodExec(
			agentPodNamespace, agentPodName, "agent",
			[]string{"cat", "/var/log/datadog/agent.log"})
		assert.NoError(c, execErr)
		assert.True(c, strings.Contains(logs, "| DEBUG |"),
			"expected DEBUG logs after RC log level change")
	}, 3*time.Minute, 10*time.Second)
}
