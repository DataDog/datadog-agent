// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oscap-io is the patched OpenSCAP engine the agent shells out to; we drive it directly
// for the cross-check.
const oscapIO = "/opt/datadog-agent/embedded/bin/oscap-io"

//go:embed testdata/golden
var goldenFS embed.FS

// oscapResults maps oscap-io's numeric result codes to the agent's result strings.
var oscapResults = map[string]string{
	"1": "passed", "2": "failed", "3": "error", "4": "error",
	"5": "skipped", "6": "notchecked", "7": "notselected",
}

// logGolden (below) logs a per-rule snapshot and, when a baseline is committed, its diff.
// On a pinned host it also gates: a rule flipping result on an unchanged rule set is an
// agent regression and fails, while a changed rule set is a content move to re-baseline.
// TestCrossCheck is a real assertion too: the agent and oscap-io share an engine, so any
// divergence is an agent Go-layer bug.

// snapshot renders a rule->result map as sorted "rule result" lines.
func snapshot(results map[string]string) string {
	lines := make([]string, 0, len(results))
	for rule, res := range results {
		lines = append(lines, rule+" "+res)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// goldenBaseline returns the committed rule->result baseline for a framework, if present.
func goldenBaseline(frameworkID string) (map[string]string, bool) {
	data, err := goldenFS.ReadFile("testdata/golden/" + frameworkID + ".txt")
	if err != nil {
		return nil, false
	}
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			m[f[0]] = f[1]
		}
	}
	return m, true
}

// goldenDiff classifies how results differ from the committed baseline. A rule present on
// only one side (added or removed) means the rule set changed, which happens when the
// benchmark content moves (a policy bump). A rule present in both with a different result
// is a flip, mapped to "was -> now".
func goldenDiff(base, results map[string]string) (added, removed []string, flips map[string]string) {
	flips = map[string]string{}
	for rule, b := range base {
		switch r, ok := results[rule]; {
		case !ok:
			removed = append(removed, rule)
		case r != b:
			flips[rule] = b + " -> " + r
		}
	}
	for rule := range results {
		if _, ok := base[rule]; !ok {
			added = append(added, rule)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed, flips
}

// timeSensitiveRules are excluded from the golden gate because their result can change with
// the calendar even on a pinned host (for example a rule comparing an account or certificate
// age against the current date). None are known yet.
var timeSensitiveRules = map[string]struct{}{}

// writeArtifact saves diagnostic output to the suite output dir for CI to upload.
func writeArtifact(t *testing.T, outDir, name, content string) {
	if err := os.WriteFile(filepath.Join(outDir, name), []byte(content+"\n"), 0o644); err != nil {
		t.Logf("could not write %s: %v", name, err)
	}
}

// logGolden logs the per-rule result snapshot and, when a baseline is committed for this
// framework+environment, its diff. The host and container variants of a framework keep
// separate baselines (cis-x.txt vs cis-x-container.txt) since their results differ.
//
// When gate is set the golden also acts as a regression check. On a pinned host with an
// unchanged rule set a flipped result is an agent regression and fails. A changed rule set
// instead means the benchmark content moved (a policy bump), which is logged as a
// re-baseline prompt rather than a failure. Callers gate only where the result is
// deterministic: host on a pinned AMI. AlmaLinux (latest AMI) and the container variants
// stay informational.
func logGolden(t *testing.T, d distro, env, outDir string, results map[string]string, gate bool) {
	key := d.frameworkID
	if env != "host" {
		key += "-" + env
	}
	snap := snapshot(results)
	t.Logf("%s (%s) golden snapshot (%d rules):\n%s", d.frameworkID, env, len(results), snap)
	writeArtifact(t, outDir, "golden-"+key+".txt", snap)
	base, ok := goldenBaseline(key)
	if !ok {
		t.Logf("%s (%s) golden: no committed baseline yet (snapshot saved as artifact)", d.frameworkID, env)
		return
	}

	added, removed, flips := goldenDiff(base, results)
	if len(added)+len(removed)+len(flips) == 0 {
		t.Logf("%s (%s) golden: identical to baseline", d.frameworkID, env)
		return
	}
	var diff []string
	for _, r := range removed {
		diff = append(diff, "- "+r+" "+base[r])
	}
	for _, r := range added {
		diff = append(diff, "+ "+r+" "+results[r])
	}
	for rule, change := range flips {
		diff = append(diff, "~ "+rule+" "+change)
	}
	sort.Strings(diff)
	t.Logf("%s (%s) golden diff vs baseline (- removed, + added, ~ changed):\n%s", d.frameworkID, env, strings.Join(diff, "\n"))

	if !gate {
		return
	}
	// A changed rule set is a content move (policy bump), not a regression: prompt a
	// re-baseline and stop, so an expected content change does not read as a failure.
	if len(added)+len(removed) > 0 {
		t.Logf("%s (%s) golden: rule set changed (%d added, %d removed) — re-baseline testdata/golden/%s.txt (not treated as a regression)",
			d.frameworkID, env, len(added), len(removed), key)
		return
	}
	// Same rule set, so a flipped result on this pinned host is an agent regression.
	var regressions []string
	for rule, change := range flips {
		if _, skip := timeSensitiveRules[rule]; skip {
			continue
		}
		regressions = append(regressions, rule+" "+change)
	}
	sort.Strings(regressions)
	assert.Emptyf(t, regressions, "%s golden: %d rule(s) flipped result on a pinned host with an unchanged rule set — agent regression:\n%s",
		d.frameworkID, len(regressions), strings.Join(regressions, "\n"))
}

// benchmarkInput extracts the datastream path and XCCDF profile the agent uses for a
// framework, by reading the bundled benchmark YAML. read is the command that reads the
// files ("sudo cat" on the host, "cat" in a container running as the agent user).
func benchmarkInput(run func(string) string, read, frameworkID string) (datastream, profile string) {
	const dir = "/etc/datadog-agent/compliance.d"
	yaml := run(fmt.Sprintf("%s %s/%s*.yaml 2>/dev/null", read, dir, frameworkID))
	if name := regexp.MustCompile(`ssg-[a-z0-9._-]+-ds\.xml(\.bz2)?`).FindString(yaml); name != "" {
		datastream = dir + "/" + name
	}
	profile = regexp.MustCompile(`xccdf_org\.ssgproject\.content_profile_[a-z0-9_]+`).FindString(yaml)
	return datastream, profile
}

// runOscapIO evaluates rules with the bundled oscap-io directly (bypassing the agent's Go
// layer) and returns rule->result. invoke runs oscap-io in the target ("sudo <oscap-io>" on
// the host, "OSCAP_PROBE_ROOT=/host <oscap-io>" in a container scanning the mounted host
// root, the same probe root the agent sets). Returns nil if oscap-io cannot be driven.
func runOscapIO(run func(string) string, invoke, datastream, profile string, rules []string) map[string]string {
	if datastream == "" || profile == "" || len(rules) == 0 {
		return nil
	}
	var stdin strings.Builder
	for _, r := range rules {
		fmt.Fprintf(&stdin, "%s %s\n", profile, r)
	}
	out := run(fmt.Sprintf("%s %s 2>/dev/null <<'OSCAPEOF'\n%sOSCAPEOF", invoke, datastream, stdin.String()))
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) == 2 {
			if res, ok := oscapResults[f[1]]; ok {
				m[f[0]] = res
			}
		}
	}
	return m
}

// TestGolden compares the per-rule results to the committed baseline and, on a pinned host,
// gates on regressions (see logGolden). AlmaLinux resolves the latest AMI, so its results
// drift with the OS and it stays informational.
func (s *hostBenchmarksSuite) TestGolden() {
	logGolden(s.T(), s.distro, "host", s.SessionOutputDir(), resultsByRule(s.distro, s.check), !s.distro.latestAMI)
}

// TestGolden logs the per-rule result snapshot and baseline diff. Container baselines are
// not committed, so this stays informational.
func (s *containerBenchmarksSuite) TestGolden() {
	logGolden(s.T(), s.distro, "container", s.SessionOutputDir(), resultsByRule(s.distro, s.check), false)
}

// assertCrossCheck drives oscap-io directly and asserts the agent's per-rule results match
// it. read reads the bundled benchmark files ("sudo cat" on the host, "cat" in a container),
// and invoke runs oscap-io with the right probe root. The agent and oscap-io share an engine,
// so any divergence is an agent Go-layer bug (rule selection, result mapping, probe-root
// handling) and is fatal. Failing to drive oscap-io at all is also fatal: it ships with the
// agent, so that signals a real breakage, not an environment quirk to skip silently.
func assertCrossCheck(t *testing.T, d distro, outDir string, agent map[string]string, run func(string) string, read, invoke string) {
	rules := make([]string, 0, len(agent))
	for rule := range agent {
		rules = append(rules, rule)
	}
	datastream, profile := benchmarkInput(run, read, d.frameworkID)
	oscap := runOscapIO(run, invoke, datastream, profile, rules)
	require.GreaterOrEqualf(t, len(oscap), d.minRules,
		"oscap-io cross-check could not run: evaluated %d rules (datastream=%q profile=%q)",
		len(oscap), datastream, profile)

	var diffs []string
	for rule, a := range agent {
		if o, ok := oscap[rule]; ok && o != a {
			diffs = append(diffs, fmt.Sprintf("%s agent=%s oscap=%s", rule, a, o))
		}
	}
	sort.Strings(diffs)
	writeArtifact(t, outDir, "oscap-crosscheck-"+d.frameworkID+".txt", strings.Join(diffs, "\n"))
	assert.Emptyf(t, diffs, "%s: agent disagrees with oscap-io on %d rules (same engine, so this is an agent Go-layer bug):\n%s",
		d.frameworkID, len(diffs), strings.Join(diffs, "\n"))
	if len(diffs) == 0 {
		t.Logf("%s oscap cross-check: agent matches oscap-io on all %d rules", d.frameworkID, len(oscap))
	}
}

// TestCrossCheck asserts the host agent's results match the bundled oscap-io run directly.
func (s *hostBenchmarksSuite) TestCrossCheck() {
	run := func(cmd string) string { out, _ := s.Env().RemoteHost.Execute(cmd); return out }
	assertCrossCheck(s.T(), s.distro, s.SessionOutputDir(), resultsByRule(s.distro, s.check), run, "sudo cat", "sudo "+oscapIO)
}

// TestCrossCheck asserts the containerized agent matches the bundled oscap-io run directly
// inside the container against the mounted host root, using OSCAP_PROBE_ROOT=/host (the same
// probe root the agent sets), catching a container-only Go-layer divergence.
func (s *containerBenchmarksSuite) TestCrossCheck() {
	run := func(cmd string) string {
		return s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", cmd)
	}
	assertCrossCheck(s.T(), s.distro, s.SessionOutputDir(), resultsByRule(s.distro, s.check), run, "cat", "OSCAP_PROBE_ROOT=/host "+oscapIO)
}
