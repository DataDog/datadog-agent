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
)

// kmtPlatform identifies a KMT platform test job by the platform name as it
// appears in the CI job name (e.g. kmt_run_sysprobe_tests_x64:
// [ubuntu_22.04, no_usm]) and, when the expectation differs per architecture,
// the KMT architecture (x86_64 or arm64; empty matches all architectures).
type kmtPlatform struct {
	name string
	arch string
}

// kmtPlatforms records, per KMT platform, the expected result of the fentry
// support check: the union of the TAG matrices of kmt_run_sysprobe_tests_x64
// and kmt_run_sysprobe_tests_arm64 in
// .gitlab/test/kernel_matrix_testing/system_probe.yml, with the expectation
// established empirically for each. Platforms without an entry are assumed
// fentry-supported, so a new platform only fails the test when the probe
// reports unsupported there, forcing an explicit entry. When fentry support
// legitimately changes on a platform (e.g. a kernel image refresh), update
// the corresponding entry in the same commit. Platforms whose support
// differs per architecture need one entry per architecture.
var kmtPlatforms = map[kmtPlatform]bool{
	{name: "ubuntu_16.04"}:                 false,
	{name: "ubuntu_18.04"}:                 false,
	{name: "ubuntu_20.04"}:                 false,
	{name: "ubuntu_22.04", arch: "x86_64"}: true,
	{name: "ubuntu_22.04", arch: "arm64"}:  false,
	{name: "ubuntu_24.04"}:                 false,
	{name: "ubuntu_25.10"}:                 true,
	{name: "ubuntu_26.04"}:                 true,
	{name: "amazon_4.14"}:                  false,
	{name: "amazon_5.4"}:                   false,
	{name: "amazon_5.10", arch: "x86_64"}:  true,
	{name: "amazon_5.10", arch: "arm64"}:   false,
	{name: "amazon_2023"}:                  false,
	{name: "amazon_6.12"}:                  true,
	{name: "fedora_37"}:                    false,
	{name: "fedora_38"}:                    false,
	{name: "debian_9"}:                     false,
	{name: "debian_10"}:                    false,
	{name: "debian_11", arch: "x86_64"}:    true,
	{name: "debian_11", arch: "arm64"}:     false,
	{name: "debian_12"}:                    false,
	{name: "centos_7.9"}:                   false,
	{name: "centos_8"}:                     false,
	{name: "rocky_8.4"}:                    false,
	{name: "rocky_9.4"}:                    false,
}

// expectedFentrySupport returns the fentry expectation for the platform named
// in the KMT CI job name (e.g. "kmt_run_sysprobe_tests_x64: [ubuntu_22.04,
// no_usm]"), along with the platform name ("" when no entry matches).
// Platforms missing from kmtPlatforms are assumed fentry-supported: the test
// fails on such a platform only when the probe reports unsupported, which
// forces an explicit entry to be added.
func expectedFentrySupport(jobName string) (bool, string) {
	var arch string
	switch {
	case strings.Contains(jobName, "_x64"):
		arch = "x86_64"
	case strings.Contains(jobName, "_arm64"):
		arch = "arm64"
	}

	matched := false
	supported := false
	name := ""
	for key, expectation := range kmtPlatforms {
		if !strings.Contains(jobName, key.name) {
			continue
		}
		if key.arch != "" && key.arch != arch {
			continue
		}
		// an arch-specific entry wins over a generic one
		if !matched || key.arch != "" {
			matched, supported, name = true, expectation, key.name
		}
	}
	if !matched {
		return true, ""
	}
	return supported, name
}

// TestFentrySupportByKMTPlatform asserts the fentry support check against the
// hard-coded per-platform expectation recorded in kmtPlatforms.
// The check itself (supportsFentry) is the gate SupportedBuildModes uses to
// decide whether the fentry build mode runs; this test pins its result per
// KMT platform so that a support change on any platform surfaces as a single
// clear failure here, instead of fentry subtests silently disappearing or
// failing in bulk across every fentry-buildmode test.
func TestFentrySupportByKMTPlatform(t *testing.T) {
	// CI_JOB_NAME is only propagated to the KMT test environment in CI (via
	// /job_env.txt, see TestJobEnv); everywhere else there is no KMT platform
	// to pin an expectation for.
	jobName := os.Getenv("CI_JOB_NAME")
	if !strings.Contains(jobName, "kmt_run_") {
		t.Skip("fentry platform expectations only apply to KMT CI test jobs")
	}

	expected, platform := expectedFentrySupport(jobName)
	if platform == "" {
		platform = "<unrecognized, assumed fentry-supported>"
	}
	assert.Equalf(t, expected, supportsFentry(),
		"fentry support on %s does not match the hard-coded expectation; "+
			"if fentry support on this platform legitimately changed, update its entry in kmtPlatforms, "+
			"or add an entry if the platform is new",
		platform)
}
