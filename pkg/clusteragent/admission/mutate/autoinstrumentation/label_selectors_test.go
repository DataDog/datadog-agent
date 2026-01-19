// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func NewFakeLabelSelector() *autoinstrumentation.LabelSelectors {
	return autoinstrumentation.NewLabelSelectors(&autoinstrumentation.LabelSelectorsConfig{
		Enabled:            false,
		MutateUnlabelled:   false,
		AddAksSelectors:    false,
		DisabledNamespaces: []string{},
	})
}

func TestLabelSelectorsConfig(t *testing.T) {
	tests := map[string]struct {
		config   map[string]any
		expected *autoinstrumentation.LabelSelectorsConfig
	}{
		"default values match expected": {
			expected: &autoinstrumentation.LabelSelectorsConfig{
				Enabled:            false,
				MutateUnlabelled:   false,
				AddAksSelectors:    false,
				DisabledNamespaces: []string{},
			},
		},
		"overridden values match expected": {
			config: map[string]any{
				"apm_config.instrumentation.enabled":             true,
				"admission_controller.mutate_unlabelled":         true,
				"admission_controller.add_aks_selectors":         true,
				"apm_config.instrumentation.disabled_namespaces": []string{"foo"},
			},
			expected: &autoinstrumentation.LabelSelectorsConfig{
				Enabled:            true,
				MutateUnlabelled:   true,
				AddAksSelectors:    true,
				DisabledNamespaces: []string{"foo"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := mutatecommon.FakeConfigWithValues(t, test.config)
			actual := autoinstrumentation.NewLabelSelectorsConfig(mockConfig)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestLabelSelectors(t *testing.T) {
	tests := map[string]struct {
		config                    *autoinstrumentation.LabelSelectorsConfig
		useNamespaceSelector      bool
		expectedNamespaceSelector *metav1.LabelSelector
		expectedObjectSelector    *metav1.LabelSelector
	}{
		"by default, everything should be allowed except for disabled namespaces and disable labels": {
			config: &autoinstrumentation.LabelSelectorsConfig{
				Enabled: true,
			},
			useNamespaceSelector: false,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   mutatecommon.DefaultDisabledNamespaces(),
					},
				},
			},
			expectedObjectSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
				},
			},
		},
		"by default, if we are using the namespace selector only, then both expressions should be in the namespace selector": {
			config: &autoinstrumentation.LabelSelectorsConfig{
				Enabled: true,
			},
			useNamespaceSelector: true,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
					{
						Key:      common.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   mutatecommon.DefaultDisabledNamespaces(),
					},
				},
			},
			expectedObjectSelector: nil,
		},
		"when instrumentation is disabled and mutate unlabelled is also disabled, only enable labels should be selected": {
			config: &autoinstrumentation.LabelSelectorsConfig{
				Enabled:          false,
				MutateUnlabelled: false,
			},
			useNamespaceSelector: false,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   mutatecommon.DefaultDisabledNamespaces(),
					},
				},
			},
			expectedObjectSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					common.EnabledLabelKey: "true",
				},
			},
		},
		"when instrumentation is disabled but mutate unlabelled is enabled, only enable labels should be selected": {
			config: &autoinstrumentation.LabelSelectorsConfig{
				Enabled:          false,
				MutateUnlabelled: true,
			},
			useNamespaceSelector: false,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   mutatecommon.DefaultDisabledNamespaces(),
					},
				},
			},
			expectedObjectSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
				},
			},
		},
		"disabled namespaces should be appended to the default disabled namepsaces": {
			config: &autoinstrumentation.LabelSelectorsConfig{
				Enabled:            true,
				DisabledNamespaces: []string{"foo"},
			},
			useNamespaceSelector: false,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   append(mutatecommon.DefaultDisabledNamespaces(), "foo"),
					},
				},
			},
			expectedObjectSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
				},
			},
		},
		"when add aks selectors is true, the additional selectors should be added": {
			config: &autoinstrumentation.LabelSelectorsConfig{
				Enabled:         true,
				AddAksSelectors: true,
			},
			useNamespaceSelector: false,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: append([]metav1.LabelSelectorRequirement{
					{
						Key:      common.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   mutatecommon.DefaultDisabledNamespaces(),
					},
				}, common.AzureAKSLabelSelectorRequirement()...),
			},
			expectedObjectSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      common.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			labelSelectors := autoinstrumentation.NewLabelSelectors(test.config)
			actualNamesapaceSelector, actualObjectSelector := labelSelectors.Get(test.useNamespaceSelector)
			require.Equal(t, test.expectedNamespaceSelector, actualNamesapaceSelector, "namespace selector does not match expected")
			require.Equal(t, test.expectedObjectSelector, actualObjectSelector, "object selector does not match expected")
		})
	}
}
