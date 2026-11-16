// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpftest

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kmtPlatform maps a KMT platform, identified by its name in the CI job name
// (e.g. kmt_run_sysprobe_tests_x64: [ubuntu_22.04, no_usm]), to the expected
// result of the fentry support check on it.
type kmtPlatform struct {
	name            string // platform name as it appears in the KMT CI job names (TAG matrix value)
	arch            string // KMT architecture (x86_64 or arm64); empty matches all architectures
	fentrySupported bool   // whether the fentry support check is expected to succeed
}

// kmtPlatforms enumerates the KMT platforms the system-probe test jobs run
// on: the union of the TAG matrices of kmt_run_sysprobe_tests_x64 and
// kmt_run_sysprobe_tests_arm64 in
// .gitlab/test/kernel_matrix_testing/system_probe.yml, with the fentry
// expectation established empirically for each. Platforms without a row are
// assumed fentry-supported, so a new platform only fails the test when the
// probe reports unsupported there, forcing an explicit row. When fentry
// support legitimately changes on a platform (e.g. a kernel image refresh),
// update the corresponding row in the same commit. Platforms whose support
// differs per architecture need one row per architecture.
var kmtPlatforms = []kmtPlatform{
	{name: "ubuntu_16.04"},
	{name: "ubuntu_18.04"},
	{name: "ubuntu_20.04"},
	{name: "ubuntu_22.04", arch: "x86_64", fentrySupported: true},
	{name: "ubuntu_22.04", arch: "arm64"},
	{name: "ubuntu_24.04"},
	{name: "ubuntu_25.10", fentrySupported: true},
	{name: "ubuntu_26.04", fentrySupported: true},
	{name: "amazon_4.14"},
	{name: "amazon_5.4"},
	{name: "amazon_5.10", arch: "x86_64", fentrySupported: true},
	{name: "amazon_5.10", arch: "arm64"},
	{name: "amazon_2023"},
	{name: "amazon_6.12", fentrySupported: true},
	{name: "fedora_37"},
	{name: "fedora_38"},
	{name: "debian_9"},
	{name: "debian_10"},
	{name: "debian_11", arch: "x86_64", fentrySupported: true},
	{name: "debian_11", arch: "arm64"},
	{name: "debian_12"},
	{name: "centos_7.9"},
	{name: "centos_8"},
	{name: "rocky_8.4"},
	{name: "rocky_9.4"},
}

// expectedFentrySupport returns the fentry expectation for the platform the
// test job is running on, along with its KMT platform name ("" when no row
// matches). The KMT CI job name embeds both the architecture and the
// platform, e.g. "kmt_run_sysprobe_tests_x64: [ubuntu_22.04, no_usm]".
// Platforms missing from kmtPlatforms are assumed fentry-supported: the test
// fails on such a platform only when the probe reports unsupported, which
// forces an explicit row to be added.
func expectedFentrySupport(tb testing.TB) (bool, string) {
	tb.Helper()
	jobName := os.Getenv("CI_JOB_NAME")
	require.NotEmpty(tb, jobName, "CI_JOB_NAME is not set; KMT CI propagates it through /job_env.txt (see TestJobEnv)")

	var arch string
	switch {
	case strings.Contains(jobName, "_x64"):
		arch = "x86_64"
	case strings.Contains(jobName, "_arm64"):
		arch = "arm64"
	}

	matched := -1
	for i, p := range kmtPlatforms {
		if !strings.Contains(jobName, p.name) {
			continue
		}
		if p.arch != "" && p.arch != arch {
			continue
		}
		if matched >= 0 {
			require.FailNowf(tb, "ambiguous KMT platform match", "job name %q matches both %q and %q", jobName, kmtPlatforms[matched].name, p.name)
		}
		matched = i
	}
	if matched < 0 {
		return true, ""
	}
	p := kmtPlatforms[matched]
	return p.fentrySupported, p.name
}

// TestFentrySupportByKMTPlatform asserts the fentry support check against the
// hard-coded per-platform expectation recorded in kmtPlatforms.
// The check itself (supportsFentry -> features.SupportsFentry) stays a
// generic "does fentry work on this kernel" probe; this test pins the
// CNM-level expectation per KMT platform so that a support change on any
// platform surfaces as a single clear failure here instead of cascading
// through every fentry-buildmode test (see CNM-5506).
func TestFentrySupportByKMTPlatform(t *testing.T) {
	// The expectations are keyed on the KMT CI job name, which only exists in
	// CI (propagated through /job_env.txt, see TestJobEnv); skip anywhere else.
	if os.Getenv("GITLAB_CI") != "true" {
		t.Skip("fentry platform expectations only apply to KMT CI runs")
	}

	expected, platform := expectedFentrySupport(t)
	if platform == "" {
		platform = "<unrecognized, assumed fentry-supported>"
	}
	assert.Equalf(t, expected, supportsFentry(),
		"fentry support on %s does not match the hard-coded expectation; "+
			"if fentry support on this platform legitimately changed, update its row in kmtPlatforms, "+
			"or add a row if the platform is new",
		platform)
}

// TestFentryPlatformExpectationsWellFormed validates the hard-coded table
// without any runtime probing, so it can catch typos on any host.
func TestFentryPlatformExpectationsWellFormed(t *testing.T) {
	type rowKey struct {
		name, arch string
	}
	seen := make(map[rowKey]struct{})
	for _, p := range kmtPlatforms {
		assert.Containsf(t, []string{"", "x86_64", "arm64"}, p.arch, "platform %q has invalid arch %q", p.name, p.arch)
		key := rowKey{p.name, p.arch}
		if _, ok := seen[key]; ok {
			require.Failf(t, "duplicate row in kmtPlatforms", "platform %q arch %q has multiple rows", p.name, p.arch)
		}
		seen[key] = struct{}{}
	}
	// Job-name matching is by substring, so no platform name may be a
	// substring of another platform's name.
	for i, p := range kmtPlatforms {
		for j, q := range kmtPlatforms {
			if i != j && p.name != q.name {
				require.Falsef(t, strings.Contains(q.name, p.name), "platform name %q is a substring of %q; job-name matching would be ambiguous", p.name, q.name)
			}
		}
	}
}
