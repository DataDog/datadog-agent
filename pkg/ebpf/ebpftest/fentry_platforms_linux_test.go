// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpftest

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// kmtPlatform identifies a KMT platform by the properties observable from
// inside its microVM: the /etc/os-release ID and VERSION_ID, the
// architecture (uname -m), plus the major.minor of the kernel release
// (uname -r) when ID+VERSION_ID alone is ambiguous (Amazon Linux 2 ships
// 4.14, 5.4 and 5.10 kernels; AL2023 ships 6.1 and 6.12).
type kmtPlatform struct {
	name            string // platform name in test/new-e2e/system-probe/config/platforms.json
	osID            string // /etc/os-release ID
	osVersionID     string // /etc/os-release VERSION_ID
	arch            string // KMT architecture (x86_64 or arm64); empty matches all architectures
	kernel          string // uname release major.minor; empty matches any
	fentrySupported bool   // whether the fentry support check is expected to succeed
}

// kmtPlatforms enumerates the KMT platforms the system-probe test jobs run
// on: the union of the TAG matrices of kmt_run_sysprobe_tests_x64 and
// kmt_run_sysprobe_tests_arm64 in
// .gitlab/test/kernel_matrix_testing/system_probe.yml, with the fentry
// expectation established empirically for each. Platforms without a row are
// assumed fentry-supported, so a new platform only fails the test when the
// probe reports unsupported there, forcing an explicit row. When fentry
// support legitimately changes on a platform
// (e.g. a kernel image refresh), update the corresponding row in the same
// commit. Platforms whose support differs per architecture need one row per
// architecture.
var kmtPlatforms = []kmtPlatform{
	{name: "ubuntu_16.04", osID: "ubuntu", osVersionID: "16.04"},
	{name: "ubuntu_18.04", osID: "ubuntu", osVersionID: "18.04"},
	{name: "ubuntu_20.04", osID: "ubuntu", osVersionID: "20.04"},
	{name: "ubuntu_22.04", osID: "ubuntu", osVersionID: "22.04", arch: "x86_64", fentrySupported: true},
	{name: "ubuntu_22.04", osID: "ubuntu", osVersionID: "22.04", arch: "arm64"},
	{name: "ubuntu_24.04", osID: "ubuntu", osVersionID: "24.04"},
	{name: "ubuntu_25.10", osID: "ubuntu", osVersionID: "25.10", fentrySupported: true},
	{name: "ubuntu_26.04", osID: "ubuntu", osVersionID: "26.04", fentrySupported: true},
	{name: "amazon_4.14", osID: "amzn", osVersionID: "2", kernel: "4.14"},
	{name: "amazon_5.4", osID: "amzn", osVersionID: "2", kernel: "5.4"},
	{name: "amazon_5.10", osID: "amzn", osVersionID: "2", arch: "x86_64", kernel: "5.10", fentrySupported: true},
	{name: "amazon_5.10", osID: "amzn", osVersionID: "2", arch: "arm64", kernel: "5.10"},
	{name: "amazon_2023", osID: "amzn", osVersionID: "2023", kernel: "6.1"},
	{name: "amazon_6.12", osID: "amzn", osVersionID: "2023", kernel: "6.12", fentrySupported: true},
	{name: "fedora_37", osID: "fedora", osVersionID: "37"},
	{name: "fedora_38", osID: "fedora", osVersionID: "38"},
	{name: "debian_9", osID: "debian", osVersionID: "9"},
	{name: "debian_10", osID: "debian", osVersionID: "10"},
	{name: "debian_11", osID: "debian", osVersionID: "11", arch: "x86_64", fentrySupported: true},
	{name: "debian_11", osID: "debian", osVersionID: "11", arch: "arm64"},
	{name: "debian_12", osID: "debian", osVersionID: "12"},
	{name: "centos_7.9", osID: "centos", osVersionID: "7"},
	{name: "centos_8", osID: "centos", osVersionID: "8"},
	{name: "rocky_8.4", osID: "rocky", osVersionID: "8.4"},
	{name: "rocky_9.4", osID: "rocky", osVersionID: "9.4"},
}

// kmtArch maps the uname machine name to the KMT architecture naming used in
// platforms.json.
func kmtArch(machine string) string {
	if machine == "aarch64" {
		return "arm64"
	}
	return machine
}

// osReleaseID returns the ID and VERSION_ID fields from /etc/os-release.
func osReleaseID(tb testing.TB) (string, string) {
	tb.Helper()
	data, err := os.ReadFile("/etc/os-release")
	require.NoError(tb, err)
	vars := make(map[string]string)
	for line := range strings.Lines(string(data)) {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vars[key] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return vars["ID"], vars["VERSION_ID"]
}

// kernelMajorMinor extracts the major.minor prefix from a kernel release
// string (e.g. "5.10.244-240.970.amzn2" -> "5.10"), or "" if the release does
// not start with a numeric version.
func kernelMajorMinor(release string) string {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// expectedFentrySupport returns the fentry expectation for the platform the
// test is running on, along with its KMT platform name ("" when no row
// matches). Platforms missing from kmtPlatforms are assumed fentry-supported:
// the test fails on such a platform only when the probe reports unsupported,
// which forces an explicit row to be added.
func expectedFentrySupport(tb testing.TB) (bool, string) {
	tb.Helper()
	osID, osVersionID := osReleaseID(tb)
	release, err := kernel.Release()
	require.NoError(tb, err)
	machine, err := kernel.Machine()
	require.NoError(tb, err)
	arch := kmtArch(machine)
	majorMinor := kernelMajorMinor(release)

	matched := -1
	for i, p := range kmtPlatforms {
		if p.osID != osID || p.osVersionID != osVersionID {
			continue
		}
		if p.kernel != "" && p.kernel != majorMinor {
			continue
		}
		if p.arch != "" && p.arch != arch {
			continue
		}
		if matched >= 0 {
			require.FailNowf(tb, "ambiguous KMT platform match", "os-release ID=%q VERSION_ID=%q kernel=%q arch=%q matches both %q and %q", osID, osVersionID, release, arch, kmtPlatforms[matched].name, p.name)
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
	// The expectations only hold on KMT microVMs, which boot the exact distro
	// kernel pinned in platforms.json. On any other host (developer machines,
	// containers) the same os-release can pair with a different kernel, so
	// skip rather than risk a spurious verdict. KMT CI runs propagate
	// GITLAB_CI through /job_env.txt (see TestJobEnv).
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
		osID, osVersionID, arch, kernel string
	}
	seen := make(map[rowKey]string)
	for _, p := range kmtPlatforms {
		assert.Containsf(t, []string{"", "x86_64", "arm64"}, p.arch, "platform %q has invalid arch %q", p.name, p.arch)
		key := rowKey{p.osID, p.osVersionID, p.arch, p.kernel}
		if prev, ok := seen[key]; ok {
			require.Failf(t, "duplicate rows in kmtPlatforms", "rows %q and %q match the same platform", prev, p.name)
		}
		seen[key] = p.name
	}
}
