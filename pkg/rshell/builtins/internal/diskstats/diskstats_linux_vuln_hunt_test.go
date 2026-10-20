// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package diskstats

import (
	"strings"
	"testing"
)

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func TestVulnHuntSubsystemDfMountEnumeration_MountInfoPathHardcoded(t *testing.T) {
	if mountInfoPath != "/proc/self/mountinfo" {
		t.Fatalf("mountInfoPath = %q, want /proc/self/mountinfo", mountInfoPath)
	}
	if strings.Contains(mountInfoPath, "..") {
		t.Fatalf("mountInfoPath must not contain traversal components: %q", mountInfoPath)
	}
}
