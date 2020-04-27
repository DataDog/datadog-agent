// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/process"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExtractDeployment(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	testInt32 := int32(2)
	testIntorStr := intstr.FromString("1%")
	tests := map[string]struct {
		input    v1.Deployment
		expected model.Deployment
	}{
		"full deploy": {
			input: v1.Deployment{
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
				},
				Spec: v1.DeploymentSpec{
					MinReadySeconds:         600,
					ProgressDeadlineSeconds: &testInt32,
					Replicas:                &testInt32,
					RevisionHistoryLimit:    &testInt32,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-deploy",
						},
					},
					Strategy: v1.DeploymentStrategy{
						Type: v1.DeploymentStrategyType("RollingUpdate"),
						RollingUpdate: &v1.RollingUpdateDeployment{
							MaxSurge:       &testIntorStr,
							MaxUnavailable: &testIntorStr,
						},
					},
				},
				Status: v1.DeploymentStatus{
					AvailableReplicas:  2,
					ObservedGeneration: 3,
					ReadyReplicas:      2,
					Replicas:           2,
					UpdatedReplicas:    2,
					Conditions: []v1.DeploymentCondition{
						{
							Reason:  "MinimumReplicasAvailable",
							Message: "Deployment has minimum availability.",
						},
						{
							Reason:  "NewReplicaSetAvailable",
							Message: "ReplicaSet \"orchestrator-intake-6d65b45d4d\" has successfully progressed.",
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
				ConditionMessage:    "Deployment has minimum availability.ReplicaSet \"orchestrator-intake-6d65b45d4d\" has successfully progressed.",
			},
		},
		"empty deploy": {input: v1.Deployment{}, expected: model.Deployment{Metadata: &model.Metadata{}, ReplicasDesired: 1}},
		"partial deploy": {
			input: v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy",
					Namespace: "namespace",
				},
				Spec: v1.DeploymentSpec{
					MinReadySeconds: 600,
					Strategy: v1.DeploymentStrategy{
						Type: v1.DeploymentStrategyType("Recreate"),
					},
				},
			}, expected: model.Deployment{
				ReplicasDesired: 1,
				Metadata: &model.Metadata{
					Name:      "deploy",
					Namespace: "namespace",
				},
				DeploymentStrategy: "Recreate",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractDeployment(&tc.input))
		})
	}
}
