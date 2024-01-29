// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package webhook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergedLabelSelector(t *testing.T) {
	tests := []struct {
		name                  string
		labelSelector1        *metav1.LabelSelector
		labelSelector2        *metav1.LabelSelector
		expectedLabelSelector *metav1.LabelSelector
	}{
		{
			name: "non-nil label selectors",
			labelSelector1: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label1": "val1",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label2",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"val2"},
					},
				},
			},
			labelSelector2: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label3": "val3",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label4",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"val4"},
					},
				},
			},
			expectedLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label1": "val1",
					"label3": "val3",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label2",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"val2"},
					},
					{
						Key:      "label4",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"val4"},
					},
				},
			},
		},
		{
			name:                  "both label selectors are nil",
			labelSelector1:        nil,
			labelSelector2:        nil,
			expectedLabelSelector: nil,
		},
		{
			name:           "first label selector is nil",
			labelSelector1: nil,
			labelSelector2: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			expectedLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
		},
		{
			name: "second label selector is nil",
			labelSelector1: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			labelSelector2: nil,
			expectedLabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedLabelSelector, mergedLabelSelector(test.labelSelector1, test.labelSelector2))
		})
	}
}
