// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

// The CWS functional suite rebuilds its eBPF manager whenever a test needs a
// different static config than the live module has, which costs seconds to tens
// of seconds depending on the kernel. Tests that share a config are scattered
// through source order, so the suite pays that cost far more often than the
// number of distinct configs requires.
//
// Go's testing package cannot reorder m.Run(), so the ordering has to be
// expressed as separate invocations of the test binary. A suite that supports
// -cws-list-groups reports how it wants to be partitioned; this file runs one
// pass per group instead of one pass for everything.
//
// Everything here degrades to today's single pass: a suite that does not know
// the flag, output that does not parse, or a partition that fails validation all
// fall back. Grouping must never make a job worse than not grouping.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	// listGroupsFlag is the flag a test suite advertises to describe its partition.
	listGroupsFlag = "-cws-list-groups"

	// groupNameFlag tells the suite which of its groups this pass is running, so
	// that it can fail when it builds more eBPF modules than the group predicted.
	groupNameFlag = "-cws-group"
)

// groupablePackages limits discovery to the packages that can plausibly support
// the flag, so the other suites are not launched just to have them reject it.
var groupablePackages = regexp.MustCompile(`^pkg/security(/|$)`)

// groupingEnvVar opts a job into grouping. It is off by default on purpose: the
// suite has package-level state (testMod, commonCfgDir), so splitting it into
// passes can expose an ordering dependency that today's accidental source order
// happens to satisfy. Run a pipeline with and without it on the same commit and
// diff the pass/fail sets before making it the default.
const groupingEnvVar = "CWS_TEST_GROUPING"

// safeTestName is what a name must look like to be spliced into a -test.run
// alternation without escaping.
var safeTestName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// testGroup is one pass of the test binary, as reported by the suite. Exactly
// one of Run and Skip is set: Run names the tests the pass executes, Skip names
// the ones it excludes (which is how the default group covers every test that
// declared nothing, including tests added after this code was written).
type testGroup struct {
	Name      string   `json:"name"`
	Signature string   `json:"signature"`
	Fields    []string `json:"fields"`
	Reasons   []string `json:"reasons"`
	Run       []string `json:"run"`
	Skip      []string `json:"skip"`
	Fresh     []string `json:"fresh"`

	ExpectedRebuilds int `json:"expectedRebuilds"`
}

// testPassPlan is one gotestsum invocation for a package: the filter it runs
// under, and the suffix that keeps its junit and testjson reports from
// colliding with the other passes'.
type testPassPlan struct {
	suffix string
	args   []string
}

// singlePass is the status quo: run the whole package once, with no filter of
// our own and no report suffix.
func singlePass() []testPassPlan {
	return []testPassPlan{{}}
}

// passPlans turns a partition into one pass per group. Each pass is told which
// group it is running so the suite can check the modules it built against what
// the group predicted.
func passPlans(groups []testGroup) []testPassPlan {
	plans := make([]testPassPlan, 0, len(groups))
	for _, g := range groups {
		args := append(g.testArgs(), groupNameFlag, g.Name)
		plans = append(plans, testPassPlan{suffix: "-" + g.Name, args: args})
	}
	return plans
}

// testArgs returns the -test.run / -test.skip arguments selecting this group.
// -test.run splits its pattern on '/' and applies each element to one level of
// the subtest path, so an anchored top-level name selects that test and all of
// its subtests.
func (g testGroup) testArgs() []string {
	if len(g.Run) > 0 {
		return []string{"-test.run", anchoredAlternation(g.Run)}
	}
	if len(g.Skip) > 0 {
		return []string{"-test.skip", anchoredAlternation(g.Skip)}
	}
	// A group with neither runs the whole suite, which is the fallback shape.
	return nil
}

func anchoredAlternation(names []string) string {
	return "^(" + strings.Join(names, "|") + ")$"
}

// discoverTestGroups asks a test suite how it wants to be partitioned. A nil
// slice means "run it as a single pass", which is never an error.
func discoverTestGroups(pkg string, suiteArgs, env []string, dir string) []testGroup {
	if !groupablePackages.MatchString(pkg) {
		return nil
	}
	if !groupingEnabled(env) {
		return nil
	}

	cmd := exec.Command(suiteArgs[0], append(append([]string{}, suiteArgs[1:]...), listGroupsFlag)...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// The overwhelmingly likely cause is a suite that predates the flag.
		fmt.Printf("[groups] %s does not support %s (%s), running it as a single pass\n",
			pkg, listGroupsFlag, err)
		return nil
	}

	groups, err := decodeTestGroups(&stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[groups] %s: cannot read the partition (%s), running it as a single pass\n", pkg, err)
		return nil
	}
	if err := validateTestGroups(groups); err != nil {
		fmt.Fprintf(os.Stderr, "[groups] %s: rejected the partition (%s), running it as a single pass\n", pkg, err)
		return nil
	}

	expected := 0
	for _, g := range groups {
		expected += g.ExpectedRebuilds
	}
	fmt.Printf("[groups] %s: %d passes, %d expected module rebuilds\n", pkg, len(groups), expected)
	for _, g := range groups {
		what := fmt.Sprintf("%d tests", len(g.Run))
		if len(g.Run) == 0 {
			what = fmt.Sprintf("everything except %d declared tests", len(g.Skip))
		}
		fmt.Printf("[groups]   %-24s rebuilds=%-3d %s %s\n",
			g.Name, g.ExpectedRebuilds, what, strings.Join(g.Fields, ","))
	}
	return groups
}

// groupingEnabled looks for the opt-in in the environment the test suite will
// run under, which is where a KMT job's additional_env_vars land, and falls back
// to the runner's own environment.
func groupingEnabled(env []string) bool {
	for _, kv := range env {
		if name, val, ok := strings.Cut(kv, "="); ok && name == groupingEnvVar {
			return val != "" && val != "0" && val != "false"
		}
	}
	val := os.Getenv(groupingEnvVar)
	return val != "" && val != "0" && val != "false"
}

func decodeTestGroups(r *bytes.Buffer) ([]testGroup, error) {
	var groups []testGroup
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var g testGroup
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&g); err != nil {
			return nil, fmt.Errorf("decode %q: %w", truncate(string(line)), err)
		}
		groups = append(groups, g)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, errors.New("no groups reported")
	}
	return groups, nil
}

// validateTestGroups checks the properties the passes have to have for the
// partition to be equivalent to a single run: every test lands in exactly one
// pass, and the pass that is expressed as a skip excludes precisely the tests
// the other passes claim.
func validateTestGroups(groups []testGroup) error {
	claimed := map[string]string{}
	var skipGroup *testGroup

	for i := range groups {
		g := &groups[i]
		if g.Name == "" {
			return fmt.Errorf("group %d has no name", i)
		}
		if len(g.Run) > 0 && len(g.Skip) > 0 {
			return fmt.Errorf("group %s sets both run and skip", g.Name)
		}
		if len(g.Run) == 0 && len(g.Skip) == 0 {
			return fmt.Errorf("group %s selects nothing", g.Name)
		}
		for _, name := range append(append([]string{}, g.Run...), g.Skip...) {
			if !safeTestName.MatchString(name) {
				return fmt.Errorf("group %s: %q is not a plain test name", g.Name, name)
			}
		}
		if len(g.Skip) > 0 {
			if skipGroup != nil {
				return fmt.Errorf("groups %s and %s are both expressed as a skip", skipGroup.Name, g.Name)
			}
			skipGroup = g
			continue
		}
		for _, name := range g.Run {
			if other, dup := claimed[name]; dup {
				return fmt.Errorf("%s is claimed by both %s and %s", name, other, g.Name)
			}
			claimed[name] = g.Name
		}
	}

	if skipGroup == nil {
		// Every pass is an explicit list, so tests added later would run in no
		// pass at all. Refuse: silently dropping tests is worse than not grouping.
		return errors.New("no group covers the tests that none of the others claim")
	}
	if len(skipGroup.Skip) != len(claimed) {
		return fmt.Errorf("%s skips %d tests but the other passes claim %d",
			skipGroup.Name, len(skipGroup.Skip), len(claimed))
	}
	for _, name := range skipGroup.Skip {
		if _, ok := claimed[name]; !ok {
			return fmt.Errorf("%s skips %s, which no pass claims", skipGroup.Name, name)
		}
	}
	return nil
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
