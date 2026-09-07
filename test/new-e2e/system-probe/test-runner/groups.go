// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

// The CWS suite rebuilds its eBPF module whenever a test needs a different
// static config than the live one, costing seconds to tens of seconds. Go
// cannot reorder m.Run(), so the suite groups its tests by config
// (pkg/security/tests/testruns.go) and this file runs one pass per group.
// Anything unexpected falls back to a single pass.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

const (
	listGroupsFlag = "-list-groups"
	groupFlag      = "-group"

	// groupLinePrefix is what the suite marks its group names with, since it
	// shares stdout with anything logged during init().
	groupLinePrefix = "testgroup "
)

// groupablePackages are the suites supporting listGroupsFlag; the others are
// not launched just to reject it. KMT deploys CWS as pkg/security, not
// pkg/security/tests (tasks/kmt.py, kmt_secagent_prepare).
var groupablePackages = regexp.MustCompile(`^pkg/security(/|$)`)

// discoverTestGroups asks a suite how it wants to be partitioned. A nil slice
// means a single pass, and is never an error.
func discoverTestGroups(pkg string, suiteArgs, env []string, dir string) []string {
	if !groupablePackages.MatchString(pkg) {
		return nil
	}

	cmd := exec.Command(suiteArgs[0], append(slices.Clone(suiteArgs[1:]), listGroupsFlag)...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[groups] could not list groups for %s (%s), running a single pass\n%s",
			pkg, err, stderr.String())
		return nil
	}

	var groups []string
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), groupLinePrefix); ok {
			groups = append(groups, name)
		}
	}
	fmt.Printf("[groups] %s: %d passes %v\n", pkg, len(groups), groups)
	return groups
}

// testPasses returns the passes to run a suite in: one per group it reports,
// or the single nameless pass, which runs everything the job selects.
func testPasses(pkg string, suiteArgs, env []string, dir string) []string {
	if groups := discoverTestGroups(pkg, suiteArgs, env, dir); len(groups) > 0 {
		return groups
	}
	return []string{""}
}

// groupPass returns what one pass changes about the command: the suffix that
// keeps its reports from clobbering the other passes', since the paths derive
// from the package alone, and the suite args selecting the group.
func groupPass(suiteArgs []string, group string) (string, []string) {
	if group == "" {
		return "", suiteArgs
	}
	// Clone: appending in place would carry the group over to the next pass.
	return "-" + group, append(slices.Clone(suiteArgs), groupFlag+"="+group)
}
