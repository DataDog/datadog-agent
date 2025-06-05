// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
)

const defaultTimeout = 10 * time.Minute

//go:embed agent_values.yaml
var agentCustomValuesFmt string

type k8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestKindSuite(t *testing.T) {
	t.Parallel()
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awskubernetes.KindProvisioner(
			awskubernetes.WithDeployTestWorkload(),
			awskubernetes.WithAgentOptions(
				kubernetesagentparams.WithDualShipping(),
				kubernetesagentparams.WithHelmValues(agentCustomValuesFmt),
			),
		)),
	}
	e2e.Run(t, &k8sSuite{}, options...)
}

func (suite *k8sSuite) TestRedisPod() {
	expectResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorPod},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return strings.HasPrefix(payload.Pod.Metadata.Name, "redis-query") &&
				payload.Pod.Metadata.Namespace == "workload-redis"
		},
		message: "find a redis-query pod",
		timeout: defaultTimeout,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client(), 1, nil)
}

func (suite *k8sSuite) TestNode() {
	expectResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorNode},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return payload.Node.Metadata.Name == fmt.Sprintf("%s-control-plane", suite.Env().KubernetesCluster.ClusterName)
		},
		message: "find a control plane node",
		timeout: defaultTimeout,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client(), 1, nil)
}

func (suite *k8sSuite) TestDeploymentManif() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifest &&
				manif.Metadata.Name == "redis" &&
				manif.Metadata.Namespace == "workload-redis"
		},
		message: "find a Deployment manifest",
		timeout: defaultTimeout,
	}.Assert(suite)
}

func (suite *k8sSuite) TestCRDManif() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCRD &&
				manif.Spec.Group == "datadoghq.com" &&
				manif.Spec.Names.Kind == "DatadogMetric"
		},
		message: "find a DatadogMetric manifest CRD",
		timeout: defaultTimeout,
	}.Assert(suite)
}

func (suite *k8sSuite) TestCRManif() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCR &&
				manif.APIVersion == "datadoghq.com/v1alpha1" &&
				manif.Kind == "DatadogMetric" &&
				manif.Metadata.Name == "redis"
		},
		message: "find a DatadogMetric manifest CR instance",
		timeout: defaultTimeout,
	}.Assert(suite)
}

func (suite *k8sSuite) TestTerminatedResource() {
	deploymentName := "terminated-deployment"
	replicas := int32(1)
	namespace := "datadog"
	client := suite.Env().KubernetesCluster.KubernetesClient.K8sClient

	// create a deployment
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deploymentName},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "terminated-resource"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "terminated-resource"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx", Image: "nginx"},
					},
				},
			},
		},
	}
	_, err := client.AppsV1().Deployments(namespace).Create(context.Background(), deploy, metav1.CreateOptions{})
	require.NoError(suite.T(), err)

	// ensure the running deployment and pod are collected by the agent
	expectResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorPod},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return strings.HasPrefix(payload.Pod.Metadata.Name, deploymentName) &&
				payload.Pod.Metadata.Namespace == namespace
		},
		message: "find a pod: " + deploymentName,
		timeout: defaultTimeout,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client(), 1, nil)
	expectResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorDeployment},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return strings.HasPrefix(payload.Deployment.Metadata.Name, deploymentName) &&
				payload.Pod.Metadata.Namespace == namespace
		},
		message: "find a deployment: " + deploymentName,
		timeout: defaultTimeout,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client(), 1, nil)

	// delete the deployment
	err = client.AppsV1().Deployments(namespace).Delete(context.Background(), deploymentName, metav1.DeleteOptions{})
	require.NoError(suite.T(), err)

	// ensure the terminated deployment and pod are collected by the agent and only one of each is found
	maximum := 1
	expectResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorPod},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return strings.HasPrefix(payload.Pod.Metadata.Name, deploymentName) &&
				payload.Pod.Metadata.DeletionTimestamp != 0
		},
		message: "find a pod: " + deploymentName,
		timeout: 3 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client(), 1, &maximum)
	expectResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorDeployment},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return strings.HasPrefix(payload.Deployment.Metadata.Name, deploymentName) &&
				payload.Pod.Metadata.DeletionTimestamp != 0
		},
		message: "find a deployment: " + deploymentName,
		timeout: 3 * time.Minute,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client(), 1, &maximum)
}
