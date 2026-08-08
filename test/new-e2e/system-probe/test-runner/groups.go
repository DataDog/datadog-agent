// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

// The CWS functional suite rebuilds its eBPF manager whenever a test needs a
// different static config than the live one has, which costs seconds to tens of
// seconds. Tests sharing a config are scattered through source order, and Go
// cannot reorder m.Run(), so the suite reports how it wants to be partitioned
// (-list-groups, see pkg/security/tests/testruns.go) and this file runs one
// pass per group.
//
// Anything unexpected falls back to a single pass: grouping must never make a
// job worse than not grouping.

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

// listGroupsFlag is the flag a test suite advertises to describe its partition.
const listGroupsFlag = "-list-groups"

// groupablePackages keeps the other suites from being launched just to have
// them reject the flag; a suite that grows support for it adds itself here.
// KMT deploys the CWS suite as pkg/security, not pkg/security/tests
// (tasks/kmt.py, kmt_secagent_prepare).
var groupablePackages = regexp.MustCompile(`^pkg/security(/|$)`)

// testGroup is one pass of the test binary, as reported by the suite. Exactly
// one of Run and Skip is set; the skip form is how the default group covers
// every test that declared nothing, including ones added later. A group with
// neither, i.e. the zero value, is the unfiltered fallback pass.
type testGroup struct {
	Name      string   `json:"name"`
	Signature string   `json:"signature"`
	Fields    []string `json:"fields"`
	Reasons   []string `json:"reasons"`
	Run       []string `json:"run"`
	Skip      []string `json:"skip"`

	ExpectedRebuilds int `json:"expectedRebuilds"`
}

// runConfig expresses a group as the per-package filter buildCommandArgs
// already applies, so a pass needs no argument-building of its own.
func (g testGroup) runConfig() packageRunConfiguration {
	return packageRunConfiguration{RunOnly: anchored(g.Run), Skip: anchored(g.Skip)}
}

// anchored builds the alternation as a single element, since buildCommandArgs
// joins with '|' and the anchoring has to end up inside it. The anchoring
// matters: -test.run matches unanchored, so a bare TestMount would drag in
// TestMountEvent. It cannot move into buildCommandArgs, whose job-configured
// filters rely on staying unanchored -- cws_ad's "TestActivityDump" is a prefix
// of the tests it selects, not one of their names.
func anchored(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	return []string{"^(" + strings.Join(names, "|") + ")$"}
}

// discoverTestGroups asks a suite how it wants to be partitioned. A nil slice
// means "run it as a single pass", which is never an error.
func discoverTestGroups(pkg string, suiteArgs, env []string, dir string) []testGroup {
	if !groupablePackages.MatchString(pkg) {
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
		fmt.Fprintf(os.Stderr, "[groups] could not list groups for %s (%s), running it as a single pass\n%s",
			pkg, err, stderr.String())
		return nil
	}

	groups, err := decodeTestGroups(&stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[groups] %s: cannot read the partition (%s), running it as a single pass\n", pkg, err)
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

func decodeTestGroups(r *bytes.Buffer) ([]testGroup, error) {
	var groups []testGroup
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("{")) {
			// Only parse what looks like JSON: the suite shares stdout with
			// anything that logs during init(), before the logger is
			// configured and before -list-groups can exit.
			continue
		}
		// Unknown fields are ignored so that a field added suite-side does not
		// make an older runner reject the partition and silently fall back.
		var g testGroup
		if err := json.Unmarshal(line, &g); err != nil {
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

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
