// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

type kindNoPulumiSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestKindNoPulumi walks the same provision -> install -> assert lifecycle
// every E2E test follows, but with the first two stages performed
// out-of-band by the e2ectl CLI instead of a Pulumi provisioner, driven by
// the colocated kind_nopulumi_test.yaml (see cmd/e2ectl's package doc):
//
//  1. provision        external: `e2ectl run --config=kind_nopulumi_test.yaml --stage=provision`
//  2. install           external: `e2ectl run --config=kind_nopulumi_test.yaml --stage=install`
//  3. assert            TestClusterAgentStatus
//  4. install (update)  TestClusterAgentVersionUpdate, via v.Env().UpdateAgent --
//     in-process, changes the cluster agent's version live and updates v.Env().Agent
//  5. assert            TestClusterAgentVersionUpdate, immediately after stage 4
//
// E2E_ENV_FILE must point at the state file e2ectl wrote/updated.
func TestKindNoPulumi(t *testing.T) {
	envFile := os.Getenv("E2E_ENV_FILE")
	if envFile == "" {
		t.Fatal("E2E_ENV_FILE must point at the state file written by `e2ectl run`")
	}

	e2e.Run(t, &kindNoPulumiSuite{}, e2e.WithProvisioner(
		provisioners.NewSingleFileProvisioner[environments.Kubernetes]("kind-nopulumi", envFile)))
}

func (v *kindNoPulumiSuite) TestClusterAgentStatus() {
	ctx := context.Background()
	namespace := v.Env().Agent.LinuxClusterAgent.Namespace
	appLabel := v.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]

	pods, err := v.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + appLabel,
		Limit:         1,
	})
	v.Require().NoError(err)
	v.Require().NotEmpty(pods.Items, "cluster-agent pod not found in namespace %s", namespace)

	stdout, stderr, err := v.Env().KubernetesCluster.KubernetesClient.PodExec(
		namespace, pods.Items[0].Name, "cluster-agent",
		[]string{"datadog-cluster-agent", "status"})
	v.Require().NoError(err, "cluster-agent status failed: %s", stderr)
	v.Empty(stderr, "stderr of `datadog-cluster-agent status` should be empty")
	v.Contains(stdout, "Running Checks")
}

// nextVersion picks a target version distinct from current, for exercising
// a real config change. "latest" is used unless the environment is already
// on it, in which case it falls back to a pinned tag.
func nextVersion(current string) string {
	if current != "latest" {
		return "latest"
	}
	return "7.65.0"
}

func (v *kindNoPulumiSuite) TestClusterAgentVersionUpdate() {
	current := v.Env().Agent
	target := nextVersion(current.LinuxClusterAgent.Version)

	// -- install stage: the config change under test --
	v.Require().NoError(v.Env().UpdateAgent(context.Background(), installers.HelmK8sInstallParams{
		AgentVersion:        current.LinuxNodeAgent.Version, // held constant
		ClusterAgentVersion: target,
		Namespace:           current.LinuxClusterAgent.Namespace,
	}))

	// -- assert stage: the update landed, both in the env and live in the cluster --
	v.Equal(target, v.Env().Agent.LinuxClusterAgent.Version)

	ctx := context.Background()
	namespace := v.Env().Agent.LinuxClusterAgent.Namespace
	appLabel := v.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]

	// Poll rather than a one-shot List: a single check can catch the old pod
	// mid-termination during the rolling update and flake.
	var podName string
	v.EventuallyWithT(func(c *assert.CollectT) {
		pods, err := v.Env().KubernetesCluster.Client().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + appLabel,
		})
		if !assert.NoError(c, err) {
			return
		}
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp != nil || len(pod.Spec.Containers) == 0 {
				continue
			}
			if strings.Contains(pod.Spec.Containers[0].Image, target) {
				podName = pod.Name
				return
			}
		}
		assert.Fail(c, "no live cluster-agent pod running image tag %q yet", target)
	}, 5*time.Minute, 10*time.Second)
	v.Require().NotEmpty(podName, "no live cluster-agent pod running image tag %q found", target)

	stdout, stderr, err := v.Env().KubernetesCluster.KubernetesClient.PodExec(
		namespace, podName, "cluster-agent",
		[]string{"datadog-cluster-agent", "status"})
	v.Require().NoError(err, "cluster-agent status failed after version update: %s", stderr)
	v.Empty(stderr, "stderr of `datadog-cluster-agent status` should be empty")
	v.Contains(stdout, "Running Checks")
}
