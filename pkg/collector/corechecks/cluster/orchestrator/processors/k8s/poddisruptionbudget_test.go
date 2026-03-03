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
	policyv1 "k8s.io/api/policy/v1"
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

func TestPodDisruptionBudgetHandlers_ExtractResource(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	// Create test pod disruption budget
	pdb := createTestPodDisruptionBudget()

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
	resourceModel := handlers.ExtractResource(ctx, pdb)

	// Validate extraction
	pdbModel, ok := resourceModel.(*model.PodDisruptionBudget)
	assert.True(t, ok)
	assert.NotNil(t, pdbModel)
	assert.Equal(t, "test-pdb", pdbModel.Metadata.Name)
	assert.Equal(t, "default", pdbModel.Metadata.Namespace)
	assert.NotNil(t, pdbModel.Spec)
	assert.Equal(t, int32(95), pdbModel.Spec.MinAvailable.IntVal)
}

func TestPodDisruptionBudgetHandlers_ResourceList(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	// Create test pod disruption budgets
	pdb1 := createTestPodDisruptionBudget()
	pdb2 := createTestPodDisruptionBudget()
	pdb2.Name = "pdb2"
	pdb2.UID = "uid2"

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
	resourceList := []*policyv1.PodDisruptionBudget{pdb1, pdb2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*policyv1.PodDisruptionBudget)
	assert.True(t, ok)
	assert.Equal(t, "test-pdb", resource1.Name)
	assert.NotSame(t, pdb1, resource1) // Should be a copy

	resource2, ok := resources[1].(*policyv1.PodDisruptionBudget)
	assert.True(t, ok)
	assert.Equal(t, "pdb2", resource2.Name)
	assert.NotSame(t, pdb2, resource2) // Should be a copy
}

func TestPodDisruptionBudgetHandlers_ResourceUID(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	pdb := createTestPodDisruptionBudget()
	expectedUID := types.UID("test-pdb-uid")
	pdb.UID = expectedUID

	uid := handlers.ResourceUID(nil, pdb)
	assert.Equal(t, expectedUID, uid)
}

func TestPodDisruptionBudgetHandlers_ResourceVersion(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	pdb := createTestPodDisruptionBudget()
	expectedVersion := "123"
	pdb.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, pdb, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestPodDisruptionBudgetHandlers_BuildMessageBody(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	pdb1 := createTestPodDisruptionBudget()
	pdb2 := createTestPodDisruptionBudget()
	pdb2.Name = "pdb2"

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

	pdb1Model := k8sTransformers.ExtractPodDisruptionBudget(ctx, pdb1)
	pdb2Model := k8sTransformers.ExtractPodDisruptionBudget(ctx, pdb2)

	// Build message body
	resourceModels := []interface{}{pdb1Model, pdb2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorPodDisruptionBudget)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.PodDisruptionBudgets, 2)
	assert.Equal(t, "test-pdb", collectorMsg.PodDisruptionBudgets[0].Metadata.Name)
	assert.Equal(t, "pdb2", collectorMsg.PodDisruptionBudgets[1].Metadata.Name)
}

func TestPodDisruptionBudgetHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	pdb := createTestPodDisruptionBudget()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "PodDisruptionBudget",
			APIVersion:       "policy/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.PodDisruptionBudget{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, pdb, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "PodDisruptionBudget", pdb.Kind)
	assert.Equal(t, "policy/v1", pdb.APIVersion)
}

func TestPodDisruptionBudgetHandlers_AfterMarshalling(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	pdb := createTestPodDisruptionBudget()

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

	// Create resource model
	resourceModel := &model.PodDisruptionBudget{}

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, pdb, resourceModel, nil)

	// Validate
	assert.False(t, skip)
}

func TestPodDisruptionBudgetHandlers_GetMetadataTags(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	// Create a pod disruption budget model with tags
	pdbModel := &model.PodDisruptionBudget{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, pdbModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestPodDisruptionBudgetHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &PodDisruptionBudgetHandlers{}

	// Create pod disruption budget with sensitive annotations and labels
	pdb := createTestPodDisruptionBudget()
	pdb.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	pdb.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, pdb)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", pdb.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", pdb.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestPodDisruptionBudgetProcessor_Process(t *testing.T) {
	// Create test pod disruption budgets with unique UIDs
	pdb1 := createTestPodDisruptionBudget()
	pdb1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	pdb1.ResourceVersion = "1218"

	pdb2 := createTestPodDisruptionBudget()
	pdb2.Name = "pdb2"
	pdb2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	pdb2.ResourceVersion = "1318"

	// Create fake client
	client := fake.NewClientset(pdb1, pdb2)
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
			NodeType:         orchestrator.K8sPodDisruptionBudget,
			Kind:             "PodDisruptionBudget",
			APIVersion:       "policy/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process pod disruption budgets
	processor := processors.NewProcessor(&PodDisruptionBudgetHandlers{})
	result, listed, processed := processor.Process(ctx, []*policyv1.PodDisruptionBudget{pdb1, pdb2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorPodDisruptionBudget)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.PodDisruptionBudgets, 2)

	expectedPdb1 := k8sTransformers.ExtractPodDisruptionBudget(ctx, pdb1)

	assert.Equal(t, expectedPdb1.Metadata, metaMsg.PodDisruptionBudgets[0].Metadata)
	assert.Equal(t, expectedPdb1.Spec, metaMsg.PodDisruptionBudgets[0].Spec)
	assert.Equal(t, expectedPdb1.Status, metaMsg.PodDisruptionBudgets[0].Status)
	assert.Equal(t, expectedPdb1.Tags, metaMsg.PodDisruptionBudgets[0].Tags)

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
	assert.Equal(t, pdb1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, pdb1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(27), manifest1.Type) // K8sPodDisruptionBudget
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestPdb policyv1.PodDisruptionBudget
	err := json.Unmarshal(manifest1.Content, &actualManifestPdb)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestPdb.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestPdb.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestPdb.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestPdb.Status.Conditions[0].LastTransitionTime.UTC()}
	actualManifestPdb.Status.DisruptedPods["test-pod"] = metav1.Time{Time: actualManifestPdb.Status.DisruptedPods["test-pod"].Time.UTC()}
	assert.Equal(t, pdb1.ObjectMeta, actualManifestPdb.ObjectMeta)
	assert.Equal(t, pdb1.Spec, actualManifestPdb.Spec)
	assert.Equal(t, pdb1.Status, actualManifestPdb.Status)
}

// Helper function to create a test pod disruption budget
func createTestPodDisruptionBudget() *policyv1.PodDisruptionBudget {
	iVal := int32(95)
	sVal := "reshape"
	iOSI := intstr.FromInt32(iVal)
	iOSS := intstr.FromString(sVal)
	var labels = map[string]string{"reshape": "all"}
	ePolicy := policyv1.AlwaysAllow
	t0 := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	t1 := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pdb",
			Namespace:       "default",
			UID:             "test-pdb-uid",
			ResourceVersion: "1218",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   &iOSI,
			MaxUnavailable: &iOSS,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UnhealthyPodEvictionPolicy: &ePolicy,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 3,
			DisruptedPods:      map[string]metav1.Time{"test-pod": t0},
			DisruptionsAllowed: 4,
			CurrentHealthy:     5,
			DesiredHealthy:     6,
			ExpectedPods:       7,
			Conditions: []metav1.Condition{
				{
					Type:               "regular",
					Status:             metav1.ConditionUnknown,
					ObservedGeneration: 2,
					LastTransitionTime: t1,
					Reason:             "why not",
					Message:            "instant",
				},
			},
		},
	}
}
