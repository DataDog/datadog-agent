// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestEnsureAKSSelectors(t *testing.T) {
	tests := []struct {
		name           string
		addAKSSelector bool
		in             *metav1.LabelSelector
		want           *metav1.LabelSelector
	}{
		{
			name:           "flag disabled leaves nil selector untouched",
			addAKSSelector: false,
			in:             nil,
			want:           nil,
		},
		{
			name:           "flag disabled leaves existing selector untouched",
			addAKSSelector: false,
			in:             &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			want:           &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
		},
		{
			name:           "flag enabled creates a selector when nil (CONS-8533)",
			addAKSSelector: true,
			in:             nil,
			want:           &metav1.LabelSelector{MatchExpressions: AzureAKSLabelSelectorRequirement()},
		},
		{
			name:           "flag enabled appends to an existing selector",
			addAKSSelector: true,
			in:             &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			want: &metav1.LabelSelector{
				MatchLabels:      map[string]string{"foo": "bar"},
				MatchExpressions: AzureAKSLabelSelectorRequirement(),
			},
		},
		{
			name:           "flag enabled does not duplicate requirements already present",
			addAKSSelector: true,
			in:             &metav1.LabelSelector{MatchExpressions: AzureAKSLabelSelectorRequirement()},
			want:           &metav1.LabelSelector{MatchExpressions: AzureAKSLabelSelectorRequirement()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("admission_controller.add_aks_selectors", tt.addAKSSelector)

			got := EnsureAKSSelectors(tt.in)

			assert.Equal(t, tt.want, got)
		})
	}
}
