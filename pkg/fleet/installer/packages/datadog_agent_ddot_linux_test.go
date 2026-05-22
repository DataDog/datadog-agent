// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDdotExtensionProcmgrRemoveStable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		packageType PackageType
		packagePath string
		want        bool
	}{
		{"oci_stable", PackageTypeOCI, "/opt/datadog-packages/datadog-agent/stable", true},
		{"oci_experiment", PackageTypeOCI, "/opt/datadog-packages/datadog-agent/experiment", false},
		{"deb", PackageTypeDEB, "/opt/datadog-agent/1.2.3", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := HookContext{PackageType: tc.packageType, PackagePath: tc.packagePath}
			if got := ddotExtensionProcmgrRemoveStable(ctx); got != tc.want {
				t.Fatalf("ddotExtensionProcmgrRemoveStable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOciAgentStableAndExperimentProcessesDirsEquivalentAt(t *testing.T) {
	t.Run("missing_stable_not_error", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		expDir := filepath.Join(base, "datadog-agent", "experiment", "processes.d")
		if err := os.MkdirAll(expDir, 0o755); err != nil {
			t.Fatal(err)
		}
		equiv, err := ociAgentStableAndExperimentProcessesDirsEquivalentAt(base)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if equiv {
			t.Fatal("expected not equivalent when stable processes.d is absent")
		}
	})

	t.Run("same_directory", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		stableDir := filepath.Join(base, "datadog-agent", "stable", "processes.d")
		expDir := filepath.Join(base, "datadog-agent", "experiment", "processes.d")
		shared := filepath.Join(base, "shared", "processes.d")
		if err := os.MkdirAll(shared, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(stableDir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(expDir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, stableDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shared, expDir); err != nil {
			t.Fatal(err)
		}
		equiv, err := ociAgentStableAndExperimentProcessesDirsEquivalentAt(base)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !equiv {
			t.Fatal("expected equivalent when both channels resolve to the same processes.d")
		}
	})
}
