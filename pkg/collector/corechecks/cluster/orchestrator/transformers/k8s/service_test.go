// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExtractService(t *testing.T) {
	tests := map[string]struct {
		input    corev1.Service
		expected model.Service
	}{
		"ClusterIP": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "002631fc-4c10-11ea-8f60-02ad5c77d02b",
					Labels: map[string]string{
						"app": "app-1",
					},
					Name:            "cluster-ip-service",
					Namespace:       "project",
					ResourceVersion: "1234",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []corev1.ServicePort{
						{
							Name:       "port-1",
							Port:       1,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(1),
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-1"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeClusterIP,
				},
				Status: corev1.ServiceStatus{},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-1"},
					Name:              "cluster-ip-service",
					Namespace:         "project",
					Uid:               "002631fc-4c10-11ea-8f60-02ad5c77d02b",
					ResourceVersion:   "1234",
				},
				Spec: &model.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []*model.ServicePort{
						{
							Name:       "port-1",
							Port:       1,
							Protocol:   "TCP",
							TargetPort: "1",
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-1"},
						},
					},
					SessionAffinity: "None",
					Type:            "ClusterIP",
				},
				Status: &model.ServiceStatus{},
			},
		},
		"ExternalName": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "a4e8d7ef-224d-11ea-bfe5-02da21d58a25",
					Labels: map[string]string{
						"app": "app-2",
					},
					Name:      "external-name-service",
					Namespace: "project",
				},
				Spec: corev1.ServiceSpec{
					ExternalName: "my.service.example.com",
					Ports: []corev1.ServicePort{
						{
							Name:       "port-2",
							Port:       2,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(2),
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-2"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeExternalName,
				},
				Status: corev1.ServiceStatus{},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-2"},
					Name:              "external-name-service",
					Namespace:         "project",
					Uid:               "a4e8d7ef-224d-11ea-bfe5-02da21d58a25",
				},
				Spec: &model.ServiceSpec{
					ExternalName: "my.service.example.com",
					Ports: []*model.ServicePort{
						{
							Name:       "port-2",
							Port:       2,
							Protocol:   "TCP",
							TargetPort: "2",
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-2"},
						},
					},
					SessionAffinity: "None",
					Type:            "ExternalName",
				},
				Status: &model.ServiceStatus{},
			},
		},
		"LoadBalancer": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "77b66dc1-6d14-11ea-a6ec-12daacdf7c55",
					Labels: map[string]string{
						"app": "app-3",
					},
					Name:      "loadbalancer-service",
					Namespace: "project",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "port-3",
							Port:       3,
							Protocol:   "TCP",
							TargetPort: intstr.FromInt(3),
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-3"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP: "192.0.2.127",
							},
						},
					},
				},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-3"},
					Name:              "loadbalancer-service",
					Namespace:         "project",
					Uid:               "77b66dc1-6d14-11ea-a6ec-12daacdf7c55",
				},
				Spec: &model.ServiceSpec{
					Ports: []*model.ServicePort{
						{
							Name:       "port-3",
							Port:       3,
							Protocol:   "TCP",
							TargetPort: "3",
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-3"},
						},
					},
					SessionAffinity: "None",
					Type:            "LoadBalancer",
				},
				Status: &model.ServiceStatus{
					LoadBalancerIngress: []string{"192.0.2.127"},
				},
			},
		},
		"NodePort": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "dfd0172f-1124-11ea-9888-02e48d9f4c6f",
					Labels: map[string]string{
						"app": "app-4",
					},
					Name:      "nodeport-service",
					Namespace: "project",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "port-4",
							Port:       4,
							Protocol:   "TCP",
							TargetPort: intstr.FromInt(4),
							NodePort:   30004,
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-4"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeNodePort,
				},
				Status: corev1.ServiceStatus{},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-4"},
					Name:              "nodeport-service",
					Namespace:         "project",
					Uid:               "dfd0172f-1124-11ea-9888-02e48d9f4c6f",
				},
				Spec: &model.ServiceSpec{
					Ports: []*model.ServicePort{
						{
							Name:       "port-4",
							Port:       4,
							Protocol:   "TCP",
							TargetPort: "4",
							NodePort:   30004,
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-4"},
						},
					},
					SessionAffinity: "None",
					Type:            "NodePort",
				},
				Status: &model.ServiceStatus{},
			},
		},
	}
	for _, test := range tests {
		assert.Equal(t, &test.expected, ExtractService(&test.input))
	}
}
