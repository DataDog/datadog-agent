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
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestPodDisruptionBudgetHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.PodDisruptionBudget{}
	resource := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pdb",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("policy", "poddisruptionbudgets")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewPodDisruptionBudgetHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
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
	processor := processors.NewProcessor(NewPodDisruptionBudgetHandlers(processorstest.NewEmptyFakeTagger()))
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
