// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// This test suite contains cluster agent specific tests that require isolation.
// Tests in this file may include disruptive operations (like deleting cluster agent pods)
// or cluster agent specific functionality (like autoscaling, external metrics, etc.) that
// could interfere with other tests that depend on cluster agent functionality.
//
// This suite uses its own dedicated Kubernetes cluster with a newer Kubernetes version
// to ensure complete isolation from other test suites.

package containers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type k8sClusterAgentSuite struct {
	baseSuite[environments.Kubernetes]
}

// TestK8SClusterAgentSuite creates a separate Kubernetes cluster specifically for cluster agent tests
// This uses a newer Kubernetes version and ensures complete isolation from other test suites
func TestK8SClusterAgentSuite(t *testing.T) {
	e2e.Run(t, &k8sClusterAgentSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithAgentOptions(kubernetesagentparams.WithDualShipping()),
		awskubernetes.WithExtraConfigParams(runner.ConfigMap{
			"ddinfra:kubernetesVersion": auto.ConfigValue{Value: "1.31"}, // Use newer Kubernetes version
		}),
	)))
}

func (suite *k8sClusterAgentSuite) SetupSuite() {
	suite.baseSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}

// getLeaderPodName retrieves the current cluster agent leader pod name
func (suite *k8sClusterAgentSuite) getLeaderPodName(ctx context.Context) string {
	var leaderPodName string

	// Find cluster agent pod, it could be either leader or follower
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", suite.Env().Agent.LinuxClusterAgent.LabelSelectors["app"]).String(),
		Limit:         1,
	})
	suite.Require().NoError(err)
	suite.Require().Len(pods.Items, 1, "Expected exactly one running cluster agent pod")

	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", pods.Items[0].Name, "cluster-agent", []string{"env", "DD_LOG_LEVEL=off", "datadog-cluster-agent", "status", "--json"})
	suite.Require().NoError(err)
	suite.Require().Empty(stderr, "Standard error of `datadog-cluster-agent status` should be empty")

	var blob interface{}
	err = json.Unmarshal([]byte(stdout), &blob)
	suite.Require().NoError(err, "Failed to unmarshal JSON output of `datadog-cluster-agent status --json`")

	blobMap, ok := blob.(map[string]interface{})
	suite.Require().Truef(ok, "Failed to assert blob as map[string]interface{}")

	suite.Require().Contains(blobMap, "leaderelection", "Field `leaderelection` not found in the JSON output")
	suite.Require().Contains(blobMap["leaderelection"], "leaderName", "Field `leaderelection.leaderName` not found in the JSON output")
	leaderPodName, ok = (blobMap["leaderelection"].(map[string]interface{}))["leaderName"].(string)
	suite.Require().Truef(ok, "Failed to assert `leaderelection.leaderName` as string")
	suite.Require().NotEmpty(leaderPodName, "Field `leaderelection.leaderName` is empty in the JSON output")

	// Verify the leader pod exists
	leaderPod, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").Get(ctx, leaderPodName, metav1.GetOptions{})
	suite.Require().NoError(err)
	suite.Require().NotNilf(leaderPod, "Leader pod: %s not found", leaderPodName)

	return leaderPodName
}

// TestClusterAgentLeaderElectionWithReelection tests cluster agent leader election by forcing re-election
// This test identifies the current leader, deletes it, and verifies that a different pod becomes the new leader
func (suite *k8sClusterAgentSuite) TestClusterAgentLeaderElectionWithReelection() {
	ctx := context.Background()

	// Step 1: Get the current leader
	suite.T().Logf("Finding current cluster agent leader...")
	originalLeaderPodName := suite.getLeaderPodName(ctx)
	suite.T().Logf("Current cluster agent leader: %s", originalLeaderPodName)

	// Step 2: Force re-election by deleting the current leader pod
	suite.T().Logf("Forcing re-election by deleting leader pod: %s", originalLeaderPodName)
	err := suite.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").Delete(ctx, originalLeaderPodName, metav1.DeleteOptions{})
	suite.Require().NoError(err, "Failed to delete leader pod")

	// Step 3: Wait for leader re-election to complete and verify a different leader is elected
	suite.T().Logf("Waiting for new leader to be elected...")
	suite.EventuallyWithTf(func(c *assert.CollectT) {
		newLeaderPodName := suite.getLeaderPodName(ctx)
		assert.NotEqual(c, originalLeaderPodName, newLeaderPodName, "New leader should be a different pod")
		suite.T().Logf("Different pod elected as leader: %s -> %s", originalLeaderPodName, newLeaderPodName)
	}, 5*time.Minute, 10*time.Second, "Different leader should be elected after re-election")

	suite.T().Logf("Cluster agent leader re-election test completed successfully")
}
