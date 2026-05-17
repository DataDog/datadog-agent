// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiscoveryCollector_List(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *DiscoveryCollector
		group    string
		version  string
		kind     string
		expected []CollectorVersion
	}{
		{
			name: "exact match with group, version, and kind",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "apps/v1", Kind: "deployments"}:  {},
							{GroupVersion: "apps/v1", Kind: "statefulsets"}: {},
							{GroupVersion: "v1", Kind: "pods"}:              {},
						},
					},
				}
			},
			group:   "apps",
			version: "v1",
			kind:    "deployments",
			expected: []CollectorVersion{
				{GroupVersion: "apps/v1", Kind: "deployments"},
			},
		},
		{
			name: "match by group and version only",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "apps/v1", Kind: "deployments"}:  {},
							{GroupVersion: "apps/v1", Kind: "statefulsets"}: {},
							{GroupVersion: "v1", Kind: "pods"}:              {},
						},
					},
				}
			},
			group:   "apps",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{GroupVersion: "apps/v1", Kind: "deployments"},
				{GroupVersion: "apps/v1", Kind: "statefulsets"},
			},
		},
		{
			name: "match by core group (empty string) and version",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "v1", Kind: "pods"}:             {},
							{GroupVersion: "v1", Kind: "nodes"}:            {},
							{GroupVersion: "apps/v1", Kind: "deployments"}: {},
						},
					},
				}
			},
			group:   "",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{GroupVersion: "v1", Kind: "pods"},
				{GroupVersion: "v1", Kind: "nodes"},
			},
		},
		{
			name: "exclude status resources",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "v1", Kind: "pods"}:             {},
							{GroupVersion: "v1", Kind: "pods/status"}:      {},
							{GroupVersion: "v1", Kind: "nodes"}:            {},
							{GroupVersion: "v1", Kind: "nodes/status"}:     {},
							{GroupVersion: "apps/v1", Kind: "deployments"}: {},
						},
					},
				}
			},
			group:   "",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{GroupVersion: "v1", Kind: "pods"},
				{GroupVersion: "v1", Kind: "nodes"},
			},
		},
		{
			name: "match by group only",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "apps/v1", Kind: "deployments"}:            {},
							{GroupVersion: "apps/v1beta1", Kind: "deployments"}:       {},
							{GroupVersion: "extensions/v1beta1", Kind: "deployments"}: {},
							{GroupVersion: "v1", Kind: "pods"}:                        {},
						},
					},
				}
			},
			group:   "apps",
			version: "",
			kind:    "",
			expected: []CollectorVersion{
				{GroupVersion: "apps/v1", Kind: "deployments"},
				{GroupVersion: "apps/v1beta1", Kind: "deployments"},
			},
		},
		{
			name: "does not match v1beta1 when exact version v1 is requested",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "eks.amazonaws.com/v1", Kind: "nodeclasses"}:      {},
							{GroupVersion: "eks.amazonaws.com/v1beta1", Kind: "nodeclasses"}: {},
						},
					},
				}
			},
			group:   "eks.amazonaws.com",
			version: "v1",
			kind:    "nodeclasses",
			expected: []CollectorVersion{
				{GroupVersion: "eks.amazonaws.com/v1", Kind: "nodeclasses"},
			},
		},
		{
			name: "does not match v1beta1 resources when v1 is requested without kind filter",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "apps/v1", Kind: "deployments"}:      {},
							{GroupVersion: "apps/v1beta1", Kind: "deployments"}: {},
							{GroupVersion: "apps/v1", Kind: "statefulsets"}:     {},
						},
					},
				}
			},
			group:   "apps",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{GroupVersion: "apps/v1", Kind: "deployments"},
				{GroupVersion: "apps/v1", Kind: "statefulsets"},
			},
		},
		{
			name: "no matches",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{GroupVersion: "v1", Kind: "pods"}: {},
						},
					},
				}
			},
			group:    "nonexistent",
			version:  "v1",
			kind:     "nonexistent",
			expected: []CollectorVersion{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := tt.setup()
			result := dc.List(tt.group, tt.version, tt.kind)
			assert.ElementsMatch(t, tt.expected, result, "List() returned unexpected result")
		})
	}
}

func TestDiscoveryCollector_OptimalVersion(t *testing.T) {
	tests := []struct {
		name             string
		setup            func() *DiscoveryCollector
		groupName        string
		preferredVersion string
		fallbackVersions []string
		expectedVersion  string
		expectedFound    bool
	}{
		{
			name: "returns preferred version when available",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "datadoghq.com",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
									{Version: "v1alpha2"},
									{Version: "v1beta1"},
								},
							},
						},
					},
				}
			},
			groupName:        "datadoghq.com",
			preferredVersion: "v1alpha2",
			fallbackVersions: []string{"v1alpha1", "v1beta1"},
			expectedVersion:  "v1alpha2",
			expectedFound:    true,
		},
		{
			name: "returns first available version when preferred not supported",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "datadoghq.com",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
									{Version: "v1beta1"},
								},
							},
						},
					},
				}
			},
			groupName:        "datadoghq.com",
			preferredVersion: "v1alpha2", // not supported
			fallbackVersions: []string{"v1beta1", "v1alpha1"},
			expectedVersion:  "v1beta1",
			expectedFound:    true,
		},
		{
			name: "returns second available version when first not supported",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "argoproj.io",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
									{Version: "v1beta1"},
								},
							},
						},
					},
				}
			},
			groupName:        "argoproj.io",
			preferredVersion: "v2",                             // not supported
			fallbackVersions: []string{"v1alpha2", "v1alpha1"}, // first not supported
			expectedVersion:  "v1alpha1",
			expectedFound:    true,
		},
		{
			name: "returns false when group not found",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "datadoghq.com",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
								},
							},
						},
					},
				}
			},
			groupName:        "unknown.io",
			preferredVersion: "v1alpha1",
			fallbackVersions: []string{"v1alpha1"},
			expectedVersion:  "",
			expectedFound:    false,
		},
		{
			name: "returns false when no versions supported",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "datadoghq.com",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
									{Version: "v1alpha2"},
								},
							},
						},
					},
				}
			},
			groupName:        "datadoghq.com",
			preferredVersion: "v2",                      // not supported
			fallbackVersions: []string{"v1beta1", "v3"}, // none supported
			expectedVersion:  "",
			expectedFound:    false,
		},
		{
			name: "works with empty preferred version",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "karpenter.sh",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1"},
									{Version: "v1beta1"},
								},
							},
						},
					},
				}
			},
			groupName:        "karpenter.sh",
			preferredVersion: "", // empty
			fallbackVersions: []string{"v1"},
			expectedVersion:  "v1",
			expectedFound:    true,
		},
		{
			name: "handles empty available versions",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "datadoghq.com",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
								},
							},
						},
					},
				}
			},
			groupName:        "datadoghq.com",
			preferredVersion: "v1alpha2", // not supported
			fallbackVersions: []string{}, // empty
			expectedVersion:  "",
			expectedFound:    false,
		},
		{
			name: "skips empty versions in available list",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						Groups: []*v1.APIGroup{
							{
								Name: "datadoghq.com",
								Versions: []v1.GroupVersionForDiscovery{
									{Version: "v1alpha1"},
									{Version: "v1alpha2"},
								},
							},
						},
					},
				}
			},
			groupName:        "datadoghq.com",
			preferredVersion: "v2",                         // not supported
			fallbackVersions: []string{"", "v1alpha1", ""}, // empty strings
			expectedVersion:  "v1alpha1",
			expectedFound:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := tt.setup()
			version, found := dc.OptimalVersion(tt.groupName, tt.preferredVersion, tt.fallbackVersions)

			assert.Equal(t, tt.expectedFound, found, "OptimalVersion() found mismatch")
			assert.Equal(t, tt.expectedVersion, version, "OptimalVersion() version mismatch")
		})
	}
}

func TestMatchesGroupVersion(t *testing.T) {
	tests := []struct {
		name     string
		cv       string
		group    string
		version  string
		expected bool
	}{
		// both empty → match all
		{name: "both empty matches anything", cv: "apps/v1", group: "", version: "", expected: true},
		{name: "both empty matches core group", cv: "v1", group: "", version: "", expected: true},

		// version empty → match any version in the group (prefix)
		{name: "version empty matches any version in group", cv: "apps/v1", group: "apps", version: "", expected: true},
		{name: "version empty matches v1beta1 in group", cv: "apps/v1beta1", group: "apps", version: "", expected: true},
		{name: "version empty does not match different group", cv: "extensions/v1beta1", group: "apps", version: "", expected: false},
		{name: "version empty does not match group with shared prefix", cv: "appsextended/v1", group: "apps", version: "", expected: false},

		// group empty → core group exact version match
		{name: "group empty matches exact core version", cv: "v1", group: "", version: "v1", expected: true},
		{name: "group empty does not match v1beta1 when v1 requested", cv: "v1beta1", group: "", version: "v1", expected: false},
		{name: "group empty does not match grouped resource", cv: "apps/v1", group: "", version: "v1", expected: false},

		// both set → exact match
		{name: "exact match", cv: "apps/v1", group: "apps", version: "v1", expected: true},
		{name: "does not match v1beta1 when v1 requested", cv: "apps/v1beta1", group: "apps", version: "v1", expected: false},
		{name: "does not match different group", cv: "extensions/v1", group: "apps", version: "v1", expected: false},
		{name: "does not match different version", cv: "apps/v2", group: "apps", version: "v1", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchesGroupVersion(tt.cv, tt.group, tt.version))
		})
	}
}
