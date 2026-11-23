// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

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
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestDeploymentHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.Deployment{}
	resource := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("apps", "deployments")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesDeployment,
		resource.Namespace+"/"+resource.Name,
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewDeploymentHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
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
	processor := processors.NewProcessor(NewDeploymentHandlers(processorstest.NewEmptyFakeTagger()))
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
