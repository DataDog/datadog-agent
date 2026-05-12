// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package containers

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type kindIPv6Suite struct {
	kindSuite
}

// TestKindIPv6Suite runs the kind container-integration assertions against an
// IPv6-only kind cluster. Agent pods have no IPv4 stack, so the IPv4-only
// fakeintake is reached through kindnet's NAT64 gateway via the well-known
// 64:ff9b::/96 prefix. See [CONS-8164].
func TestKindIPv6Suite(t *testing.T) {
	helmValues := `
datadog:
    logLevel: DEBUG
clusterAgent:
    envDict:
        DD_CLUSTER_AGENT_LANGUAGE_DETECTION_PATCHER_BASE_BACKOFF: "10s"
`
	e2e.Run(t, &kindIPv6Suite{},
		e2e.WithStackName("kind-ipv6"),
		e2e.WithProvisioner(provkind.Provisioner(
			provkind.WithRunOptions(
				scenkind.WithIPFamily("ipv6"),
				scenkind.WithVMOptions(
					scenec2.WithInstanceType("t3.xlarge"),
				),
				scenkind.WithFakeintakeOptions(
					fakeintake.WithMemory(2048),
					fakeintake.WithRetentionPeriod("31m"),
					fakeintake.WithIPv6NAT64(),
				),
				scenkind.WithDeployDogstatsd(),
				scenkind.WithDeployTestWorkload(),
				scenkind.WithAgentOptions(
					kubernetesagentparams.WithDualShipping(),
					kubernetesagentparams.WithHelmValues(helmValues),
					kubernetesagentparams.WithHelmValues(containerHelmValues),
					kubernetesagentparams.WithKubernetesUseEndpointSlices(),
				),
				scenkind.WithDeployArgoRollout(),
			),
		)),
	)
}

// Test0AgentPodIsIPv6Only asserts each agent DaemonSet pod has only IPv6
// PodIPs, catching the case where the ipFamily plumbing silently falls back
// to dual-stack or IPv4. The leading 0 orders it right after
// k8sSuite.Test00UpAndRunning.
func (suite *kindIPv6Suite) Test0AgentPodIsIPv6Only() {
	ctx := context.Background()

	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + suite.Env().Agent.LinuxNodeAgent.LabelSelectors["app"],
	})
	require.NoError(suite.T(), err, "failed to list datadog agent pods")
	require.NotEmpty(suite.T(), pods.Items, "no datadog agent pods found")

	for _, pod := range pods.Items {
		require.NotEmpty(suite.T(), pod.Status.PodIPs, "pod %s has no PodIPs", pod.Name)
		for _, entry := range pod.Status.PodIPs {
			ip := net.ParseIP(entry.IP)
			require.NotNilf(suite.T(), ip, "pod %s PodIP %q is not a valid IP", pod.Name, entry.IP)
			assert.Nilf(suite.T(), ip.To4(), "pod %s PodIP %q is IPv4 (expected IPv6-only); full PodIPs=%+v", pod.Name, entry.IP, pod.Status.PodIPs)
		}
	}
}
