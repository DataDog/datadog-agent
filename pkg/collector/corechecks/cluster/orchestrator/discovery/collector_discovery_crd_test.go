// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
