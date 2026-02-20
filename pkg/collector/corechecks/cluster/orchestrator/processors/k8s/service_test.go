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

func TestServiceHandlers_ExtractResource(t *testing.T) {
	handlers := &ServiceHandlers{}

	// Create test service
	service := createTestService()

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
	resourceModel := handlers.ExtractResource(ctx, service)

	// Validate extraction
	serviceModel, ok := resourceModel.(*model.Service)
	assert.True(t, ok)
	assert.NotNil(t, serviceModel)
	assert.Equal(t, "test-service", serviceModel.Metadata.Name)
	assert.Equal(t, "default", serviceModel.Metadata.Namespace)
	assert.Equal(t, "ClusterIP", serviceModel.Spec.Type)
	assert.Equal(t, "10.0.0.1", serviceModel.Spec.ClusterIP)
	assert.Len(t, serviceModel.Spec.Ports, 1)
	assert.Equal(t, "port-1", serviceModel.Spec.Ports[0].Name)
	assert.Equal(t, int32(80), serviceModel.Spec.Ports[0].Port)
}

func TestServiceHandlers_ResourceList(t *testing.T) {
	handlers := &ServiceHandlers{}

	// Create test services
	service1 := createTestService()
	service2 := createTestService()
	service2.Name = "service2"
	service2.UID = "uid2"

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
	resourceList := []*corev1.Service{service1, service2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*corev1.Service)
	assert.True(t, ok)
	assert.Equal(t, "test-service", resource1.Name)
	assert.NotSame(t, service1, resource1) // Should be a copy

	resource2, ok := resources[1].(*corev1.Service)
	assert.True(t, ok)
	assert.Equal(t, "service2", resource2.Name)
	assert.NotSame(t, service2, resource2) // Should be a copy
}

func TestServiceHandlers_ResourceUID(t *testing.T) {
	handlers := &ServiceHandlers{}

	service := createTestService()
	expectedUID := types.UID("test-service-uid")
	service.UID = expectedUID

	uid := handlers.ResourceUID(nil, service)
	assert.Equal(t, expectedUID, uid)
}

func TestServiceHandlers_ResourceVersion(t *testing.T) {
	handlers := &ServiceHandlers{}

	service := createTestService()
	expectedVersion := "123"
	service.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, service, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestServiceHandlers_BuildMessageBody(t *testing.T) {
	handlers := &ServiceHandlers{}

	service1 := createTestService()
	service2 := createTestService()
	service2.Name = "service2"

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

	service1Model := k8sTransformers.ExtractService(ctx, service1)
	service2Model := k8sTransformers.ExtractService(ctx, service2)

	// Build message body
	resourceModels := []interface{}{service1Model, service2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorService)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Services, 2)
	assert.Equal(t, "test-service", collectorMsg.Services[0].Metadata.Name)
	assert.Equal(t, "service2", collectorMsg.Services[1].Metadata.Name)
}

func TestServiceHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &ServiceHandlers{}

	service := createTestService()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Service",
			APIVersion:       "v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.Service{}
	skip := handlers.BeforeMarshalling(ctx, service, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "Service", service.Kind)
	assert.Equal(t, "v1", service.APIVersion)
}

func TestServiceHandlers_AfterMarshalling(t *testing.T) {
	handlers := &ServiceHandlers{}

	service := createTestService()

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
	resourceModel := &model.Service{}

	// Create test YAML
	testYAML := []byte("apiVersion: v1\nkind: Service\nmetadata:\n  name: test-service")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, service, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestServiceHandlers_GetMetadataTags(t *testing.T) {
	handlers := &ServiceHandlers{}

	// Create a service model with tags
	serviceModel := &model.Service{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, serviceModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestServiceHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &ServiceHandlers{}

	// Create service with sensitive annotations and labels
	service := createTestService()
	service.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	service.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, service)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", service.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", service.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestServiceProcessor_Process(t *testing.T) {
	// Create test services with unique UIDs
	service1 := createTestService()
	service1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	service1.ResourceVersion = "1222"

	service2 := createTestService()
	service2.Name = "service2"
	service2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	service2.ResourceVersion = "1322"

	// Create fake client
	client := fake.NewClientset(service1, service2)
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
			NodeType:         orchestrator.K8sService,
			Kind:             "Service",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process services
	processor := processors.NewProcessor(&ServiceHandlers{})
	result, listed, processed := processor.Process(ctx, []*corev1.Service{service1, service2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorService)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Services, 2)

	expectedService1 := k8sTransformers.ExtractService(ctx, service1)

	assert.Equal(t, expectedService1.Metadata, metaMsg.Services[0].Metadata)
	assert.Equal(t, expectedService1.Spec, metaMsg.Services[0].Spec)
	assert.Equal(t, expectedService1.Status, metaMsg.Services[0].Status)
	assert.Equal(t, expectedService1.Tags, metaMsg.Services[0].Tags)

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
	assert.Equal(t, service1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, service1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(3), manifest1.Type) // K8sService
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestService corev1.Service
	err := json.Unmarshal(manifest1.Content, &actualManifestService)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestService.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestService.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, service1.ObjectMeta, actualManifestService.ObjectMeta)
	assert.Equal(t, service1.Spec, actualManifestService.Spec)
	assert.Equal(t, service1.Status, actualManifestService.Status)
}

// Helper function to create a test service
func createTestService() *corev1.Service {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-service",
			Namespace:       "default",
			UID:             "test-service-uid",
			ResourceVersion: "1222",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: creationTime,
		},
		Spec: corev1.ServiceSpec{
			Type:        corev1.ServiceTypeClusterIP,
			ClusterIP:   "10.0.0.1",
			ExternalIPs: []string{"192.168.1.1"},
			Ports: []corev1.ServicePort{
				{
					Name:       "port-1",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(8080),
				},
			},
			Selector: map[string]string{
				"app": "test-app",
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
		Status: corev1.ServiceStatus{},
	}
}
