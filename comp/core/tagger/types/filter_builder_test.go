// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterBuilderOps(t *testing.T) {
	tests := []struct {
		name              string
		do                func(*FilterBuilder)
		buildCard         TagCardinality
		expectBuildFilter Filter
	}{
		{
			name:      "do nothing",
			do:        func(_ *FilterBuilder) {},
			buildCard: HighCardinality,
			expectBuildFilter: Filter{
				prefixes:    map[EntityIDPrefix]struct{}{},
				cardinality: HighCardinality,
			},
		},
		{
			name: "only includes",
			do: func(fb *FilterBuilder) {
				fb.Include(KubernetesDeployment, ContainerID)
				fb.Include(Host)
			},
			buildCard: HighCardinality,
			expectBuildFilter: Filter{
				prefixes: map[EntityIDPrefix]struct{}{
					KubernetesDeployment: {},
					ContainerID:          {},
					Host:                 {},
				},
				cardinality: HighCardinality,
			},
		},
		{
			name: "only excludes",
			do: func(fb *FilterBuilder) {
				fb.Exclude(KubernetesDeployment, ContainerID)
				fb.Exclude(Host)
			},
			buildCard: HighCardinality,
			expectBuildFilter: Filter{
				prefixes: map[EntityIDPrefix]struct{}{
					ContainerImageMetadata: {},
					ECSTask:                {},
					KubernetesMetadata:     {},
					KubernetesPodUID:       {},
					Process:                {},
				},
				cardinality: HighCardinality,
			},
		},
		{
			name: "both includes and excludes",
			do: func(fb *FilterBuilder) {
				fb.Include(ContainerImageMetadata)
				fb.Exclude(KubernetesDeployment, ContainerID)
				fb.Include(ContainerID)
				fb.Exclude(Host, KubernetesMetadata)
				fb.Include(Host, Process)
			},
			buildCard: HighCardinality,
			expectBuildFilter: Filter{
				prefixes: map[EntityIDPrefix]struct{}{
					ContainerImageMetadata: {},
					Process:                {},
				},
				cardinality: HighCardinality,
			},
		},
	}

	for _, test := range tests {
		fb := NewFilterBuilder()
		test.do(fb)
		filter := fb.Build(test.buildCard)
		assert.Truef(t, reflect.DeepEqual(*filter, test.expectBuildFilter), "expected %v, found %v", test.expectBuildFilter, filter)
	}
}

func TestNilFilterBuilderOps(t *testing.T) {
	var fb *FilterBuilder

	assert.Panics(t, func() { fb.Include(ContainerID) })
	assert.Panics(t, func() { fb.Exclude(ContainerID) })
	assert.Panics(t, func() { fb.Build(HighCardinality) })
}
