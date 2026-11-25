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
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestIngressHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.Ingress{}
	resource := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("networking.k8s.io", "ingresses")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewIngressHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
}

func TestIngressProcessor_Process(t *testing.T) {
	// Create test ingresses with unique UIDs
	ingress1 := createTestIngress("ingress-1", "namespace-1")
	ingress1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	ingress1.ResourceVersion = "1209"

	ingress2 := createTestIngress("ingress-2", "namespace-2")
	ingress2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	ingress2.ResourceVersion = "1309"

	// Create fake client
	client := fake.NewClientset(ingress1, ingress2)
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
			NodeType:         orchestrator.K8sIngress,
			Kind:             "Ingress",
			APIVersion:       "networking.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process ingresses
	processor := processors.NewProcessor(NewIngressHandlers(processorstest.NewEmptyFakeTagger()))
	result, listed, processed := processor.Process(ctx, []*netv1.Ingress{ingress1, ingress2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorIngress)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Ingresses, 2)

	expectedIngress1 := k8sTransformers.ExtractIngress(ctx, ingress1)

	assert.Equal(t, expectedIngress1.Metadata, metaMsg.Ingresses[0].Metadata)
	assert.Equal(t, expectedIngress1.Spec, metaMsg.Ingresses[0].Spec)
	assert.Equal(t, expectedIngress1.Status, metaMsg.Ingresses[0].Status)
	assert.Equal(t, expectedIngress1.Tags, metaMsg.Ingresses[0].Tags)

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
	assert.Equal(t, ingress1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, ingress1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(17), manifest1.Type) // K8sIngress
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestIngress netv1.Ingress
	err := json.Unmarshal(manifest1.Content, &actualManifestIngress)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestIngress.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestIngress.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, ingress1.ObjectMeta, actualManifestIngress.ObjectMeta)
	assert.Equal(t, ingress1.Spec, actualManifestIngress.Spec)
	assert.Equal(t, ingress1.Status, actualManifestIngress.Status)
}

func createTestIngress(name, namespace string) *netv1.Ingress {
	pathType := netv1.PathTypeImplementationSpecific
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			ResourceVersion:   "1209",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: pointer.Ptr("nginx"),
			DefaultBackend: &netv1.IngressBackend{
				Resource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.Ptr("apiGroup"),
					Kind:     "kind",
					Name:     "name",
				},
			},
			Rules: []netv1.IngressRule{
				{
					Host: "*.host.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/*",
									PathType: &pathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "service",
											Port: netv1.ServiceBackendPort{
												Number: 443,
												Name:   "https",
											},
										},
									},
								},
								{
									Path:     "/api/*",
									PathType: &pathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "api-service",
											Port: netv1.ServiceBackendPort{
												Number: 8080,
												Name:   "http",
											},
										},
									},
								},
							},
						},
					},
				},
				{
					Host: "api.example.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "api-service",
											Port: netv1.ServiceBackendPort{
												Number: 8080,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []netv1.IngressTLS{
				{
					Hosts:      []string{"*.host.com", "api.example.com"},
					SecretName: "tls-secret",
				},
			},
		},
		Status: netv1.IngressStatus{
			LoadBalancer: netv1.IngressLoadBalancerStatus{
				Ingress: []netv1.IngressLoadBalancerIngress{
					{
						Hostname: "foo.us-east-1.elb.amazonaws.com",
						IP:       "192.168.1.1",
						Ports: []netv1.IngressPortStatus{
							{
								Port:     80,
								Protocol: v1.ProtocolTCP,
							},
							{
								Port:     443,
								Protocol: v1.ProtocolTCP,
							},
						},
					},
					{
						Hostname: "bar.us-west-2.elb.amazonaws.com",
						IP:       "192.168.1.2",
					},
				},
			},
		},
	}
}
