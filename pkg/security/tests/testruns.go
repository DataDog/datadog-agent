// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package tests holds tests related files
package tests

// testRuns partitions the suite into a sequence of `go test` invocations that
// each build the eBPF module as few times as possible. The partition is derived
// from the declaration registry alone (see testdecl.go), so -cws-list-groups
// can report it without running anything.
//
// Go's testing package cannot reorder m.Run(), so ordering has to be expressed
// as separate invocations of the test binary: one per static config.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync/atomic"
)

// testRun is one invocation of the test binary. Run and Skip are lists of
// top-level test names; the caller turns them into a single -test.run /
// -test.skip anchored alternation.
type testRun struct {
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`

	// Fields is the human-readable form of Signature: the config fields that
	// differ from the default.
	Fields []string `json:"fields,omitempty"`

	// Reasons explains, for the ungrouped run, why each of its tests cannot
	// share a module.
	Reasons []string `json:"reasons,omitempty"`

	Run  []string `json:"run,omitempty"`
	Skip []string `json:"skip,omitempty"`

	// Fresh lists the tests in this run that force a rebuild even though the
	// live module already matches their config.
	Fresh []string `json:"fresh,omitempty"`

	// ExpectedRebuilds is how many times this run should build the module: for
	// a config run, one for the run itself plus one per test in Fresh; for the
	// ungrouped run, the sum of what its tests build.
	//
	// It is an upper bound, for two reasons. A skipped test builds nothing --
	// which matters here, since most of the activity-dump tests are gated on
	// DEDICATED_ACTIVITY_DUMP_NODE. And Go runs the tests a pass selects in
	// source order, which no -test.run pattern can change, so a fresh test that
	// happens to come first absorbs the run's own build instead of adding to it.
	ExpectedRebuilds int `json:"expectedRebuilds"`
}

// defaultRunName is the run that holds every test with no declared config. It
// is a real config group, not a leftover bucket: all of its tests use the
// default config, so it builds the module once for the large majority of the
// suite. Expressing it as a skip of everything declared is what lets a newly
// added default-config test be swept in with no declaration at all.
const defaultRunName = "default"

// ungroupedRunName holds the tests that cannot share a module with anything.
// Each rebuilds regardless, so putting them in one run or in several costs the
// same; one shared run is simpler.
const ungroupedRunName = "ungrouped"

// testRuns returns the partition, in the order the runs should execute. The
// default run is last so that a suite with nothing declared yet degenerates to
// exactly today's single pass.
func testRuns() []testRun {
	defaultSig := defaultConfigSignature()

	bySig := map[string][]*declaredConfig{}
	var ungrouped []*declaredConfig
	// Tests declared with the default config -- typically to mark them
	// needsFreshModule -- belong to the default run, not to one of their own.
	var defaultFresh []string

	for _, d := range declaredConfigs {
		switch {
		case d.ungrouped:
			ungrouped = append(ungrouped, d)
		case d.signature == defaultSig:
			if d.needsFresh {
				defaultFresh = append(defaultFresh, d.name)
			}
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
			if d.needsFresh {
				run.Fresh = append(run.Fresh, d.name)
			}
		}
		sort.Strings(run.Run)
		sort.Strings(run.Fresh)
		run.ExpectedRebuilds = 1 + len(run.Fresh)
		declaredElsewhere = append(declaredElsewhere, run.Run...)
		runs = append(runs, run)
	}

	if len(ungrouped) > 0 {
		run := testRun{Name: ungroupedRunName}
		for _, d := range ungrouped {
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
	sort.Strings(defaultFresh)
	runs = append(runs, testRun{
		Name:             defaultRunName,
		Signature:        defaultSig,
		Skip:             declaredElsewhere,
		Fresh:            defaultFresh,
		ExpectedRebuilds: 1 + len(defaultFresh),
	})

	return runs
}

// printTestRuns writes the partition as one JSON object per line, so that both
// a human and the KMT test runner can read the same output.
func printTestRuns(w io.Writer) error {
	enc := json.NewEncoder(w)
	for _, run := range testRuns() {
		if err := enc.Encode(run); err != nil {
			return err
		}
	}
	return nil
}

// moduleBuilds counts how many times this process built the eBPF module.
// Grouping exists to keep that number at the floor for each run, so the count is
// the metric the whole design is judged by -- see checkModuleBuilds.
var moduleBuilds atomic.Int64

func recordModuleBuild() {
	moduleBuilds.Add(1)
}

// checkModuleBuilds compares the modules this run actually built against what
// its partition predicted, and reports a non-zero exit code if it built more.
//
// Building *fewer* is normal and not a regression: a skipped test builds
// nothing, and most of the activity-dump tests are gated on
// DEDICATED_ACTIVITY_DUMP_NODE. Building more means a test in this run did not
// use the config the run was built for -- exactly the regression grouping is
// meant to prevent, and one that is otherwise invisible because every test
// still passes.
func checkModuleBuilds(runName string) int {
	if runName == "" {
		return 0
	}

	observed := int(moduleBuilds.Load())
	for _, run := range testRuns() {
		if run.Name != runName {
			continue
		}
		if observed > run.ExpectedRebuilds {
			fmt.Printf("[grouping] run %q built the module %d times, expected at most %d: "+
				"a test in this run is not using the config the run was scheduled for\n",
				runName, observed, run.ExpectedRebuilds)
			return 1
		}
		fmt.Printf("[grouping] run %q built the module %d times (expected at most %d)\n",
			runName, observed, run.ExpectedRebuilds)
		return 0
	}

	fmt.Printf("[grouping] unknown run %q, built the module %d times\n", runName, observed)
	return 0
}
