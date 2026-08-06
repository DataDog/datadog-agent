// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// These exercise the declaration registry itself and need neither root nor
// eBPF, so they run in any environment the suite is built for.

func TestDeclareTestFuncName(t *testing.T) {
	// runtime.FuncForPC's output format is load-bearing for the registry keys,
	// so pin the trimming rather than trusting the string shape.
	assert.Equal(t, "TestDeclareTestFuncName", testFuncName(TestDeclareTestFuncName))
	assert.Equal(t, "TestConfigSignatureMatchesEqual", testFuncName(TestConfigSignatureMatchesEqual))

	assert.Panics(t, func() { testFuncName("TestDeclareTestFuncName") },
		"a name string must not be accepted: it would silently desync on rename")
	assert.Panics(t, func() { testFuncName(func(*testing.T) {}) },
		"a closure has no resolvable test name")
	assert.Panics(t, func() { testFuncName(configSignature) },
		"a non-test function must not be accepted")
}

func TestConfigSignatureMatchesEqual(t *testing.T) {
	// Grouping puts two tests in the same run when their signatures match, and
	// newTestModule reuses the module when Equal says so. If the two ever
	// disagree, a run silently rebuilds the module and grouping stops paying
	// off without anything failing.
	configs := []testOpts{
		{},
		{networkIngressEnabled: true},
		{networkRawPacketEnabled: true},
		{networkIngressEnabled: true, networkRawPacketEnabled: true},
		{disableERPCDentryResolution: true},
		{disableMapDentryResolution: true},
		{activityDumpDuration: time.Second},
		{activityDumpDuration: 2 * time.Second},
		{dnsPort: 1},
		{dnsPort: 2},
		{envsWithValue: nil},
		{envsWithValue: []string{}},
		{envsWithValue: []string{"LD_PRELOAD"}},
		{envsWithValue: []string{"LD_PRELOAD", "PATH"}},
		{envsWithValue: []string{"PATH", "LD_PRELOAD"}},
		{securityProfileDir: "a"},
		{securityProfileDir: "b"},
		{securityProfileDir: `a"`},
		{activityDumpLocalStorageFormats: []string{"json", "protobuf"}},
		{activityDumpLocalStorageFormats: []string{"profile"}},
	}

	for i, a := range configs {
		for j, b := range configs {
			assert.Equalf(t, a.Equal(b), configSignature(a) == configSignature(b),
				"Equal and configSignature disagree on configs %d and %d:\n%v\n%v",
				i, j, nonDefaultFields(a), nonDefaultFields(b))
		}
	}

	// Two independently built copies of the same config must group together.
	assert.Equal(t,
		configSignature(testOpts{envsWithValue: []string{"LD_PRELOAD"}}),
		configSignature(testOpts{envsWithValue: []string{"LD_PRELOAD"}}))
}

func TestUndeclarableFields(t *testing.T) {
	assert.Empty(t, undeclarableFields(testOpts{}))
	assert.Empty(t, undeclarableFields(testOpts{networkIngressEnabled: true}))

	assert.Equal(t, []string{"preStartCallback"},
		undeclarableFields(testOpts{preStartCallback: func(*testModule) {}}))
	assert.Equal(t, []string{"ruleMatchHandler"},
		undeclarableFields(testOpts{ruleMatchHandler: func(*testModule, *model.Event, *rules.Rule) {}}))
	assert.Equal(t, []string{"tagger"},
		undeclarableFields(testOpts{tagger: NewFakeMonoTagger()}))

	// declare must refuse such a config outright rather than accept something it
	// cannot group correctly.
	assert.Panics(t, func() {
		declare(TestUndeclarableFields, testOpts{preStartCallback: func(*testModule) {}})
	})
}

func TestLookupDeclaredConfigWalksSubtestPath(t *testing.T) {
	const parent = "TestLookupDeclaredConfigWalksSubtestPathFixture"
	declaredConfigs[parent] = &declaredConfig{name: parent, opts: testOpts{dnsPort: 53}}
	t.Cleanup(func() { delete(declaredConfigs, parent) })

	for _, name := range []string{parent, parent + "/sub", parent + "/sub/deeper"} {
		d, ok := lookupDeclaredConfig(name)
		require.Truef(t, ok, "no declaration found for %q", name)
		assert.Equal(t, parent, d.name)
	}

	_, ok := lookupDeclaredConfig(parent + "Suffix")
	assert.False(t, ok, "a name that merely shares a prefix must not match")

	_, ok = lookupDeclaredConfig("TestSomethingUndeclared")
	assert.False(t, ok)
}

func TestTestRunsPartition(t *testing.T) {
	runs := testRuns()
	require.NotEmpty(t, runs)

	last := runs[len(runs)-1]
	assert.Equal(t, defaultRunName, last.Name,
		"the default run must come last, so a suite with nothing declared is exactly today's single pass")
	assert.Empty(t, last.Run, "the default run is expressed as a skip, not as a list")

	// Every declared test appears in exactly one run, and the default run skips
	// precisely those. This is the invariant that guarantees no test is dropped
	// or run twice.
	seen := map[string]string{}
	for _, run := range runs[:len(runs)-1] {
		assert.NotEmptyf(t, run.Run, "run %s has no tests", run.Name)
		for _, name := range run.Run {
			if other, dup := seen[name]; dup {
				t.Errorf("%s appears in both %s and %s", name, other, run.Name)
			}
			seen[name] = run.Name
			_, declared := lookupDeclaredConfig(name)
			assert.Truef(t, declared, "%s is scheduled but not declared", name)
		}
		if run.Name != ungroupedRunName {
			assert.Equalf(t, 1+len(run.Fresh), run.ExpectedRebuilds,
				"run %s: a config run builds the module once, plus once per fresh test", run.Name)
		} else {
			assert.GreaterOrEqualf(t, run.ExpectedRebuilds, len(run.Run),
				"run %s: every ungrouped test builds at least one module", run.Name)
		}
	}

	assert.Len(t, last.Skip, len(seen))
	for _, name := range last.Skip {
		assert.Containsf(t, seen, name, "the default run skips %s, which no run claims", name)
	}

	// Tests declared with the default config -- typically only to mark them
	// needsFreshModule -- stay in the default run rather than forming one of
	// their own.
	for name, d := range declaredConfigs {
		if !d.ungrouped && d.signature == defaultConfigSignature() {
			assert.NotContainsf(t, seen, name,
				"%s uses the default config and must stay in the default run", name)
		}
	}
}
