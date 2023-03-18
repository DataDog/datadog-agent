// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"fmt"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExtractDeployment(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	testInt32 := int32(2)
	testIntorStr := intstr.FromString("1%")
	tests := map[string]struct {
		input    appsv1.Deployment
		expected model.Deployment
	}{
		"full deploy": {
			input: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
					Name:              "deploy",
					Namespace:         "namespace",
					CreationTimestamp: timestamp,
					Labels: map[string]string{
						"label": "foo",
					},
					Annotations: map[string]string{
						"annotation": "bar",
					},
					ResourceVersion: "1234",
				},
				Spec: appsv1.DeploymentSpec{
					MinReadySeconds:         600,
					ProgressDeadlineSeconds: &testInt32,
					Replicas:                &testInt32,
					RevisionHistoryLimit:    &testInt32,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-deploy",
						},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.DeploymentStrategyType("RollingUpdate"),
						RollingUpdate: &appsv1.RollingUpdateDeployment{
							MaxSurge:       &testIntorStr,
							MaxUnavailable: &testIntorStr,
						},
					},
				},
				Status: appsv1.DeploymentStatus{
					AvailableReplicas:  2,
					ObservedGeneration: 3,
					ReadyReplicas:      2,
					Replicas:           2,
					UpdatedReplicas:    2,
					Conditions: []appsv1.DeploymentCondition{
						{
							Type:    appsv1.DeploymentAvailable,
							Status:  corev1.ConditionFalse,
							Reason:  "MinimumReplicasAvailable",
							Message: "Deployment has minimum availability.",
						},
						{
							Type:    appsv1.DeploymentProgressing,
							Status:  corev1.ConditionFalse,
							Reason:  "NewReplicaSetAvailable",
							Message: `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
						},
					},
				},
			}, expected: model.Deployment{
				Metadata: &model.Metadata{
					Name:              "deploy",
					Namespace:         "namespace",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
					CreationTimestamp: 1389744000,
					Labels:            []string{"label:foo"},
					Annotations:       []string{"annotation:bar"},
					ResourceVersion:   "1234",
				},
				ReplicasDesired:    2,
				DeploymentStrategy: "RollingUpdate",
				MaxUnavailable:     "1%",
				MaxSurge:           "1%",
				Paused:             false,
				Selectors: []*model.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "In",
						Values:   []string{"test-deploy"},
					},
				},
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   2,
				UnavailableReplicas: 0,
				ConditionMessage:    `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
			},
		},
		"empty deploy": {input: appsv1.Deployment{}, expected: model.Deployment{Metadata: &model.Metadata{}, ReplicasDesired: 1}},
		"deploy with resources": {
			input: appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{Template: getTemplateWithResourceRequirements()},
			},
			expected: model.Deployment{
				Metadata:             &model.Metadata{},
				ReplicasDesired:      1,
				ResourceRequirements: getExpectedModelResourceRequirements(),
			}},
		"partial deploy": {
			input: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy",
					Namespace: "namespace",
				},
				Spec: appsv1.DeploymentSpec{
					MinReadySeconds: 600,
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.DeploymentStrategyType("RollingUpdate"),
					},
				},
			}, expected: model.Deployment{
				ReplicasDesired: 1,
				Metadata: &model.Metadata{
					Name:      "deploy",
					Namespace: "namespace",
				},
				DeploymentStrategy: "RollingUpdate",
			},
		},
		"partial deploy with ust": {
			input: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels:    map[string]string{kubernetes.VersionTagLabelKey: "some-version"},
					Name:      "deploy",
					Namespace: "namespace",
				},
				Spec: appsv1.DeploymentSpec{
					MinReadySeconds: 600,
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.DeploymentStrategyType("RollingUpdate"),
					},
				},
			}, expected: model.Deployment{
				Tags:            []string{"version:some-version"},
				ReplicasDesired: 1,
				Metadata: &model.Metadata{
					Labels:    []string{"tags.datadoghq.com/version:some-version"},
					Name:      "deploy",
					Namespace: "namespace",
				},
				DeploymentStrategy: "RollingUpdate",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractDeployment(&tc.input))
		})
	}
}

func TestExtractDeploymentConditionMessage(t *testing.T) {
	for nb, tc := range []struct {
		conditions []appsv1.DeploymentCondition
		message    string
	}{
		{
			conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentReplicaFailure,
					Status:  corev1.ConditionFalse,
					Message: "foo",
				},
			},
			message: "foo",
		}, {
			conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Message: "foo",
				}, {
					Type:    appsv1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Message: "bar",
				},
			},
			message: "bar",
		}, {
			conditions: []appsv1.DeploymentCondition{
				{
					Type:    appsv1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Message: "foo",
				}, {
					Type:    appsv1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Message: "bar",
				},
			},
			message: "foo",
		},
	} {
		t.Run(fmt.Sprintf("case %d", nb), func(t *testing.T) {
			assert.EqualValues(t, tc.message, extractDeploymentConditionMessage(tc.conditions))
		})
	}
}
