// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractNamespace(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	tests := map[string]struct {
		input    corev1.Namespace
		expected model.Namespace
	}{
		"standard": {
			input: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					CreationTimestamp: creationTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Name:            "my-name",
					Namespace:       "my-namespace",
					ResourceVersion: "1234",
					Finalizers:      []string{"final", "izers"},
					UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
				},
				Status: corev1.NamespaceStatus{
					Phase: "a-phase",
					Conditions: []corev1.NamespaceCondition{
						{
							Type:    "NamespaceFinalizersRemaining",
							Status:  "False",
							Message: "wrong msg",
						},
						{
							Type:    "NamespaceDeletionContentFailure",
							Status:  "True",
							Message: "also the wrong msg",
						},
						{
							Type:    "NamespaceDeletionDiscoveryFailure",
							Status:  "True",
							Message: "right msg",
						},
					},
				},
			},
			expected: model.Namespace{
				Metadata: &model.Metadata{
					Annotations:       []string{"annotation:my-annotation"},
					CreationTimestamp: creationTime.Unix(),
					Labels:            []string{"app:my-app"},
					Name:              "my-name",
					Namespace:         "my-namespace",
					ResourceVersion:   "1234",
					Finalizers:        []string{"final", "izers"},
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
				},
				Status:           "a-phase",
				ConditionMessage: "right msg",
			},
		},
		"nil-safety": {
			input: corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       corev1.NamespaceSpec{},
				Status:     corev1.NamespaceStatus{},
			},
			expected: model.Namespace{
				Metadata: &model.Metadata{},
				Status:   "",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractNamespace(&tc.input))
		})
	}
}
