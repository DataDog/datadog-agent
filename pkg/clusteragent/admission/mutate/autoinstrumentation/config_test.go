// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// valueFromTracerConfigs is the expected result of decoding testdata/targets_env_value_from.yaml and
// valueFromTargetsJSON. The unquoted boolean value is decoded as "1".
var valueFromTracerConfigs = []TracerConfig{
	{Name: "DD_PROFILING_ENABLED", Value: "1"},
	{
		Name: "DD_SERVICE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.labels['app']"},
		},
	},
	{
		Name: "DD_API_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
				Key:                  "api-key",
			},
		},
	},
	{
		Name: "DD_ENV",
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-config"},
				Key:                  "env",
				Optional:             pointer.Ptr(true),
			},
		},
	},
	{
		Name: "MEMORY_LIMIT",
		ValueFrom: &corev1.EnvVarSource{
			ResourceFieldRef: &corev1.ResourceFieldSelector{
				ContainerName: "app",
				Resource:      "limits.memory",
				Divisor:       resource.MustParse("1Mi"),
			},
		},
	},
	{
		Name: "CPU_LIMIT",
		ValueFrom: &corev1.EnvVarSource{
			ResourceFieldRef: &corev1.ResourceFieldSelector{
				Resource: "limits.cpu",
				Divisor:  resource.MustParse("1"),
			},
		},
	},
}

const valueFromTargetsJSON = `[{"name": "value-from", "ddTraceConfigs": [
	{"name": "DD_PROFILING_ENABLED", "value": true},
	{"name": "DD_SERVICE", "valueFrom": {"fieldRef": {"fieldPath": "metadata.labels['app']"}}},
	{"name": "DD_API_KEY", "valueFrom": {"secretKeyRef": {"name": "datadog-secret", "key": "api-key"}}},
	{"name": "DD_ENV", "valueFrom": {"configMapKeyRef": {"name": "datadog-config", "key": "env", "optional": true}}},
	{"name": "MEMORY_LIMIT", "valueFrom": {"resourceFieldRef": {"containerName": "app", "resource": "limits.memory", "divisor": "1Mi"}}},
	{"name": "CPU_LIMIT", "valueFrom": {"resourceFieldRef": {"resource": "limits.cpu", "divisor": 1}}}
]}]`

func TestNewInstrumentationConfig(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		expected    *InstrumentationConfig
		shouldErr   bool
		errContains string
	}{
		{
			name:       "valid config enabled namespaces",
			configPath: "testdata/enabled_namespaces.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:            true,
				OnDemand:           true,
				EnabledNamespaces:  []string{"application"},
				DisabledNamespaces: []string{},
				LibVersions: map[string]string{
					"python": "v3",
				},
				InjectorImageTag: "foo",
				Targets:          []Target{},
				InjectionMode:    "auto",
				APMBasic:         APMBasicConfig{TracerVersions: map[string]string{"c": "0"}},
			},
		},
		{
			name:       "config with extra fields errors",
			configPath: "testdata/extra_fields.yaml",
			shouldErr:  true,
			expected:   nil,
		},
		{
			name:       "valid config disabled namespaces",
			configPath: "testdata/disabled_namespaces.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:            true,
				OnDemand:           true,
				EnabledNamespaces:  []string{},
				DisabledNamespaces: []string{"default"},
				LibVersions: map[string]string{
					"python": "default",
				},
				InjectorImageTag: "foo",
				Targets:          []Target{},
				InjectionMode:    "auto",
				APMBasic:         APMBasicConfig{TracerVersions: map[string]string{"c": "0"}},
			},
		},
		{
			name:       "valid targets based config",
			configPath: "testdata/targets.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:           true,
				OnDemand:          true,
				EnabledNamespaces: []string{},
				InjectorImageTag:  "0",
				LibVersions:       map[string]string{},
				DisabledNamespaces: []string{
					"hacks",
				},
				Targets: []Target{
					{
						Name: "Billing Service",
						PodSelector: &PodSelector{
							MatchLabels: map[string]string{
								"app": "billing-service",
							},
							MatchExpressions: []SelectorMatchExpression{
								{
									Key:      "env",
									Operator: "In",
									Values:   []string{"prod"},
								},
							},
						},
						NamespaceSelector: &NamespaceSelector{
							MatchNames: []string{"billing"},
						},
						TracerVersions: map[string]string{
							"java": "default",
						},
						TracerConfigs: []TracerConfig{
							{
								Name:  "DD_PROFILING_ENABLED",
								Value: "true",
							},
							{
								Name:  "DD_DATA_JOBS_ENABLED",
								Value: "true",
							},
						},
					},
				},
				InjectionMode: "auto",
				APMBasic:      APMBasicConfig{TracerVersions: map[string]string{"c": "0"}},
			},
		},
		{
			name:       "valid targets based config with namespace label selector",
			configPath: "testdata/targets_namespace_labels.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:           true,
				OnDemand:          true,
				EnabledNamespaces: []string{},
				InjectorImageTag:  "0",
				LibVersions:       map[string]string{},
				DisabledNamespaces: []string{
					"hacks",
				},
				Targets: []Target{
					{
						Name: "Billing Service",
						PodSelector: &PodSelector{
							MatchLabels: map[string]string{
								"app": "billing-service",
							},
							MatchExpressions: []SelectorMatchExpression{
								{
									Key:      "env",
									Operator: "In",
									Values:   []string{"prod"},
								},
							},
						},
						NamespaceSelector: &NamespaceSelector{
							MatchLabels: map[string]string{
								"app": "billing",
							},
							MatchExpressions: []SelectorMatchExpression{
								{
									Key:      "env",
									Operator: "In",
									Values:   []string{"prod"},
								},
							},
						},
						TracerVersions: map[string]string{
							"java": "default",
						},
						TracerConfigs: []TracerConfig{
							{
								Name:  "DD_PROFILING_ENABLED",
								Value: "true",
							},
							{
								Name:  "DD_DATA_JOBS_ENABLED",
								Value: "true",
							},
						},
					},
				},
				InjectionMode: "auto",
				APMBasic:      APMBasicConfig{TracerVersions: map[string]string{"c": "0"}},
			},
		},
		{
			name:       "can provide DD_SERVICE from arbitrary label",
			configPath: "testdata/filter_service_env_var_from.yaml",
			expected: &InstrumentationConfig{
				Enabled:            true,
				OnDemand:           true,
				EnabledNamespaces:  []string{},
				DisabledNamespaces: []string{},
				InjectorImageTag:   "0",
				LibVersions:        map[string]string{},
				Targets: []Target{
					{
						Name: "name-services",
						TracerConfigs: []TracerConfig{
							{
								Name: "DD_SERVICE",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.labels['app.kubernetes.io/name']",
									},
								},
							},
						},
					},
				},
				InjectionMode: "auto",
				APMBasic:      APMBasicConfig{TracerVersions: map[string]string{"c": "0"}},
			},
		},
		{
			name:       "tracer configs with every valueFrom source",
			configPath: "testdata/targets_env_value_from.yaml",
			expected: &InstrumentationConfig{
				Enabled:            true,
				OnDemand:           true,
				EnabledNamespaces:  []string{},
				DisabledNamespaces: []string{},
				InjectorImageTag:   "0",
				LibVersions:        map[string]string{},
				Targets: []Target{
					{
						Name:          "value-from",
						TracerConfigs: valueFromTracerConfigs,
					},
				},
				InjectionMode: "auto",
				APMBasic:      APMBasicConfig{TracerVersions: map[string]string{"c": "0"}},
			},
		},
		{
			name:        "unknown field in valueFrom",
			configPath:  "testdata/targets_env_value_from_extra_field.yaml",
			shouldErr:   true,
			errContains: "fooBar",
		},
		{
			name:        "unknown field in tracer config",
			configPath:  "testdata/targets_env_extra_field.yaml",
			shouldErr:   true,
			errContains: "vaule",
		},
		{
			name:       "both enabled and disabled namespaces",
			configPath: "testdata/both_enabled_and_disabled.yaml",
			shouldErr:  true,
		},
		{
			name:       "both labels and names for a namespace",
			configPath: "testdata/both_labels_and_names.yaml",
			shouldErr:  true,
		},
		{
			name:       "both enabled namespaces and targets",
			configPath: "testdata/both_enabled_and_targets.yaml",
			shouldErr:  true,
		},
		{
			name:       "both library versions and targets",
			configPath: "testdata/both_versions_and_targets.yaml",
			shouldErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.NewFromFile(t, tt.configPath)
			actual, err := NewInstrumentationConfig(mockConfig)
			if tt.shouldErr {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestLibVersionsEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		expected map[string]string
	}{
		{
			name: "valid lib versions",
			expected: map[string]string{
				"python": "1",
				"js":     "2",
				"java":   "3",
			},
		},
		{
			name:     "empty",
			expected: map[string]string{},
		},
		{
			name:     "nil",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.expected)
			require.NoError(t, err)
			t.Setenv("DD_APM_INSTRUMENTATION_LIB_VERSIONS", string(data))
			actual, err := NewInstrumentationConfig(configmock.New(t))
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual.LibVersions)
		})
	}
}

func TestEnabledNamespacesEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
	}{
		{
			name:     "valid namespaces",
			expected: []string{"foo", "bar"},
		},
		{
			name:     "empty",
			expected: []string{},
		},
		{
			name:     "nil",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.expected)
			require.NoError(t, err)
			t.Setenv("DD_APM_INSTRUMENTATION_ENABLED_NAMESPACES", string(data))
			actual, err := NewInstrumentationConfig(configmock.New(t))
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual.EnabledNamespaces)
		})
	}
}

func TestDisabledNamespacesEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
	}{
		{
			name:     "valid namespaces",
			expected: []string{"default", "kube-system"},
		},
		{
			name:     "empty",
			expected: []string{},
		},
		{
			name:     "nil",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.expected)
			require.NoError(t, err)
			t.Setenv("DD_APM_INSTRUMENTATION_DISABLED_NAMESPACES", string(data))
			actual, err := NewInstrumentationConfig(configmock.New(t))
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual.DisabledNamespaces)
		})
	}
}

func TestTargetEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		expected []Target
	}{
		{
			name: "valid target",
			expected: []Target{
				{
					Name: "Billing Service",
					PodSelector: &PodSelector{
						MatchLabels: map[string]string{
							"app": "billing-service",
						},
						MatchExpressions: []SelectorMatchExpression{
							{
								Key:      "env",
								Operator: "In",
								Values:   []string{"prod"},
							},
						},
					},
					NamespaceSelector: &NamespaceSelector{
						MatchNames: []string{"billing"},
					},
					TracerVersions: map[string]string{
						"java": "default",
					},
				},
			},
		},
		{
			name: "target with many omitted fields",
			expected: []Target{
				{
					Name: "Billing Service",
					PodSelector: &PodSelector{
						MatchLabels: map[string]string{
							"app": "billing-service",
						},
					},
				},
			},
		},
		{
			name: "target with env valueFrom",
			expected: []Target{
				{
					Name: "default-target",
					TracerConfigs: []TracerConfig{
						{
							Name: "DD_SERVICE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.labels['foo-bar']",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.expected)
			require.NoError(t, err)

			t.Setenv("DD_APM_INSTRUMENTATION_TARGETS", string(data))

			actual, err := NewInstrumentationConfig(configmock.New(t))
			require.NoError(t, err)

			require.Equal(t, tt.expected, actual.Targets)
		})
	}
}

func TestTargetEnvVarValueFrom(t *testing.T) {
	t.Setenv("DD_APM_INSTRUMENTATION_TARGETS", valueFromTargetsJSON)

	actual, err := NewInstrumentationConfig(configmock.New(t))
	require.NoError(t, err)

	require.Equal(t, []Target{{Name: "value-from", TracerConfigs: valueFromTracerConfigs}}, actual.Targets)
}

func TestGetPinnedLibraries(t *testing.T) {

	tests := []struct {
		name          string
		libVersions   map[string]string
		checkDefaults bool
		expected      pinnedLibraries
	}{
		{
			name:     "no pinned library versions",
			expected: pinnedLibraries{areSetToDefaults: false},
		},
		{
			name:          "no pinned library versions, checkDefaults",
			checkDefaults: true,
			expected:      pinnedLibraries{areSetToDefaults: false},
		},
		{
			name:        "default libs, not checking defaults always false",
			libVersions: defaultLibraries,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
					defaultLibInfo(php),
				},
			},
		},
		{
			name:          "default libs",
			libVersions:   defaultLibraries,
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
					defaultLibInfo(php),
				},
				areSetToDefaults: true,
			},
		},
		{
			name: "default libs with explicit c lib",
			libVersions: map[string]string{
				"java":   "v1",
				"python": "v4",
				"js":     "v6",
				"dotnet": "v3",
				"ruby":   "v2",
				"php":    "v1",
				"c":      "v0",
			},
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
					defaultLibInfo(php),
					defaultLibInfoWithVersion(c, "v0"),
				},
				areSetToDefaults: false,
			},
		},
		{
			name: "explicit c lib cannot substitute for missing default lib",
			libVersions: map[string]string{
				"java":   "v1",
				"python": "v4",
				"js":     "v6",
				"dotnet": "v3",
				"ruby":   "v2",
				"c":      "v0",
			},
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
					defaultLibInfoWithVersion(c, "v0"),
				},
				areSetToDefaults: false,
			},
		},
		{
			name:          "default libs, one missing",
			libVersions:   defaultLibrariesFor("java", "python", "js", "dotnet"),
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
				},
			},
		},
		{
			name: "explicitly default libs",
			libVersions: map[string]string{
				"java":   "default",
				"python": "default",
				"js":     "default",
				"dotnet": "default",
				"ruby":   "default",
				"php":    "default",
			},
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
					defaultLibInfo(php),
				},
				areSetToDefaults: true,
			},
		},
		{
			name: "default libs (major versions)",
			libVersions: map[string]string{
				"java":   "v1",
				"python": "v4",
				"js":     "v6",
				"dotnet": "v3",
				"ruby":   "v2",
				"php":    "v1",
			},
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfo(js),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
					defaultLibInfo(php),
				},
				areSetToDefaults: true,
			},
		},
		{
			name: "default libs (major versions mismatch)",
			libVersions: map[string]string{
				"java":   "v1",
				"python": "v4",
				"js":     "v3",
				"dotnet": "v3",
				"ruby":   "v2",
			},
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					defaultLibInfoWithVersion(js, "v3"),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
				},
				areSetToDefaults: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pinned := getPinnedLibraries(tt.libVersions, "registry", tt.checkDefaults)
			require.ElementsMatch(t, tt.expected.libs, pinned.libs, "libs match")
			require.Equal(t, tt.expected.areSetToDefaults, pinned.areSetToDefaults, "areSetToDefaults match")
		})
	}
}
