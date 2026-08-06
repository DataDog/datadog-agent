// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeTestGroups(t *testing.T) {
	out := bytes.NewBufferString(`
{"name":"config-abc","signature":"abc","fields":["networkIngressEnabled=true"],"run":["TestA","TestB"],"expectedRebuilds":1}
{"name":"default","signature":"000","skip":["TestA","TestB"],"fresh":["TestC"],"expectedRebuilds":2}
`)
	groups, err := decodeTestGroups(out)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, []string{"TestA", "TestB"}, groups[0].Run)
	assert.Equal(t, []string{"TestA", "TestB"}, groups[1].Skip)
	assert.Equal(t, 2, groups[1].ExpectedRebuilds)
	assert.NoError(t, validateTestGroups(groups))

	assert.Equal(t, []string{"-test.run", "^(TestA|TestB)$"}, groups[0].testArgs())
	assert.Equal(t, []string{"-test.skip", "^(TestA|TestB)$"}, groups[1].testArgs())
}

func TestDecodeTestGroupsRejectsGarbage(t *testing.T) {
	for name, input := range map[string]string{
		"empty":         "",
		"not json":      "some log line the suite printed\n",
		"unknown field": `{"name":"x","run":["TestA"],"somethingNew":1}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeTestGroups(bytes.NewBufferString(input))
			assert.Error(t, err, "a suite that does not speak this protocol must fall back, not be trusted")
		})
	}
}

func TestValidateTestGroups(t *testing.T) {
	// Anything that would drop a test, run one twice, or splice something odd
	// into a regexp has to be rejected so the caller falls back to one pass.
	tests := map[string][]testGroup{
		"a test in two passes": {
			{Name: "a", Run: []string{"TestA"}},
			{Name: "b", Run: []string{"TestA"}},
			{Name: "default", Skip: []string{"TestA"}},
		},
		"skip does not cover every claimed test": {
			{Name: "a", Run: []string{"TestA"}},
			{Name: "b", Run: []string{"TestB"}},
			{Name: "default", Skip: []string{"TestA"}},
		},
		"skip covers a test no pass claims": {
			{Name: "a", Run: []string{"TestA"}},
			{Name: "default", Skip: []string{"TestA", "TestB"}},
		},
		"no catch-all pass": {
			{Name: "a", Run: []string{"TestA"}},
			{Name: "b", Run: []string{"TestB"}},
		},
		"two catch-all passes": {
			{Name: "a", Skip: []string{"TestA"}},
			{Name: "b", Skip: []string{"TestA"}},
		},
		"a pass selects nothing": {
			{Name: "a"},
			{Name: "default", Skip: []string{"TestA"}},
		},
		"both run and skip": {
			{Name: "a", Run: []string{"TestA"}, Skip: []string{"TestB"}},
			{Name: "default", Skip: []string{"TestA"}},
		},
		"regexp metacharacter in a name": {
			{Name: "a", Run: []string{"TestA|TestEverythingElse"}},
			{Name: "default", Skip: []string{"TestA|TestEverythingElse"}},
		},
		"unnamed pass": {
			{Run: []string{"TestA"}},
			{Name: "default", Skip: []string{"TestA"}},
		},
	}

	for name, groups := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, validateTestGroups(groups))
		})
	}

	assert.NoError(t, validateTestGroups([]testGroup{
		{Name: "config-a", Run: []string{"TestA", "TestB"}},
		{Name: "config-b", Run: []string{"TestC"}},
		{Name: "default", Skip: []string{"TestA", "TestB", "TestC"}},
	}))
}

func TestPassPlans(t *testing.T) {
	assert.Equal(t, []testPassPlan{{}}, singlePass())

	plans := passPlans([]testGroup{
		{Name: "config-a", Run: []string{"TestA"}},
		{Name: "default", Skip: []string{"TestA"}},
	})
	require.Len(t, plans, 2)
	// The suffix keeps each pass's junit and testjson reports distinct.
	assert.Equal(t, "-config-a", plans[0].suffix)
	assert.Equal(t, "-default", plans[1].suffix)
	assert.NotEqual(t, plans[0].suffix, plans[1].suffix)

	// Each pass carries its filter and the name of the group it is running, so
	// the suite can check its own rebuild count against that group's prediction.
	assert.Equal(t, []string{"-test.run", "^(TestA)$", "-cws-group", "config-a"}, plans[0].args)
	assert.Equal(t, []string{"-test.skip", "^(TestA)$", "-cws-group", "default"}, plans[1].args)
}

func TestHasRunFilters(t *testing.T) {
	// A configured filter has to win over grouping: both use -test.run, and the
	// last flag on the command line is the one that takes effect.
	cfg := &testConfig{userProvidedConfig: userProvidedConfig{
		PackagesRunConfig: map[string]packageRunConfiguration{
			"pkg/security/tests": {RunOnly: []string{"TestOnlyThis"}},
			"pkg/other":          {Exclude: true},
		},
	}}
	assert.True(t, hasRunFilters(cfg, "pkg/security/tests"))
	assert.False(t, hasRunFilters(cfg, "pkg/other"))
	assert.False(t, hasRunFilters(cfg, "pkg/unlisted"))

	all := &testConfig{userProvidedConfig: userProvidedConfig{
		PackagesRunConfig: map[string]packageRunConfiguration{
			matchAllPackages: {Skip: []string{"TestFlaky"}},
		},
	}}
	assert.True(t, hasRunFilters(all, "pkg/security/tests"))
}

func TestGroupingEnabled(t *testing.T) {
	// Off unless a job asks for it: splitting the suite into passes can expose an
	// ordering dependency, so the default has to stay the status quo.
	t.Setenv(groupingEnvVar, "")
	assert.False(t, groupingEnabled(nil))
	assert.False(t, groupingEnabled([]string{"SOMETHING=1"}))
	assert.False(t, groupingEnabled([]string{groupingEnvVar + "="}))
	assert.False(t, groupingEnabled([]string{groupingEnvVar + "=0"}))
	assert.False(t, groupingEnabled([]string{groupingEnvVar + "=false"}))

	assert.True(t, groupingEnabled([]string{groupingEnvVar + "=1"}))
	assert.True(t, groupingEnabled([]string{"OTHER=x", groupingEnvVar + "=true"}))

	// The suite's environment wins over the runner's, since that is where a KMT
	// job's additional_env_vars land.
	t.Setenv(groupingEnvVar, "1")
	assert.True(t, groupingEnabled(nil))
	assert.False(t, groupingEnabled([]string{groupingEnvVar + "=0"}))
}

func TestGroupablePackages(t *testing.T) {
	for _, pkg := range []string{"pkg/security/tests", "pkg/security"} {
		assert.True(t, groupablePackages.MatchString(pkg), pkg)
	}
	for _, pkg := range []string{"pkg/network/tracer", "pkg/securityfoo", "pkg/ebpf"} {
		assert.False(t, groupablePackages.MatchString(pkg), pkg)
	}
}
