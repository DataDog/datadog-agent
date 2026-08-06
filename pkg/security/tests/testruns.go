// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

// testRuns partitions the suite into `go test` invocations that each build the
// eBPF module as few times as possible. Go's testing package cannot reorder
// m.Run(), so the ordering has to be separate invocations of the test binary.
// The partition comes from the declaration registry alone (testdecl.go), so
// -list-groups can report it without running anything.

import (
	"encoding/json"
	"io"
	"sort"
)

// testRun is one invocation of the test binary, consumed as JSON by the KMT
// test runner (test/new-e2e/system-probe/test-runner). Run and Skip hold plain
// top-level test names; the runner turns them into an anchored alternation.
type testRun struct {
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`

	// Fields and Reasons are for humans reading -list-groups: the config
	// this run is, and why the inline-config tests cannot share a module.
	Fields  []string `json:"fields,omitempty"`
	Reasons []string `json:"reasons,omitempty"`

	Run  []string `json:"run,omitempty"`
	Skip []string `json:"skip,omitempty"`

	// ExpectedRebuilds is what the run is predicted to cost, which the runner
	// prints. An upper bound: a skipped test builds nothing, and most
	// activity-dump tests are gated on DEDICATED_ACTIVITY_DUMP_NODE.
	ExpectedRebuilds int `json:"expectedRebuilds"`
}

// defaultRunName is a real config group, not a leftover bucket: every test in
// it uses the default config, so it builds the module once for most of the
// suite. Expressing it as a skip of everything declared is what sweeps in a
// newly added default-config test with no declaration at all.
const defaultRunName = "default"

// inlineConfigRunName holds the tests that build their own module. Each builds
// one regardless, so one shared run costs the same as several.
const inlineConfigRunName = "inline-config"

// testRuns returns the partition in execution order, default run last so that a
// suite with nothing declared degenerates to exactly one unfiltered pass.
func testRuns() []testRun {
	defaultSig := defaultConfigSignature()

	bySig := map[string][]*declaredConfig{}
	var inlineConfig []*declaredConfig

	for _, d := range declaredConfigs {
		switch {
		case d.inlineConfig:
			inlineConfig = append(inlineConfig, d)
		case d.signature == defaultSig:
			// Redundant but not an error; the test is already in the default run.
		default:
			bySig[d.signature] = append(bySig[d.signature], d)
		}
	}

	sigs := make([]string, 0, len(bySig))
	for sig := range bySig {
		sigs = append(sigs, sig)
	}
	sort.Strings(sigs)

	var runs []testRun
	var declaredElsewhere []string // what the default run has to skip

	for _, sig := range sigs {
		group := bySig[sig]
		run := testRun{
			Name:      "config-" + sig,
			Signature: sig,
			Fields:    nonDefaultFields(group[0].opts),
		}
		for _, d := range group {
			run.Run = append(run.Run, d.name)
		}
		sort.Strings(run.Run)
		run.ExpectedRebuilds = 1
		declaredElsewhere = append(declaredElsewhere, run.Run...)
		runs = append(runs, run)
	}

	if len(inlineConfig) > 0 {
		run := testRun{Name: inlineConfigRunName}
		for _, d := range inlineConfig {
			run.Run = append(run.Run, d.name)
			run.Reasons = append(run.Reasons, d.name+": "+d.reason)
			run.ExpectedRebuilds += max(1, d.moduleBuilds)
		}
		sort.Strings(run.Run)
		sort.Strings(run.Reasons)
		declaredElsewhere = append(declaredElsewhere, run.Run...)
		runs = append(runs, run)
	}

	sort.Strings(declaredElsewhere)
	runs = append(runs, testRun{
		Name:             defaultRunName,
		Signature:        defaultSig,
		Skip:             declaredElsewhere,
		ExpectedRebuilds: 1,
	})

	return runs
}

// printTestRuns writes one JSON object per line, so a human and the KMT test
// runner read the same output.
func printTestRuns(w io.Writer) error {
	enc := json.NewEncoder(w)
	for _, run := range testRuns() {
		if err := enc.Encode(run); err != nil {
			return err
		}
	}
	return nil
}
