// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestTargetFilter(t *testing.T) {
	tests := map[string]struct {
		configPath string
		in         *corev1.Pod
		namespaces []workloadmeta.KubernetesMetadata
		expected   []libInfo
	}{
		"a rule without selectors applies as a default": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels: map[string]string{
						"app": "frontend",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("default", nil),
			},
			expected: []libInfo{
				{
					ctrName: "",
					lang:    js,
					image:   "registry/dd-lib-js-init:v5",
				},
			},
		},
		"a pod that matches no targets gets no values": {
			configPath: "testdata/filter_no_default.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels: map[string]string{
						"app": "frontend",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("default", nil),
			},
			expected: nil,
		},
		"a single service example matches rule": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "billing-service",
					Labels: map[string]string{
						"app": "billing-service",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("billing-service", nil),
			},
			expected: []libInfo{
				{
					ctrName: "",
					lang:    python,
					image:   "registry/dd-lib-python-init:v2",
				},
			},
		},
		"a java microservice service matches rule": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "application",
					Labels: map[string]string{
						"language": "java",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expected: []libInfo{
				{
					ctrName: "",
					lang:    java,
					image:   "registry/dd-lib-java-init:v1",
				},
			},
		},
		"a disabled namespace gets no tracers": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "infra",
					Labels: map[string]string{
						"language": "java",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("infra", nil),
			},
			expected: nil,
		},
		"namespace labels are used to match namespaces": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels:    map[string]string{},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", map[string]string{
					"tracing": "yes",
					"env":     "prod",
				}),
			},
			expected: []libInfo{
				{
					ctrName: "",
					lang:    dotnet,
					image:   "registry/dd-lib-dotnet-init:v1",
				},
			},
		},
		"misconfigured namespace labels gets no tracers": {
			configPath: "testdata/filter_no_default.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Labels:    map[string]string{},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", map[string]string{
					"tracing": "yes",
					"env":     "prod",
				}),
			},
			expected: nil,
		},
		"missing namespace in store gets no tracers": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels:    map[string]string{},
				},
			},
			expected: nil,
		},
		"unset tracer versions applies all tracers": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "application",
					Labels: map[string]string{
						"language": "unknown",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expected: []libInfo{
				{
					ctrName: "",
					lang:    java,
					image:   "registry/dd-lib-java-init:v1",
				},
				{
					ctrName: "",
					lang:    js,
					image:   "registry/dd-lib-js-init:v5",
				},
				{
					ctrName: "",
					lang:    python,
					image:   "registry/dd-lib-python-init:v2",
				},
				{
					ctrName: "",
					lang:    dotnet,
					image:   "registry/dd-lib-dotnet-init:v3",
				},
				{
					ctrName: "",
					lang:    ruby,
					image:   "registry/dd-lib-ruby-init:v2",
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Load the config.
			mockConfig := configmock.NewFromFile(t, test.configPath)
			mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.container_registry", "registry")
			config, err := NewConfig(mockConfig)
			require.NoError(t, err)

			// Create a mock meta.
			wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Supply(coreconfig.Params{}),
				fx.Provide(func() log.Component { return logmock.New(t) }),
				coreconfig.MockModule(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Add the namespaces.
			for _, ns := range test.namespaces {
				wmeta.Set(&ns)
			}

			// Create the filter.
			f, err := NewTargetFilter(config, wmeta)
			require.NoError(t, err)

			// Filter the pod.
			actual := f.filter(test.in)

			// Validate the output.
			require.Equal(t, test.expected, actual)
		})
	}
}

func newTestNamespace(name string, labels map[string]string) workloadmeta.KubernetesMetadata {
	return workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "namespaces", "", name)),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   name,
			Labels: labels,
		},
	}
}
