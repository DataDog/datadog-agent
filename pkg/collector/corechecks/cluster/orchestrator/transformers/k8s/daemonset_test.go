// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	model "github.com/DataDog/agent-payload/v5/process"

	"testing"
)

func TestExtractDaemonset(t *testing.T) {
	testIntOrStrPercent := intstr.FromString("1%")
	testIntOrStrNumber := intstr.FromInt(1)
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000

	tests := map[string]struct {
		input    v1.DaemonSet
		expected model.DaemonSet
	}{
		"empty ds": {input: v1.DaemonSet{}, expected: model.DaemonSet{Metadata: &model.Metadata{}, Spec: &model.DaemonSetSpec{}, Status: &model.DaemonSetStatus{}}},
		"ds with resources": {
			input: v1.DaemonSet{
				Spec: v1.DaemonSetSpec{Template: getTemplateWithResourceRequirements()},
			},
			expected: model.DaemonSet{
				Metadata: &model.Metadata{},
				Spec:     &model.DaemonSetSpec{ResourceRequirements: getExpectedModelResourceRequirements()},
				Status:   &model.DaemonSetStatus{},
			},
		},
		"ds with numeric rolling update options": {
			input: v1.DaemonSet{
				Spec: v1.DaemonSetSpec{
					UpdateStrategy: v1.DaemonSetUpdateStrategy{
						Type: v1.DaemonSetUpdateStrategyType("RollingUpdate"),
						RollingUpdate: &v1.RollingUpdateDaemonSet{
							MaxUnavailable: &testIntOrStrNumber,
						},
					},
				},
			}, expected: model.DaemonSet{
				Metadata: &model.Metadata{},
				Spec: &model.DaemonSetSpec{
					DeploymentStrategy: "RollingUpdate",
					MaxUnavailable:     "1",
				},
				Status: &model.DaemonSetStatus{},
			},
		},
		"partial ds": {
			input: v1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "daemonset",
					Namespace: "namespace",
				},
				Spec: v1.DaemonSetSpec{
					UpdateStrategy: v1.DaemonSetUpdateStrategy{
						Type: v1.DaemonSetUpdateStrategyType("RollingUpdate"),
						RollingUpdate: &v1.RollingUpdateDaemonSet{
							MaxUnavailable: &testIntOrStrPercent,
						},
					},
				},
				Status: v1.DaemonSetStatus{
					Conditions: []v1.DaemonSetCondition{
						{
							Type:               "Test",
							Status:             corev1.ConditionFalse,
							LastTransitionTime: timestamp,
							Reason:             "test reason",
							Message:            "test message",
						},
					},
					CurrentNumberScheduled: 1,
					NumberReady:            1,
				},
			}, expected: model.DaemonSet{
				Metadata: &model.Metadata{
					Name:      "daemonset",
					Namespace: "namespace",
				},
				Conditions: []*model.DaemonSetCondition{
					{
						Type:               "Test",
						Status:             string(corev1.ConditionFalse),
						LastTransitionTime: timestamp.Unix(),
						Reason:             "test reason",
						Message:            "test message",
					},
				},
				Tags: []string{"kube_condition_test:false"},
				Spec: &model.DaemonSetSpec{
					DeploymentStrategy: "RollingUpdate",
					MaxUnavailable:     "1%",
				},
				Status: &model.DaemonSetStatus{
					CurrentNumberScheduled: 1,
					NumberReady:            1,
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractDaemonSet(&tc.input))
		})
	}
}
