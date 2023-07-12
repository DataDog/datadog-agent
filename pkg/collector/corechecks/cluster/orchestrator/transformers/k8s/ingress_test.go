// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractIngress(t *testing.T) {
	pathType := netv1.PathTypeImplementationSpecific

	tests := map[string]struct {
		input    netv1.Ingress
		expected model.Ingress
	}{
		"empty": {input: netv1.Ingress{}, expected: model.Ingress{Metadata: &model.Metadata{}, Spec: &model.IngressSpec{}, Status: &model.IngressStatus{}}},
		"with spec and status": {
			input: netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "ingress",
					Namespace:   "namespace",
					Annotations: map[string]string{"key": "val"},
				},
				Spec: netv1.IngressSpec{
					Rules: []netv1.IngressRule{
						{
							Host: "*.host.com",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{Paths: []netv1.HTTPIngressPath{
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
					LoadBalancer: v1.LoadBalancerStatus{
						Ingress: []v1.LoadBalancerIngress{
							{Hostname: "foo.us-east-1.elb.amazonaws.com"},
						},
					},
				},
			},
			expected: model.Ingress{
				Metadata: &model.Metadata{
					Name:        "ingress",
					Namespace:   "namespace",
					Annotations: []string{"key:val"},
				},
				Spec: &model.IngressSpec{
					DefaultBackend: &model.IngressBackend{
						Resource: &model.TypedLocalObjectReference{
							ApiGroup: "apiGroup",
							Kind:     "kind",
							Name:     "name",
						},
					},
					Rules: []*model.IngressRule{
						{
							Host: "*.host.com",
							HttpPaths: []*model.HTTPIngressPath{
								{
									Path:     "/*",
									PathType: "ImplementationSpecific",
									Backend: &model.IngressBackend{
										Service: &model.IngressServiceBackend{
											ServiceName: "service",
											PortName:    "https",
											PortNumber:  443,
										},
									},
								},
							},
						},
					},
					Tls: []*model.IngressTLS{
						{Hosts: []string{"*.host.com"}, SecretName: "secret"},
					},
					IngressClassName: "ingressClassName",
				},
				Status: &model.IngressStatus{
					Ingress: []*model.LoadBalancerIngress{
						{Hostname: "foo.us-east-1.elb.amazonaws.com"},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractIngress(&tc.input))
		})
	}
}

func TestExtractIngressStatus(t *testing.T) {
	errMsg := "error"
	tests := map[string]struct {
		input    netv1.IngressStatus
		expected model.IngressStatus
	}{
		"empty": {input: netv1.IngressStatus{}, expected: model.IngressStatus{Ingress: []*model.LoadBalancerIngress{}}},
		"multiple ingress statuses": {
			input: netv1.IngressStatus{
				LoadBalancer: v1.LoadBalancerStatus{
					Ingress: []v1.LoadBalancerIngress{
						{IP: "ip1", Ports: []v1.PortStatus{{Port: 80, Error: &errMsg}}},
						{Hostname: "hostname1", Ports: []v1.PortStatus{{Protocol: "TCP", Port: 8080}}},
						{IP: "ip2"},
					},
				},
			},
			expected: model.IngressStatus{
				Ingress: []*model.LoadBalancerIngress{
					{Ip: "ip1", Ports: []*model.PortStatus{{Port: 80, Error: "error"}}},
					{Hostname: "hostname1", Ports: []*model.PortStatus{{Protocol: "TCP", Port: 8080}}},
					{Ip: "ip2"},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractIngressStatus(tc.input))
		})
	}
}

func TestExtractIngressRules(t *testing.T) {
	pathType1 := netv1.PathTypeImplementationSpecific
	pathType2 := netv1.PathTypeExact
	tests := map[string]struct {
		input    []netv1.IngressRule
		expected []*model.IngressRule
	}{
		"empty": {input: []netv1.IngressRule{}, expected: []*model.IngressRule{}},
		"multiple rules": {
			input: []netv1.IngressRule{
				{
					Host: "host1",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{Path: "path1-1", PathType: &pathType1},
								{Path: "path1-2", PathType: &pathType1},
							},
						},
					},
				},
				{
					Host: "host2",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{Path: "path2-1", PathType: &pathType2},
								{Path: "path2-2", PathType: &pathType2},
							},
						},
					},
				},
			},
			expected: []*model.IngressRule{
				{
					Host: "host1",
					HttpPaths: []*model.HTTPIngressPath{
						{Path: "path1-1", PathType: "ImplementationSpecific", Backend: &model.IngressBackend{}},
						{Path: "path1-2", PathType: "ImplementationSpecific", Backend: &model.IngressBackend{}},
					},
				},
				{
					Host: "host2",
					HttpPaths: []*model.HTTPIngressPath{
						{Path: "path2-1", PathType: "Exact", Backend: &model.IngressBackend{}},
						{Path: "path2-2", PathType: "Exact", Backend: &model.IngressBackend{}},
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractIngressRules(tc.input))
		})
	}
}

func TestExtractIngressBackend(t *testing.T) {
	tests := map[string]struct {
		input    netv1.IngressBackend
		expected model.IngressBackend
	}{
		"empty": {input: netv1.IngressBackend{}, expected: model.IngressBackend{}},
		"with resource": {
			input: netv1.IngressBackend{
				Resource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.Ptr("apiGroup"),
					Kind:     "kind",
					Name:     "name",
				},
			},
			expected: model.IngressBackend{
				Resource: &model.TypedLocalObjectReference{
					ApiGroup: "apiGroup",
					Kind:     "kind",
					Name:     "name",
				},
			},
		},
		"with service and port name": {
			input: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: "service",
					Port: netv1.ServiceBackendPort{
						Name: "https",
					},
				},
			},
			expected: model.IngressBackend{
				Service: &model.IngressServiceBackend{
					ServiceName: "service",
					PortName:    "https",
				},
			},
		},
		"with service and port number": {
			input: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: "service",
					Port: netv1.ServiceBackendPort{
						Number: 443,
					},
				},
			},
			expected: model.IngressBackend{
				Service: &model.IngressServiceBackend{
					ServiceName: "service",
					PortNumber:  443,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractIngressBackend(&tc.input))
		})
	}
}

func TestExtractIngressTLS(t *testing.T) {
	tests := map[string]struct {
		input    []netv1.IngressTLS
		expected []*model.IngressTLS
	}{
		"empty": {input: []netv1.IngressTLS{}, expected: []*model.IngressTLS{}},
		"multiple TLS": {
			input: []netv1.IngressTLS{
				{Hosts: []string{"host1-1", "host1-2"}, SecretName: "secret1"},
				{Hosts: []string{"host2"}, SecretName: "secret2"},
				{Hosts: []string{"host3"}, SecretName: "secret3"},
			},
			expected: []*model.IngressTLS{
				{Hosts: []string{"host1-1", "host1-2"}, SecretName: "secret1"},
				{Hosts: []string{"host2"}, SecretName: "secret2"},
				{Hosts: []string{"host3"}, SecretName: "secret3"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractIngressTLS(tc.input))
		})
	}
}
