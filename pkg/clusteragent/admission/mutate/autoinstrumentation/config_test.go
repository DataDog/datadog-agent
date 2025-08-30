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

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewInstrumentationConfig(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		expected   *InstrumentationConfig
		shouldErr  bool
	}{
		{
			name:       "valid config enabled namespaces",
			configPath: "testdata/enabled_namespaces.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:            true,
				EnabledNamespaces:  []string{"default"},
				DisabledNamespaces: []string{},
				LibVersions: map[string]string{
					"python": "default",
				},
				Version:          "v2",
				InjectorImageTag: "foo",
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
				EnabledNamespaces:  []string{},
				DisabledNamespaces: []string{"default"},
				LibVersions: map[string]string{
					"python": "default",
				},
				Version:          "v2",
				InjectorImageTag: "foo",
			},
		},
		{
			name:       "valid targets based config",
			configPath: "testdata/targets.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:           true,
				EnabledNamespaces: []string{},
				InjectorImageTag:  "0",
				LibVersions:       map[string]string{},
				Version:           "v2",
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
			},
		},
		{
			name:       "valid targets based config with namespace label selector",
			configPath: "testdata/targets_namespace_labels.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:           true,
				EnabledNamespaces: []string{},
				InjectorImageTag:  "0",
				LibVersions:       map[string]string{},
				Version:           "v2",
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
			},
		},
		{
			name:       "can provide DD_SERVICE from arbitrary label",
			configPath: "testdata/filter_service_env_var_from.yaml",
			expected: &InstrumentationConfig{
				Enabled:            true,
				EnabledNamespaces:  []string{},
				DisabledNamespaces: []string{},
				InjectorImageTag:   "0",
				Version:            "v2",
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
			},
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
				"python": "v3",
				"js":     "v5",
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
				"python": "v3",
				"js":     "v3",
				"dotnet": "v3",
				"ruby":   "v2",
			},
			checkDefaults: true,
			expected: pinnedLibraries{
				libs: []libInfo{
					defaultLibInfo(java),
					defaultLibInfo(python),
					js.libInfo("", "registry/dd-lib-js-init:v3"),
					defaultLibInfo(dotnet),
					defaultLibInfo(ruby),
				},
				areSetToDefaults: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageResolver := newNoOpImageResolver()
			pinned := getPinnedLibraries(tt.libVersions, "registry", tt.checkDefaults, imageResolver)
			require.ElementsMatch(t, tt.expected.libs, pinned.libs, "libs match")
			require.Equal(t, tt.expected.areSetToDefaults, pinned.areSetToDefaults, "areSetToDefaults match")
		})
	}
}
