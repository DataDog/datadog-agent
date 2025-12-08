// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	admissioncommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func TestNewWebhookConfig(t *testing.T) {
	tests := map[string]struct {
		config   map[string]any
		expected *autoinstrumentation.WebhookConfig
	}{
		"defaults load as expected": {
			expected: &autoinstrumentation.WebhookConfig{
				IsEnabled: true,
				Endpoint:  "/injectlib",
			},
		},
		"disabled configuration is disabled": {
			config: map[string]any{
				"admission_controller.auto_instrumentation.enabled": false,
			},
			expected: &autoinstrumentation.WebhookConfig{
				IsEnabled: false,
				Endpoint:  "/injectlib",
			},
		},
		"updated endpoint is updated": {
			config: map[string]any{
				"admission_controller.auto_instrumentation.endpoint": "/foo",
			},
			expected: &autoinstrumentation.WebhookConfig{
				IsEnabled: true,
				Endpoint:  "/foo",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, test.config)
			actual := autoinstrumentation.NewWebhookConfig(mockConfig)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestWebhookIsEnabled(t *testing.T) {
	tests := map[string]struct {
		config   *autoinstrumentation.WebhookConfig
		expected bool
	}{
		"enabled configuration is enabled": {
			config: &autoinstrumentation.WebhookConfig{
				IsEnabled: true,
			},
			expected: true,
		},
		"disabled configuration is disabled": {
			config: &autoinstrumentation.WebhookConfig{
				IsEnabled: false,
			},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockMeta := common.FakeStore(t)
			mockMutator := common.FakeMutator(t, false)
			mockLabelSelectors := NewFakeLabelSelector()

			webhook, err := autoinstrumentation.NewWebhook(test.config, mockMeta, mockMutator, mockLabelSelectors)
			require.NoError(t, err)

			require.Equal(t, test.expected, webhook.IsEnabled())
		})
	}
}

func TestWebhookEndpoint(t *testing.T) {
	tests := map[string]struct {
		config   *autoinstrumentation.WebhookConfig
		expected string
	}{
		"configuration sets endpoint": {
			config: &autoinstrumentation.WebhookConfig{
				Endpoint: "/foo",
			},
			expected: "/foo",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockMeta := common.FakeStore(t)
			mockMutator := common.FakeMutator(t, false)
			mockLabelSelectors := NewFakeLabelSelector()

			webhook, err := autoinstrumentation.NewWebhook(test.config, mockMeta, mockMutator, mockLabelSelectors)
			require.NoError(t, err)

			require.Equal(t, test.expected, webhook.Endpoint())
		})
	}
}

func TestWebhookLabelSelectors(t *testing.T) {
	tests := map[string]struct {
		config                    map[string]any
		useNamespaceSelector      bool
		expectedSelector          *metav1.LabelSelector
		expectedNamespaceSelector *metav1.LabelSelector
	}{
		"default config with namespace selector enabled only uses namespace selector": {
			useNamespaceSelector: true,
			expectedSelector:     nil,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      admissioncommon.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   common.DefaultDisabledNamespaces(),
					},
				},
			},
		},
		"default config with namespace selector disabled uses both selectors": {
			useNamespaceSelector: false,
			expectedSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					admissioncommon.EnabledLabelKey: "true",
				},
			},
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      admissioncommon.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   common.DefaultDisabledNamespaces(),
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, test.config)
			mockMeta := common.FakeStore(t)
			mockMutator := common.FakeMutator(t, false)

			labelSelectors := autoinstrumentation.NewLabelSelectors(autoinstrumentation.NewLabelSelectorsConfig(mockConfig))
			config := autoinstrumentation.NewWebhookConfig(mockConfig)
			webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, labelSelectors)
			require.NoError(t, err)

			namespaceSelector, selector := webhook.LabelSelectors(test.useNamespaceSelector)
			require.Equal(t, test.expectedSelector, selector, "object selector does not match")
			require.Equal(t, test.expectedNamespaceSelector, namespaceSelector, "namespace selector does not match")
		})
	}
}

func TestWebhookResources(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookResources, webhook.Resources())
}

func TestWebhookTimeout(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, int32(0), webhook.Timeout())
}

func TestWebhookOperations(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookOperations, webhook.Operations())
}

func TestWebhookMatchConditions(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookMatchConditions, webhook.MatchConditions())
}

func TestWebhookName(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookName, webhook.Name())
}

func TestWebhookType(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, admissioncommon.MutatingWebhook, webhook.WebhookType().String())
}

func TestWebhookFunc(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMeta := common.FakeStore(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMeta, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	pod := common.FakePod("foo")
	b, err := json.Marshal(pod)
	require.NoError(t, err)

	resp := webhook.WebhookFunc()(&admission.Request{
		Object:    b,
		Namespace: "foo",
	})
	require.NotNil(t, resp)

	require.Equal(t, true, mockMutator.Called)
}
