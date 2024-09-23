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

func TestFilterOps(t *testing.T) {
	f := Filter{
		prefixes: map[EntityIDPrefix]struct{}{
			KubernetesDeployment: {},
			KubernetesPodUID:     {},
		},
		cardinality: OrchestratorCardinality,
	}

	// assert cardinality is correct
	cardinality := f.GetCardinality()
	assert.Equal(t, OrchestratorCardinality, cardinality)

	// assert GetPrefixes
	expectedPrefixes := map[EntityIDPrefix]struct{}{
		KubernetesDeployment: {},
		KubernetesPodUID:     {},
	}
	assert.Truef(t, reflect.DeepEqual(expectedPrefixes, f.GetPrefixes()), "expected %v, found %v", expectedPrefixes, f.GetPrefixes())
}

func TestNilFilter(t *testing.T) {
	var f *Filter

	assert.Truef(t, reflect.DeepEqual(f.GetPrefixes(), AllPrefixesSet()), "expected %v, found %v", AllPrefixesSet(), f.GetPrefixes())
	assert.Equalf(t, HighCardinality, f.GetCardinality(), "nil filter should have cardinality HIGH, found %v", f.GetCardinality())

	for prefix := range AllPrefixesSet() {
		assert.Truef(t, f.MatchesPrefix(prefix), "nil filter should match any prefix, didn't match %v", prefix)
	}
}
