// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	"github.com/stretchr/testify/assert"
)

func TestConvertLabels(t *testing.T) {
	tests := []struct {
		name         string
		domainLabels []*kubeAutoscaling.DomainLabels
		expected     map[string]string
	}{
		{
			name: "basic",
			domainLabels: []*kubeAutoscaling.DomainLabels{
				{
					Key:   "foo",
					Value: "bar",
				},
			},
			expected: map[string]string{"foo": "bar"},
		},
		{
			name: "multiple",
			domainLabels: []*kubeAutoscaling.DomainLabels{
				{
					Key:   "foo",
					Value: "bar",
				},
				{
					Key:   "baz",
					Value: "qux",
				},
			},
			expected: map[string]string{"foo": "bar", "baz": "qux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertLabels(tt.domainLabels)
			assert.Equal(t, tt.expected, result, "Output of convertLabels does not match expected result")
		})
	}

}

func TestConvertTaints(t *testing.T) {
	tests := []struct {
		name     string
		taints   []*kubeAutoscaling.Taints
		expected []corev1.Taint
	}{
		{
			name: "basic",
			taints: []*kubeAutoscaling.Taints{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: "NoSchedule",
				},
			},
			expected: []corev1.Taint{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		{
			name: "basic",
			taints: []*kubeAutoscaling.Taints{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: "NoSchedule",
				},
				{
					Key:    "baz",
					Value:  "qux",
					Effect: "PreferNoSchedule",
				},
			},
			expected: []corev1.Taint{
				{
					Key:    "foo",
					Value:  "bar",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "baz",
					Value:  "qux",
					Effect: corev1.TaintEffectPreferNoSchedule,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTaints(tt.taints)
			assert.Equal(t, tt.expected, result, "Output of convertTaints does not match expected result")
		})
	}

}
