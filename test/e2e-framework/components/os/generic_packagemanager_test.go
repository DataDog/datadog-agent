// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package os

import "testing"

func TestEnsureCommandWithCheckedBinaryDoesNotRequireAnUnconditionalUpdate(t *testing.T) {
	manager := &GenericPackageManager{
		installCmd: "apt-get install -y",
		updateCmd:  "apt-get update -y",
	}

	got, needsUpdate := manager.ensureCommand("curl", "curl")
	want := "bash -c 'command -v curl || (apt-get update -y && apt-get install -y curl)'"
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if needsUpdate {
		t.Fatal("a checked binary must not create an unconditional update resource")
	}
}

func TestEnsureCommandWithoutBinaryCheckUpdatesBeforeInstalling(t *testing.T) {
	manager := &GenericPackageManager{
		installCmd: "apt-get install -y",
		updateCmd:  "apt-get update -y",
	}

	got, needsUpdate := manager.ensureCommand("package-without-binary", "")
	if got != "apt-get install -y package-without-binary" {
		t.Fatalf("command = %q", got)
	}
	if !needsUpdate {
		t.Fatal("an unchecked package must refresh the package database before installing")
	}
}
