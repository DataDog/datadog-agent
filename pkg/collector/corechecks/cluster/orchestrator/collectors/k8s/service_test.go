// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	model "github.com/DataDog/agent-payload/v5/process"

	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestServiceCollector(t *testing.T) {
	creationTime := CreateTestTime()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "service",
			Namespace:       "namespace",
			ResourceVersion: "1223",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 8080,
					},
				},
				{
					Name:     "https",
					Protocol: corev1.ProtocolTCP,
					Port:     443,
					TargetPort: intstr.IntOrString{
						IntVal: 8443,
					},
				},
			},
			Selector: map[string]string{
				"app": "test-app",
			},
			ClusterIP:       "10.96.0.1",
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityClientIP,
			SessionAffinityConfig: &corev1.SessionAffinityConfig{
				ClientIP: &corev1.ClientIPConfig{
					TimeoutSeconds: &[]int32{10800}[0],
				},
			},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP: "192.168.1.1",
					},
				},
			},
		},
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewServiceCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{service},
		ExpectedMetadataType:       &model.CollectorService{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
