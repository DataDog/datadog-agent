// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestDeploymentHandlers_ExtractResource(t *testing.T) {
	handlers := &DeploymentHandlers{}

	// Create test deployment
	deployment := createTestDeployment("test-deployment", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Extract resource
	resourceModel := handlers.ExtractResource(ctx, deployment)

	// Validate extraction
	deploymentModel, ok := resourceModel.(*model.Deployment)
	assert.True(t, ok)
	assert.NotNil(t, deploymentModel)
	assert.Equal(t, "test-deployment", deploymentModel.Metadata.Name)
	assert.Equal(t, "test-namespace", deploymentModel.Metadata.Namespace)
	assert.Equal(t, int32(3), deploymentModel.ReplicasDesired)
	assert.Equal(t, "RollingUpdate", deploymentModel.DeploymentStrategy)
}

func TestDeploymentHandlers_ResourceList(t *testing.T) {
	handlers := &DeploymentHandlers{}

	// Create test deployments
	deployment1 := createTestDeployment("deployment-1", "namespace-1")
	deployment2 := createTestDeployment("deployment-2", "namespace-2")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Convert list
	resourceList := []*appsv1.Deployment{deployment1, deployment2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*appsv1.Deployment)
	assert.True(t, ok)
	assert.Equal(t, "deployment-1", resource1.Name)
	assert.NotSame(t, deployment1, resource1) // Should be a copy

	resource2, ok := resources[1].(*appsv1.Deployment)
	assert.True(t, ok)
	assert.Equal(t, "deployment-2", resource2.Name)
	assert.NotSame(t, deployment2, resource2) // Should be a copy
}

func TestDeploymentHandlers_ResourceUID(t *testing.T) {
	handlers := &DeploymentHandlers{}

	deployment := createTestDeployment("test-deployment", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	deployment.UID = expectedUID

	uid := handlers.ResourceUID(nil, deployment)
	assert.Equal(t, expectedUID, uid)
}

func TestDeploymentHandlers_ResourceVersion(t *testing.T) {
	handlers := &DeploymentHandlers{}

	deployment := createTestDeployment("test-deployment", "test-namespace")
	expectedVersion := "v123"
	deployment.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.Deployment{}

	version := handlers.ResourceVersion(nil, deployment, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestDeploymentHandlers_BuildMessageBody(t *testing.T) {
	handlers := &DeploymentHandlers{}

	deployment1 := createTestDeployment("deployment-1", "namespace-1")
	deployment2 := createTestDeployment("deployment-2", "namespace-2")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	deployment1Model := k8sTransformers.ExtractDeployment(ctx, deployment1)
	deployment2Model := k8sTransformers.ExtractDeployment(ctx, deployment2)

	// Build message body
	resourceModels := []interface{}{deployment1Model, deployment2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorDeployment)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Deployments, 2)
	assert.Equal(t, "deployment-1", collectorMsg.Deployments[0].Metadata.Name)
	assert.Equal(t, "deployment-2", collectorMsg.Deployments[1].Metadata.Name)
}

func TestDeploymentHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &DeploymentHandlers{}

	deployment := createTestDeployment("test-deployment", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Deployment",
			APIVersion:       "apps/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.Deployment{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, deployment, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "Deployment", deployment.Kind)
	assert.Equal(t, "apps/v1", deployment.APIVersion)
}

func TestDeploymentHandlers_AfterMarshalling(t *testing.T) {
	handlers := &DeploymentHandlers{}

	deployment := createTestDeployment("test-deployment", "test-namespace")
	resourceModel := &model.Deployment{}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Test YAML
	testYAML := []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, deployment, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestDeploymentHandlers_GetMetadataTags(t *testing.T) {
	handlers := &DeploymentHandlers{}

	// Create deployment model with tags
	deploymentModel := &model.Deployment{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, deploymentModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestDeploymentHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &DeploymentHandlers{}

	// Create deployment with sensitive annotations and labels
	deployment := createTestDeployment("test-deployment", "test-namespace")
	deployment.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	deployment.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, deployment)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", deployment.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", deployment.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestDeploymentProcessor_Process(t *testing.T) {
	// Create test deployments with unique UIDs
	deployment1 := createTestDeployment("deployment-1", "namespace-1")
	deployment1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	deployment1.ResourceVersion = "1207"

	deployment2 := createTestDeployment("deployment-2", "namespace-2")
	deployment2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	deployment2.ResourceVersion = "1307"

	// Create fake client
	client := fake.NewClientset(deployment1, deployment2)
	apiClient := &apiserver.APIClient{Cl: client}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			NodeType:         orchestrator.K8sDeployment,
			Kind:             "Deployment",
			APIVersion:       "apps/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process deployments
	processor := processors.NewProcessor(&DeploymentHandlers{})
	result, listed, processed := processor.Process(ctx, []*appsv1.Deployment{deployment1, deployment2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorDeployment)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Deployments, 2)

	expectedDeployment1 := k8sTransformers.ExtractDeployment(ctx, deployment1)

	assert.Equal(t, expectedDeployment1.Metadata, metaMsg.Deployments[0].Metadata)
	assert.Equal(t, expectedDeployment1.ReplicasDesired, metaMsg.Deployments[0].ReplicasDesired)
	assert.Equal(t, expectedDeployment1.DeploymentStrategy, metaMsg.Deployments[0].DeploymentStrategy)
	assert.Equal(t, expectedDeployment1.MaxUnavailable, metaMsg.Deployments[0].MaxUnavailable)
	assert.Equal(t, expectedDeployment1.MaxSurge, metaMsg.Deployments[0].MaxSurge)
	assert.Equal(t, expectedDeployment1.Paused, metaMsg.Deployments[0].Paused)
	assert.Equal(t, expectedDeployment1.Selectors, metaMsg.Deployments[0].Selectors)
	assert.Equal(t, expectedDeployment1.Replicas, metaMsg.Deployments[0].Replicas)
	assert.Equal(t, expectedDeployment1.UpdatedReplicas, metaMsg.Deployments[0].UpdatedReplicas)
	assert.Equal(t, expectedDeployment1.ReadyReplicas, metaMsg.Deployments[0].ReadyReplicas)
	assert.Equal(t, expectedDeployment1.AvailableReplicas, metaMsg.Deployments[0].AvailableReplicas)
	assert.Equal(t, expectedDeployment1.UnavailableReplicas, metaMsg.Deployments[0].UnavailableReplicas)
	assert.Equal(t, expectedDeployment1.ConditionMessage, metaMsg.Deployments[0].ConditionMessage)
	assert.Equal(t, expectedDeployment1.ResourceRequirements, metaMsg.Deployments[0].ResourceRequirements)
	assert.Equal(t, expectedDeployment1.Tags, metaMsg.Deployments[0].Tags)
	assert.Equal(t, expectedDeployment1.Conditions, metaMsg.Deployments[0].Conditions)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.GroupId)
	assert.Equal(t, "test-host", manifestMsg.HostName)
	assert.Len(t, manifestMsg.Manifests, 2)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)

	// Validate manifest details
	manifest1 := manifestMsg.Manifests[0]
	assert.Equal(t, deployment1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, deployment1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(18), manifest1.Type) // K8sDeployment
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestDeployment appsv1.Deployment
	err := json.Unmarshal(manifest1.Content, &actualManifestDeployment)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestDeployment.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestDeployment.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestDeployment.Status.Conditions[0].LastTransitionTime = metav1.NewTime(actualManifestDeployment.Status.Conditions[0].LastTransitionTime.UTC())
	actualManifestDeployment.Status.Conditions[1].LastTransitionTime = metav1.NewTime(actualManifestDeployment.Status.Conditions[1].LastTransitionTime.UTC())
	assert.Equal(t, deployment1.ObjectMeta, actualManifestDeployment.ObjectMeta)
	assert.Equal(t, deployment1.Spec, actualManifestDeployment.Spec)
	assert.Equal(t, deployment1.Status, actualManifestDeployment.Status)
}

func createTestDeployment(name, namespace string) *appsv1.Deployment {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	testIntOrStrPercent := intstr.FromString("1%")
	testInt32 := int32(3)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: "1207",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: appsv1.DeploymentSpec{
			MinReadySeconds:         600,
			ProgressDeadlineSeconds: &testInt32,
			Replicas:                &testInt32,
			RevisionHistoryLimit:    &testInt32,
			Paused:                  false,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-deploy",
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.DeploymentStrategyType("RollingUpdate"),
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &testIntOrStrPercent,
					MaxUnavailable: &testIntOrStrPercent,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-deploy",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			AvailableReplicas:   2,
			ObservedGeneration:  3,
			ReadyReplicas:       2,
			Replicas:            2,
			UpdatedReplicas:     2,
			UnavailableReplicas: 0,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:               appsv1.DeploymentAvailable,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: creationTime,
					Reason:             "MinimumReplicasAvailable",
					Message:            "Deployment has minimum availability.",
				},
				{
					Type:               appsv1.DeploymentProgressing,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: creationTime,
					Reason:             "NewReplicaSetAvailable",
					Message:            `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
				},
			},
		},
	}
}
