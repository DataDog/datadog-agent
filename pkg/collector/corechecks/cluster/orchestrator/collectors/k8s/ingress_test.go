// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestIngressCollector(t *testing.T) {
	pathType1 := netv1.PathTypeImplementationSpecific

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ingress",
			Namespace:       "namespace",
			Annotations:     map[string]string{"annotation": "my-annotation"},
			Labels:          map[string]string{"app": "my-app"},
			UID:             "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion: "1211",
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "*.host.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{Paths: []netv1.HTTPIngressPath{
							{
								Path:     "/*",
								PathType: &pathType1,
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
						}},
					},
				},
			},
			DefaultBackend: &netv1.IngressBackend{
				Resource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.Ptr("apiGroup"),
					Kind:     "kind",
					Name:     "name",
				},
			},
			TLS: []netv1.IngressTLS{
				{
					Hosts:      []string{"*.host.com"},
					SecretName: "secret",
				},
			},
			IngressClassName: pointer.Ptr("ingressClassName"),
		},
		Status: netv1.IngressStatus{
			LoadBalancer: netv1.IngressLoadBalancerStatus{
				Ingress: []netv1.IngressLoadBalancerIngress{
					{Hostname: "foo.us-east-1.elb.amazonaws.com"},
				},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewIngressCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{ingress},
		ExpectedMetadataType:       &model.CollectorIngress{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
