// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation_test

import (
	"encoding/json"
	"fmt"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	admissioncommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
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
		"CRD instrumentation does not enable the webhook": {
			config: map[string]any{
				"admission_controller.auto_instrumentation.enabled": false,
				"instrumentation_crd_controller.enabled":            true,
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
			mockMutator := common.FakeMutator(t, false)
			mockLabelSelectors := NewFakeLabelSelector()

			webhook, err := autoinstrumentation.NewWebhook(test.config, mockMutator, mockLabelSelectors)
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
			mockMutator := common.FakeMutator(t, false)
			mockLabelSelectors := NewFakeLabelSelector()

			webhook, err := autoinstrumentation.NewWebhook(test.config, mockMutator, mockLabelSelectors)
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
		"default on-demand config with namespace selector enabled only uses namespace selector": {
			useNamespaceSelector: true,
			expectedSelector:     nil,
			expectedNamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      admissioncommon.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
					{
						Key:      admissioncommon.NamespaceLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   common.DefaultDisabledNamespaces(),
					},
				},
			},
		},
		"default on-demand config with namespace selector disabled uses both selectors": {
			useNamespaceSelector: false,
			expectedSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      admissioncommon.EnabledLabelKey,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"false"},
					},
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
			mockMutator := common.FakeMutator(t, false)

			labelSelectors := autoinstrumentation.NewLabelSelectors(autoinstrumentation.NewLabelSelectorsConfig(mockConfig))
			config := autoinstrumentation.NewWebhookConfig(mockConfig)
			webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, labelSelectors)
			require.NoError(t, err)

			namespaceSelector, selector := webhook.LabelSelectors(test.useNamespaceSelector)
			require.Equal(t, test.expectedSelector, selector, "object selector does not match")
			require.Equal(t, test.expectedNamespaceSelector, namespaceSelector, "namespace selector does not match")
		})
	}
}

func TestWebhookResources(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookResources, webhook.Resources())
}

func TestWebhookTimeout(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, int32(0), webhook.Timeout())
}

func TestWebhookOperations(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookOperations, webhook.Operations())
}

func TestWebhookMatchConditions(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookMatchConditions, webhook.MatchConditions())
}

func TestWebhookName(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, autoinstrumentation.WebhookName, webhook.Name())
}

func TestWebhookType(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
	require.NoError(t, err)

	require.Equal(t, admissioncommon.MutatingWebhook, webhook.WebhookType().String())
}

func TestWebhookFunc(t *testing.T) {
	mockConfig := common.FakeConfig(t)
	mockMutator := common.FakeMutator(t, false)
	mockLabelSelectors := NewFakeLabelSelector()

	config := autoinstrumentation.NewWebhookConfig(mockConfig)
	webhook, err := autoinstrumentation.NewWebhook(config, mockMutator, mockLabelSelectors)
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

func TestWebhookPatchPreservesUnknownFields(t *testing.T) {
	for _, tt := range []struct {
		name            string
		containerFields string
	}{
		{
			name:            "unknown field alongside known fields",
			containerFields: `,"securityContext":{"runAsNonRoot":true,"capabilities":{"add":["NET_ADMIN"],"drop":["ALL"],"customField":["custom-value"]}}`,
		},
		{
			name:            "unknown field only in nested object",
			containerFields: `,"securityContext":{"capabilities":{"customField":{"enabled":true,"values":[1,null,"custom-value"]}}}`,
		},
		{
			name: "optional parent objects absent",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, map[string]any{
				"apm_config.instrumentation.enabled":                                true,
				"apm_config.instrumentation.lib_versions":                           map[string]string{"python": "v3"},
				"admission_controller.auto_instrumentation.gradual_rollout.enabled": false,
			})
			webhook, err := autoinstrumentation.NewAutoInstrumentation(mockConfig, common.FakeStore(t), nil, nil, nil, nil)
			require.NoError(t, err)

			// Keep this request raw: marshaling a corev1.Pod would discard unknown fields before the test.
			// Annotations, resources, env, volume mounts, init containers, and volumes are absent.
			rawPod := []byte(fmt.Sprintf(`{
				"metadata":{"name":"test-pod","namespace":"application","labels":{"tags.datadoghq.com/env":"test"}},
				"spec":{"customField":{"enabled":true,"values":["custom-value",null,7]},"containers":[
					{"name":"app","image":"app:latest","customField":42%s},
					{"name":"worker","image":"worker:latest","customField":"worker-value","securityContext":{"capabilities":{"customField":["worker-capability"]}}}
				]}
			}`, tt.containerFields))
			response := webhook.WebhookFunc()(&admission.Request{Object: rawPod, Namespace: "application"})
			require.NotNil(t, response)
			require.True(t, response.Allowed)
			patch, err := jsonpatch.DecodePatch(response.Patch)
			require.NoError(t, err)
			patchedJSON, err := patch.Apply(rawPod)
			require.NoError(t, err, "patch must apply to the original raw request")

			var pod corev1.Pod
			require.NoError(t, json.Unmarshal(patchedJSON, &pod))
			validator := testutils.NewPodValidator(&pod, testutils.InjectionModeAuto)
			validator.RequireInjection(t, []string{"app", "worker"})
			validator.RequireLibraryVersions(t, map[string]string{"python": "v3"})
			validator.RequireEnvs(t, map[string]string{"DD_ENV": "test"}, []string{"app", "worker"})

			// Compare original fields by container name without decoding away unknown fields.
			var original, patched struct {
				Spec struct {
					CustomField json.RawMessage              `json:"customField"`
					Containers  []map[string]json.RawMessage `json:"containers"`
				} `json:"spec"`
			}
			require.NoError(t, json.Unmarshal(rawPod, &original))
			require.NoError(t, json.Unmarshal(patchedJSON, &patched))
			require.JSONEq(t, string(original.Spec.CustomField), string(patched.Spec.CustomField))
			containersByName := make(map[string]map[string]json.RawMessage)
			for _, container := range patched.Spec.Containers {
				var name string
				require.NoError(t, json.Unmarshal(container["name"], &name))
				containersByName[name] = container
			}
			for _, container := range original.Spec.Containers {
				var name string
				require.NoError(t, json.Unmarshal(container["name"], &name))
				actual, found := containersByName[name]
				require.True(t, found, "container %q must still exist", name)
				for field, expected := range container {
					require.JSONEq(t, string(expected), string(actual[field]), "container %q field %q", name, field)
				}
			}
		})
	}
}
