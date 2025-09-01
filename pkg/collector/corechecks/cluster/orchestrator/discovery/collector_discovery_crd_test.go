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
							{Version: "apps/v1", Name: "deployments"}:  {},
							{Version: "apps/v1", Name: "statefulsets"}: {},
							{Version: "v1", Name: "pods"}:              {},
						},
					},
				}
			},
			group:   "apps",
			version: "v1",
			kind:    "deployments",
			expected: []CollectorVersion{
				{Version: "apps/v1", Name: "deployments"},
			},
		},
		{
			name: "match by group and version only",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{Version: "apps/v1", Name: "deployments"}:  {},
							{Version: "apps/v1", Name: "statefulsets"}: {},
							{Version: "v1", Name: "pods"}:              {},
						},
					},
				}
			},
			group:   "apps",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{Version: "apps/v1", Name: "deployments"},
				{Version: "apps/v1", Name: "statefulsets"},
			},
		},
		{
			name: "match by core group (empty string) and version",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{Version: "v1", Name: "pods"}:             {},
							{Version: "v1", Name: "nodes"}:            {},
							{Version: "apps/v1", Name: "deployments"}: {},
						},
					},
				}
			},
			group:   "",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{Version: "v1", Name: "pods"},
				{Version: "v1", Name: "nodes"},
			},
		},
		{
			name: "exclude status resources",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{Version: "v1", Name: "pods"}:             {},
							{Version: "v1", Name: "pods/status"}:      {},
							{Version: "v1", Name: "nodes"}:            {},
							{Version: "v1", Name: "nodes/status"}:     {},
							{Version: "apps/v1", Name: "deployments"}: {},
						},
					},
				}
			},
			group:   "",
			version: "v1",
			kind:    "",
			expected: []CollectorVersion{
				{Version: "v1", Name: "pods"},
				{Version: "v1", Name: "nodes"},
			},
		},
		{
			name: "match by group only",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{Version: "apps/v1", Name: "deployments"}:            {},
							{Version: "apps/v1beta1", Name: "deployments"}:       {},
							{Version: "extensions/v1beta1", Name: "deployments"}: {},
							{Version: "v1", Name: "pods"}:                        {},
						},
					},
				}
			},
			group:   "apps",
			version: "",
			kind:    "",
			expected: []CollectorVersion{
				{Version: "apps/v1", Name: "deployments"},
				{Version: "apps/v1beta1", Name: "deployments"},
			},
		},
		{
			name: "no matches",
			setup: func() *DiscoveryCollector {
				return &DiscoveryCollector{
					cache: DiscoveryCache{
						CollectorForVersion: map[CollectorVersion]struct{}{
							{Version: "v1", Name: "pods"}: {},
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
