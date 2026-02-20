// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var (
	defaultLibraries = map[string]string{
		"java":   "v1",
		"python": "v4",
		"ruby":   "v2",
		"dotnet": "v3",
		"js":     "v5",
		"php":    "v1",
	}

	// TODO: Add new entry when a new language is supported
	defaultLibImageVersions = map[language]string{
		java:   "registry/dd-lib-java-init:" + defaultLibraries["java"],
		js:     "registry/dd-lib-js-init:" + defaultLibraries["js"],
		python: "registry/dd-lib-python-init:" + defaultLibraries["python"],
		dotnet: "registry/dd-lib-dotnet-init:" + defaultLibraries["dotnet"],
		ruby:   "registry/dd-lib-ruby-init:" + defaultLibraries["ruby"],
		php:    "registry/dd-lib-php-init:" + defaultLibraries["php"],
	}

	imageResolver = imageresolver.NewNoOpResolver()
)

func TestNewTargetMutator(t *testing.T) {
	tests := map[string]struct {
		configPath string
		shouldErr  bool
	}{
		"valid config": {
			configPath: "testdata/filter.yaml",
			shouldErr:  false,
		},
		"invalid config": {
			configPath: "testdata/filter_invalid_configs.yaml",
			shouldErr:  true,
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
				fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Create the mutator.
			_, err = NewTargetMutator(config, wmeta, imageResolver)

			// Validate the output.
			if test.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMutatePod(t *testing.T) {
	tests := map[string]struct {
		configPath                  string
		in                          *corev1.Pod
		namespaces                  []workloadmeta.KubernetesMetadata
		expectedEnv                 map[string]string
		expectedAnnotations         map[string]string
		expectedInitContainerImages []string
		expectNoChange              bool
	}{
		"a matching rule has single step enabled": {
			configPath: "testdata/filter_simple_namespace.yaml",
			in:         mutatecommon.FakePodWithNamespace("foo-service", "application"),
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expectedInitContainerImages: []string{
				"registry/apm-inject:0",
				"registry/dd-lib-python-init:v3",
			},
			expectedEnv: map[string]string{
				"DD_INJECT_SENDER_TYPE":           "k8s",
				"DD_INSTRUMENTATION_INSTALL_ID":   "",
				"DD_INSTRUMENTATION_INSTALL_TYPE": "k8s_single_step",
				"DD_LOGS_INJECTION":               "true",
				"DD_RUNTIME_METRICS_ENABLED":      "true",
				"DD_TRACE_ENABLED":                "true",
				"DD_TRACE_HEALTH_METRICS_ENABLED": "true",
				"LD_PRELOAD":                      "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
				AppliedTargetEnvVar:               "{\"name\":\"Application Namespace\",\"namespaceSelector\":{\"matchNames\":[\"application\"]},\"ddTraceVersions\":{\"python\":\"v3\"},\"ddTraceConfigs\":[{\"name\":\"DD_PROFILING_ENABLED\",\"value\":\"true\"},{\"name\":\"DD_DATA_JOBS_ENABLED\",\"value\":\"true\"}]}",
			},
			expectedAnnotations: map[string]string{
				annotation.AppliedTarget: "{\"name\":\"Application Namespace\",\"namespaceSelector\":{\"matchNames\":[\"application\"]},\"ddTraceVersions\":{\"python\":\"v3\"},\"ddTraceConfigs\":[{\"name\":\"DD_PROFILING_ENABLED\",\"value\":\"true\"},{\"name\":\"DD_DATA_JOBS_ENABLED\",\"value\":\"true\"}]}",
			},
		},
		"no matching rule does not mutate pod": {
			configPath: "testdata/filter_simple_namespace.yaml",
			in:         mutatecommon.FakePodWithNamespace("foo-service", "foo"),
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", nil),
			},
			expectNoChange: true,
		},
<<<<<<< HEAD
		// Re-admission guard: when the webhook runs again on an already-injected pod we must not
		// mutate further (e.g. must not append to LD_PRELOAD or add duplicate init containers).
=======
>>>>>>> 6b4e53246ba (Add tests for re-admission in init-container and img-vol mode)
		"re-admission with init_container mode init container already present does not mutate": {
			configPath: "testdata/filter_simple_namespace.yaml",
			in: mutatecommon.FakePodSpec{
				NS: "application",
				InitContainers: []corev1.Container{
					{Name: "datadog-lib-python-init", Image: "registry/dd-lib-python-init:v3"},
				},
			}.Create(),
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expectNoChange: true,
		},
		"re-admission with image_volume mode init container already present does not mutate": {
			configPath: "testdata/filter_simple_namespace.yaml",
			in: mutatecommon.FakePodSpec{
				NS: "application",
				InitContainers: []corev1.Container{
					{Name: libraryinjection.InjectLDPreloadInitContainerName, Image: "registry/apm-inject:0"},
				},
			}.Create(),
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expectNoChange: true,
		},
		"tracer configs get applied": {
			configPath: "testdata/filter_simple_configs.yaml",
			in: mutatecommon.WithLabels(
				mutatecommon.FakePodWithNamespace("foo-service", "application"),
				map[string]string{"language": "python"},
			),
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expectedInitContainerImages: []string{
				"registry/apm-inject:0",
				"registry/dd-lib-python-init:v3",
			},
			expectedEnv: map[string]string{
				"DD_PROFILING_ENABLED":            "true",
				"DD_DATA_JOBS_ENABLED":            "true",
				"DD_INJECT_SENDER_TYPE":           "k8s",
				"DD_INSTRUMENTATION_INSTALL_ID":   "",
				"DD_INSTRUMENTATION_INSTALL_TYPE": "k8s_single_step",
				"DD_LOGS_INJECTION":               "true",
				"DD_RUNTIME_METRICS_ENABLED":      "true",
				"DD_TRACE_ENABLED":                "true",
				"DD_TRACE_HEALTH_METRICS_ENABLED": "true",
				"LD_PRELOAD":                      "/opt/datadog-packages/datadog-apm-inject/stable/inject/launcher.preload.so",
				AppliedTargetEnvVar:               "{\"name\":\"Python Apps\",\"podSelector\":{\"matchLabels\":{\"language\":\"python\"}},\"ddTraceVersions\":{\"python\":\"v3\"},\"ddTraceConfigs\":[{\"name\":\"DD_PROFILING_ENABLED\",\"value\":\"true\"},{\"name\":\"DD_DATA_JOBS_ENABLED\",\"value\":\"true\"}]}",
			},
			expectedAnnotations: map[string]string{
				annotation.AppliedTarget: "{\"name\":\"Python Apps\",\"podSelector\":{\"matchLabels\":{\"language\":\"python\"}},\"ddTraceVersions\":{\"python\":\"v3\"},\"ddTraceConfigs\":[{\"name\":\"DD_PROFILING_ENABLED\",\"value\":\"true\"},{\"name\":\"DD_DATA_JOBS_ENABLED\",\"value\":\"true\"}]}",
			},
		},
		"service name is applied when set in tracer configs": {
			configPath: "testdata/filter_simple_service.yaml",
			in: mutatecommon.FakePodSpec{
				Labels:     map[string]string{"language": "python"},
				NS:         "application",
				ParentKind: "replicaset",
				ParentName: "deployment-1234",
			}.Create(),
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expectedInitContainerImages: []string{
				"registry/apm-inject:0",
				"registry/dd-lib-python-init:v3",
			},
			expectedEnv: map[string]string{
				"DD_SERVICE": "best-service",
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
				fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Add the namespaces.
			for _, ns := range test.namespaces {
				wmeta.Set(&ns)
			}

			// Create the mutator.
			f, err := NewTargetMutator(config, wmeta, imageresolver.NewNoOpResolver())
			require.NoError(t, err)

			input := test.in.DeepCopy()

			// Mutate the pod.
			mutated, err := f.MutatePod(test.in, test.in.Namespace, nil)

			// If there is no change, validate that the pod is unchanged.
			if test.expectNoChange {
				require.False(t, mutated)
				require.NoError(t, err)
				require.Equal(t, input, test.in)
				return
			}

			require.True(t, mutated)
			require.NoError(t, err)
			require.Equal(t, 1, len(test.in.Spec.Containers))

			// Validate the desired env.
			actualEnv := make(map[string]string, len(test.in.Spec.Containers[0].Env))
			for _, env := range test.in.Spec.Containers[0].Env {
				actualEnv[env.Name] = env.Value
			}
			for k, v := range test.expectedEnv {
				require.Equal(t, v, actualEnv[k])
			}

			// Validate the init containers.
			actualInitContainerImages := make([]string, len(test.in.Spec.InitContainers))
			for i, ctr := range test.in.Spec.InitContainers {
				actualInitContainerImages[i] = ctr.Image
			}
			require.ElementsMatch(t, test.expectedInitContainerImages, actualInitContainerImages)

			// Validate the annotations.
			for k, v := range test.expectedAnnotations {
				require.Equal(t, v, test.in.Annotations[k])
			}
		})
	}
}

func TestShouldMutatePod(t *testing.T) {
	tests := map[string]struct {
		configPath string
		in         *corev1.Pod
		expected   bool
		namespaces []workloadmeta.KubernetesMetadata
	}{
		"a matching rule gets mutated": {
			configPath: "testdata/filter_no_default.yaml",
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
			expected: true,
		},
		"no matching rule is not mutated": {
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
			expected: false,
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
				fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Add the namespaces.
			for _, ns := range test.namespaces {
				wmeta.Set(&ns)
			}

			// Create the mutator.
			f, err := NewTargetMutator(config, wmeta, imageresolver.NewNoOpResolver())
			require.NoError(t, err)

			// Determine if the pod should be mutated.
			actual := f.ShouldMutatePod(test.in)

			// Validate the output.
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestIsNamespaceEligible(t *testing.T) {
	tests := map[string]struct {
		configPath string
		in         string
		expected   bool
		namespaces []workloadmeta.KubernetesMetadata
	}{
		"a matchNames namespace is eligible": {
			configPath: "testdata/filter_no_default.yaml",
			in:         "billing-service",
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("billing-service", nil),
			},
			expected: true,
		},
		"a rule without a namespace selector is eligible": {
			configPath: "testdata/filter_no_default.yaml",
			in:         "foo",
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", nil),
			},
			expected: true,
		},
		"a matchLabels namespace is eligible": {
			configPath: "testdata/filter_no_default.yaml",
			in:         "foo",
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", map[string]string{
					"tracing": "yes",
					"env":     "prod",
				}),
			},
			expected: true,
		},
		"a disabled namespace is not eligible": {
			configPath: "testdata/filter_no_default.yaml",
			in:         "infra",
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("infra", nil),
			},
			expected: false,
		},
		"a common disabled namespace is not eligible": {
			configPath: "testdata/filter_no_default.yaml",
			in:         "kube-system",
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("kube-system", nil),
			},
			expected: false,
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
				fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Add the namespaces.
			for _, ns := range test.namespaces {
				wmeta.Set(&ns)
			}

			// Create the mutator.
			f, err := NewTargetMutator(config, wmeta, imageresolver.NewNoOpResolver())
			require.NoError(t, err)

			// Determine if the namespace is eligible.
			actual := f.IsNamespaceEligible(test.in)

			// Validate the output.
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestGetTargetFromAnnotation(t *testing.T) {
	tests := map[string]struct {
		configPath string
		in         *corev1.Pod
		expected   *targetInternal
	}{
		"a pod with no annotations gets no values": {
			configPath: "testdata/filter_limited.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
				},
			},
			expected: nil,
		},
		"a pod with an annotation gets a value": {
			configPath: "testdata/filter_limited.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						common.EnabledLabelKey: "true",
					},
					Annotations: map[string]string{
						"admission.datadoghq.com/python-lib.version": "v3",
					},
				},
			},
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(python, "v3"),
				},
			},
		},
		"a pod with an annotation but disabled label gets no value": {
			configPath: "testdata/filter_limited.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						common.EnabledLabelKey: "false",
					},
					Annotations: map[string]string{
						"admission.datadoghq.com/python-lib.version": "v3",
					},
				},
			},
			expected: nil,
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
				fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Create the mutator.
			f, err := NewTargetMutator(config, wmeta, imageresolver.NewNoOpResolver())
			require.NoError(t, err)

			// Get the target from the annotation.
			actual := f.getTargetFromAnnotation(test.in)

			// Validate the output.
			if test.expected == nil {
				require.Nil(t, actual.target)
			} else {
				require.NotNil(t, actual)
				require.Equal(t, test.expected.libVersions, actual.target.libVersions)
			}
		})
	}
}

func TestGetTargetLibraries(t *testing.T) {
	imageResolver := imageresolver.NewNoOpResolver()

	tests := map[string]struct {
		configPath string
		in         *corev1.Pod
		namespaces []workloadmeta.KubernetesMetadata
		expected   *targetInternal
	}{
		"a rule without selectors applies as a default": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						"app": "frontend",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", nil),
			},
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(js, "v5"),
				},
			},
		},
		"a pod that matches no targets gets no values": {
			configPath: "testdata/filter_no_default.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						"app": "frontend",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("foo", nil),
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
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(python, "v3"),
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
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(java, "v1"),
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
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(dotnet, "v1"),
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
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(java, "v1"),
					defaultLibInfoWithVersion(js, "v5"),
					defaultLibInfoWithVersion(python, "v4"),
					defaultLibInfoWithVersion(dotnet, "v3"),
					defaultLibInfoWithVersion(ruby, "v2"),
					defaultLibInfoWithVersion(php, "v1"),
				},
			},
		},
		"a default disabled namespace gets no tracers": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					Labels: map[string]string{
						"language": "java",
					},
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("kube-system", nil),
			},
			expected: nil,
		},
		"enabled namespace gets converted to target": {
			configPath: "testdata/enabled_namespaces.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "application",
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(python, "v3"),
				},
			},
		},
		"no targets with instrumentation enabled injects all SDKs": {
			configPath: "testdata/instrumentation_enabled.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "application",
				},
			},
			namespaces: []workloadmeta.KubernetesMetadata{
				newTestNamespace("application", nil),
			},
			expected: &targetInternal{
				libVersions: []libInfo{
					defaultLibInfoWithVersion(java, "v1"),
					defaultLibInfoWithVersion(js, "v5"),
					defaultLibInfoWithVersion(python, "v4"),
					defaultLibInfoWithVersion(dotnet, "v3"),
					defaultLibInfoWithVersion(ruby, "v2"),
					defaultLibInfoWithVersion(php, "v1"),
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
				fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			// Add the namespaces.
			for _, ns := range test.namespaces {
				wmeta.Set(&ns)
			}

			// Create the mutator.
			f, err := NewTargetMutator(config, wmeta, imageResolver)
			require.NoError(t, err)

			// Filter the pod.
			actual := f.getMatchingTarget(test.in)

			// Validate the output.
			if test.expected == nil {
				require.Nil(t, actual)
			} else {
				require.NotNil(t, actual)
				require.Equal(t, test.expected.libVersions, actual.libVersions)
			}
		})
	}
}

func TestLanguageDetection(t *testing.T) {
	tests := map[string]struct {
		config                     map[string]any
		pod                        *corev1.Pod
		deployments                []mutatecommon.MockDeployment
		expectedInitContainerNames []string
	}{
		"default target uses language detection when enabled": {
			config: map[string]interface{}{
				"apm_config.instrumentation.enabled":                                       true,
				"language_detection.enabled":                                               true,
				"language_detection.reporting.enabled":                                     true,
				"admission_controller.auto_instrumentation.inject_auto_detected_libraries": true,
			},
			pod: mutatecommon.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []mutatecommon.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "deployment",
					Namespace:      "ns",
					Languages:      languageSetOf("python"),
				},
			},
			expectedInitContainerNames: []string{
				"datadog-init-apm-inject",
				"datadog-lib-python-init",
			},
		},
		"user set default libraries uses language detection when enabled": {
			config: map[string]interface{}{
				"apm_config.instrumentation.enabled":                                       true,
				"language_detection.enabled":                                               true,
				"language_detection.reporting.enabled":                                     true,
				"admission_controller.auto_instrumentation.inject_auto_detected_libraries": true,
				"apm_config.instrumentation.lib_versions":                                  defaultLibraries,
			},
			pod: mutatecommon.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []mutatecommon.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "deployment",
					Namespace:      "ns",
					Languages:      languageSetOf("python"),
				},
			},
			expectedInitContainerNames: []string{
				"datadog-init-apm-inject",
				"datadog-lib-python-init",
			},
		},
		"default target does not use language detection when disabled": {
			config: map[string]interface{}{
				"apm_config.instrumentation.enabled":                                       true,
				"language_detection.enabled":                                               false,
				"language_detection.reporting.enabled":                                     false,
				"admission_controller.auto_instrumentation.inject_auto_detected_libraries": false,
			},
			pod: mutatecommon.FakePodSpec{
				ParentKind: "replicaset",
				ParentName: "deployment-123",
			}.Create(),
			deployments: []mutatecommon.MockDeployment{
				{
					ContainerName:  "pod",
					DeploymentName: "deployment",
					Namespace:      "ns",
					Languages:      languageSetOf("python"),
				},
			},
			expectedInitContainerNames: []string{
				"datadog-init-apm-inject",
				"datadog-lib-python-init",
				"datadog-lib-java-init",
				"datadog-lib-js-init",
				"datadog-lib-dotnet-init",
				"datadog-lib-ruby-init",
				"datadog-lib-php-init",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Load the config.
			mockConfig := configmock.New(t)
			for k, v := range test.config {
				mockConfig.SetWithoutSource(k, v)
			}
			config, err := NewConfig(mockConfig)
			require.NoError(t, err)

			// Create a mock meta.
			wmeta := mutatecommon.FakeStoreWithDeployment(t, test.deployments)

			// Create the mutator.
			m, err := NewTargetMutator(config, wmeta, imageResolver)
			require.NoError(t, err)

			// Mutate the pod.
			mutated, err := m.MutatePod(test.pod, test.pod.Namespace, nil)
			require.NoError(t, err)
			require.True(t, mutated)

			// Ensure the init containers match.
			actualInitContainerNames := []string{}
			for _, container := range test.pod.Spec.InitContainers {
				actualInitContainerNames = append(actualInitContainerNames, container.Name)
			}
			require.ElementsMatch(t, test.expectedInitContainerNames, actualInitContainerNames)
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

func languageSetOf(languages ...string) languagemodels.LanguageSet {
	set := languagemodels.LanguageSet{}
	for _, l := range languages {
		_ = set.Add(languagemodels.LanguageName(l))
	}
	return set
}

func defaultLibInfo(l language) libInfo {
	return libInfo{
		lang:       l,
		image:      defaultLibImageVersions[l],
		registry:   "registry",
		repository: fmt.Sprintf("dd-lib-%s-init", l),
		tag:        defaultLibraries[string(l)],
		ctrName:    "",
	}
}

func defaultLibInfoWithVersion(l language, version string) libInfo {
	return libInfo{
		lang:       l,
		image:      fmt.Sprintf("registry/dd-lib-%s-init:%s", l, version),
		registry:   "registry",
		repository: fmt.Sprintf("dd-lib-%s-init", l),
		tag:        version,
		ctrName:    "",
	}
}

func defaultLibrariesFor(languages ...string) map[string]string {
	out := map[string]string{}
	for _, l := range languages {
		out[l] = defaultLibraries[l]
	}
	return out
}
