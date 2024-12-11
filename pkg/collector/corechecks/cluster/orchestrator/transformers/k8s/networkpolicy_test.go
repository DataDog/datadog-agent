// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExtractNetworkPolicy(t *testing.T) {
	protocol := v1.Protocol("TCP")
	tests := map[string]struct {
		input    networkingv1.NetworkPolicy
		expected *model.NetworkPolicy
	}{
		"standard": {
			input: networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
				},
				Spec: networkingv1.NetworkPolicySpec{
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							From: []networkingv1.NetworkPolicyPeer{
								{
									IPBlock: &networkingv1.IPBlock{
										CIDR: "10.0.0.0/24",
									},
								},
							},
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
									Protocol: &protocol,
								},
							},
						},
					},
				},
			},
			expected: &model.NetworkPolicy{
				Metadata: &model.Metadata{
					Annotations: []string{"annotation:my-annotation"},
				},
				Spec: &model.NetworkPolicySpec{
					Ingress: []*model.NetworkPolicyIngressRule{
						{
							From: []*model.NetworkPolicyPeer{
								{
									IpBlock: &model.NetworkPolicyIPBlock{
										Cidr: "10.0.0.0/24",
									},
								},
							},
							Ports: []*model.NetworkPolicyPort{
								{
									Port:     80,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			},
		},
		"nil-safety": {
			input: networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       networkingv1.NetworkPolicySpec{},
			},
			expected: &model.NetworkPolicy{
				Metadata: &model.Metadata{},
				Spec:     &model.NetworkPolicySpec{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, ExtractNetworkPolicy(&tc.input))
		})
	}
}

func TestExtractNetworkPolicySpec(t *testing.T) {
	protocol := v1.Protocol("TCP")
	tests := map[string]struct {
		input    networkingv1.NetworkPolicySpec
		expected *model.NetworkPolicySpec
	}{
		"standard": {
			input: networkingv1.NetworkPolicySpec{
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{
						From: []networkingv1.NetworkPolicyPeer{
							{
								IPBlock: &networkingv1.IPBlock{
									CIDR: "10.0.0.0/24",
								},
							},
						},
						Ports: []networkingv1.NetworkPolicyPort{
							{
								Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
								Protocol: &protocol,
							},
						},
					},
				},
			},
			expected: &model.NetworkPolicySpec{
				Ingress: []*model.NetworkPolicyIngressRule{
					{
						From: []*model.NetworkPolicyPeer{
							{
								IpBlock: &model.NetworkPolicyIPBlock{
									Cidr: "10.0.0.0/24",
								},
							},
						},
						Ports: []*model.NetworkPolicyPort{
							{
								Port:     80,
								Protocol: "TCP",
							},
						},
					},
				},
			},
		},
		"nil-safety": {
			input: networkingv1.NetworkPolicySpec{},
			expected: &model.NetworkPolicySpec{
				Ingress: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicySpec(&tc.input))
		})
	}
}

func TestExtractNetworkPolicyIngressRule(t *testing.T) {
	protocol := v1.Protocol("TCP")
	tests := map[string]struct {
		input    networkingv1.NetworkPolicyIngressRule
		expected *model.NetworkPolicyIngressRule
	}{
		"standard": {
			input: networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: "10.0.0.0/24",
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
						Protocol: &protocol,
					},
				},
			},
			expected: &model.NetworkPolicyIngressRule{
				From: []*model.NetworkPolicyPeer{
					{
						IpBlock: &model.NetworkPolicyIPBlock{
							Cidr: "10.0.0.0/24",
						},
					},
				},
				Ports: []*model.NetworkPolicyPort{
					{
						Port:     80,
						Protocol: "TCP",
					},
				},
			},
		},
		"nil-safety": {
			input: networkingv1.NetworkPolicyIngressRule{},
			expected: &model.NetworkPolicyIngressRule{
				From:  []*model.NetworkPolicyPeer{},
				Ports: []*model.NetworkPolicyPort{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicyIngressRule(&tc.input))
		})
	}
}

func TestExtractNetworkPolicyEgressRule(t *testing.T) {
	protocol := v1.Protocol("TCP")
	tests := map[string]struct {
		input    networkingv1.NetworkPolicyEgressRule
		expected *model.NetworkPolicyEgressRule
	}{
		"standard": {
			input: networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: "10.0.0.0/24",
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
						Protocol: &protocol,
					},
				},
			},
			expected: &model.NetworkPolicyEgressRule{
				To: []*model.NetworkPolicyPeer{
					{
						IpBlock: &model.NetworkPolicyIPBlock{
							Cidr: "10.0.0.0/24",
						},
					},
				},
				Ports: []*model.NetworkPolicyPort{
					{
						Port:     80,
						Protocol: "TCP",
					},
				},
			},
		},
		"nil-safety": {
			input: networkingv1.NetworkPolicyEgressRule{},
			expected: &model.NetworkPolicyEgressRule{
				To:    []*model.NetworkPolicyPeer{},
				Ports: []*model.NetworkPolicyPort{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicyEgressRule(&tc.input))
		})
	}
}

func TestExtractNetworkPolicyPeer(t *testing.T) {
	tests := map[string]struct {
		input    networkingv1.NetworkPolicyPeer
		expected *model.NetworkPolicyPeer
	}{
		"standard": {
			input: networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{
					CIDR: "10.0.0.0/24",
				},
			},
			expected: &model.NetworkPolicyPeer{
				IpBlock: &model.NetworkPolicyIPBlock{
					Cidr: "10.0.0.0/24",
				},
			},
		},
		"nil-safety": {
			input: networkingv1.NetworkPolicyPeer{},
			expected: &model.NetworkPolicyPeer{
				IpBlock:           nil,
				NamespaceSelector: nil,
				PodSelector:       nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicyPeer(&tc.input))
		})
	}
}

func TestExtractNetworkPolicyIPBlock(t *testing.T) {
	tests := map[string]struct {
		input    networkingv1.IPBlock
		expected *model.NetworkPolicyIPBlock
	}{
		"standard": {
			input: networkingv1.IPBlock{
				CIDR: "10.0.0.0/24",
			},
			expected: &model.NetworkPolicyIPBlock{
				Cidr: "10.0.0.0/24",
			},
		},
		"nil-safety": {
			input: networkingv1.IPBlock{},
			expected: &model.NetworkPolicyIPBlock{
				Cidr:   "",
				Except: nil,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicyIPBlock(&tc.input))
		})
	}
}

func TestExtractNetworkPolicyPort(t *testing.T) {
	protocol := v1.Protocol("TCP")
	tests := map[string]struct {
		input    networkingv1.NetworkPolicyPort
		expected *model.NetworkPolicyPort
	}{
		"standard": {
			input: networkingv1.NetworkPolicyPort{
				Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
				Protocol: &protocol,
			},
			expected: &model.NetworkPolicyPort{
				Port:     80,
				Protocol: "TCP",
			},
		},
		"nil-safety": {
			input: networkingv1.NetworkPolicyPort{},
			expected: &model.NetworkPolicyPort{
				Port:     0,
				Protocol: "",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicyPort(&tc.input))
		})
	}
}

func TestExtractNetworkPolicyTypes(t *testing.T) {
	tests := map[string]struct {
		input    []networkingv1.PolicyType
		expected []string
	}{
		"standard": {
			input: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			expected: []string{
				"Ingress",
				"Egress",
			},
		},
		"nil-safety": {
			input:    nil,
			expected: nil,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractNetworkPolicyTypes(tc.input))
		})
	}
}
