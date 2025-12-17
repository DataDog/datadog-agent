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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestLimitRangeHandlers_ExtractResource(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	// Create test limit range
	limitRange := createTestLimitRange("test-limitrange", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, limitRange)

	// Validate extraction
	limitRangeModel, ok := resourceModel.(*model.LimitRange)
	assert.True(t, ok)
	assert.NotNil(t, limitRangeModel)
	assert.Equal(t, "test-limitrange", limitRangeModel.Metadata.Name)
	assert.Equal(t, "test-namespace", limitRangeModel.Metadata.Namespace)
	assert.NotNil(t, limitRangeModel.Spec)
	assert.Len(t, limitRangeModel.Spec.Limits, 2)
	assert.Contains(t, limitRangeModel.LimitTypes, "Container")
	assert.Contains(t, limitRangeModel.LimitTypes, "Pod")
}

func TestLimitRangeHandlers_ResourceList(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	// Create test limit ranges
	limitRange1 := createTestLimitRange("limitrange-1", "namespace-1")
	limitRange2 := createTestLimitRange("limitrange-2", "namespace-2")

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
	resourceList := []*corev1.LimitRange{limitRange1, limitRange2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*corev1.LimitRange)
	assert.True(t, ok)
	assert.Equal(t, "limitrange-1", resource1.Name)
	assert.NotSame(t, limitRange1, resource1) // Should be a copy

	resource2, ok := resources[1].(*corev1.LimitRange)
	assert.True(t, ok)
	assert.Equal(t, "limitrange-2", resource2.Name)
	assert.NotSame(t, limitRange2, resource2) // Should be a copy
}

func TestLimitRangeHandlers_ResourceUID(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	limitRange := createTestLimitRange("test-limitrange", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	limitRange.UID = expectedUID

	uid := handlers.ResourceUID(nil, limitRange)
	assert.Equal(t, expectedUID, uid)
}

func TestLimitRangeHandlers_ResourceVersion(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	limitRange := createTestLimitRange("test-limitrange", "test-namespace")
	expectedVersion := "v123"
	limitRange.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.LimitRange{}

	version := handlers.ResourceVersion(nil, limitRange, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestLimitRangeHandlers_BuildMessageBody(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	limitRange1 := createTestLimitRange("limitrange-1", "namespace-1")
	limitRange2 := createTestLimitRange("limitrange-2", "namespace-2")

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

	limitRange1Model := k8sTransformers.ExtractLimitRange(ctx, limitRange1)
	limitRange2Model := k8sTransformers.ExtractLimitRange(ctx, limitRange2)

	// Build message body
	resourceModels := []interface{}{limitRange1Model, limitRange2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorLimitRange)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.LimitRanges, 2)
	assert.Equal(t, "limitrange-1", collectorMsg.LimitRanges[0].Metadata.Name)
	assert.Equal(t, "limitrange-2", collectorMsg.LimitRanges[1].Metadata.Name)
}

func TestLimitRangeHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	limitRange := createTestLimitRange("test-limitrange", "test-namespace")

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

	// Set context values
	ctx.Kind = "LimitRange"
	ctx.APIVersion = "v1"

	// Create resource model
	resourceModel := &model.LimitRange{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, limitRange, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "LimitRange", limitRange.Kind)
	assert.Equal(t, "v1", limitRange.APIVersion)
}

func TestLimitRangeHandlers_AfterMarshalling(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	limitRange := createTestLimitRange("test-limitrange", "test-namespace")
	resourceModel := &model.LimitRange{}

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
	testYAML := []byte(`{"apiVersion":"v1","kind":"LimitRange","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, limitRange, resourceModel, testYAML)
	assert.False(t, skip)
}

func TestLimitRangeHandlers_GetMetadataTags(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	// Create limit range model with tags
	limitRangeModel := &model.LimitRange{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, limitRangeModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestLimitRangeHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &LimitRangeHandlers{}

	// Create limit range with sensitive annotations and labels
	limitRange := createTestLimitRange("test-limitrange", "test-namespace")
	limitRange.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	limitRange.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, limitRange)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", limitRange.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", limitRange.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestLimitRangeProcessor_Process(t *testing.T) {
	// Create test limit ranges with unique UIDs
	limitRange1 := createTestLimitRange("limitrange-1", "namespace-1")
	limitRange1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	limitRange1.ResourceVersion = "1211"

	limitRange2 := createTestLimitRange("limitrange-2", "namespace-2")
	limitRange2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	limitRange2.ResourceVersion = "1311"

	// Create fake client
	client := fake.NewClientset(limitRange1, limitRange2)
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
			NodeType:         orchestrator.K8sLimitRange,
			Kind:             "LimitRange",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process limit ranges
	processor := processors.NewProcessor(&LimitRangeHandlers{})
	result, listed, processed := processor.Process(ctx, []*corev1.LimitRange{limitRange1, limitRange2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorLimitRange)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.LimitRanges, 2)

	expectedLimitRange1 := k8sTransformers.ExtractLimitRange(ctx, limitRange1)

	assert.Equal(t, expectedLimitRange1.Metadata, metaMsg.LimitRanges[0].Metadata)
	assert.Equal(t, expectedLimitRange1.Spec, metaMsg.LimitRanges[0].Spec)
	assert.Equal(t, expectedLimitRange1.LimitTypes, metaMsg.LimitRanges[0].LimitTypes)
	assert.Equal(t, expectedLimitRange1.Tags, metaMsg.LimitRanges[0].Tags)

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
	assert.Equal(t, limitRange1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, limitRange1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(25), manifest1.Type) // K8sLimitRange
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestLimitRange corev1.LimitRange
	err := json.Unmarshal(manifest1.Content, &actualManifestLimitRange)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestLimitRange.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestLimitRange.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, limitRange1.ObjectMeta, actualManifestLimitRange.ObjectMeta)
	assert.Equal(t, limitRange1.Spec, actualManifestLimitRange.Spec)
}

func createTestLimitRange(name, namespace string) *corev1.LimitRange {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			ResourceVersion:   "1211",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("50Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					MaxLimitRequestRatio: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("2"),
					},
					Min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("10Mi"),
					},
				},
				{
					Type: corev1.LimitTypePod,
					Default: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("500Mi"),
					},
					DefaultRequest: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("250Mi"),
					},
					Max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("1000Mi"),
					},
					Min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			},
		},
	}
}
