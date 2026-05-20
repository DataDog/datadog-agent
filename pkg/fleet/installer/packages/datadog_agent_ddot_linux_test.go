// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import "testing"

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

func TestShouldRemoveProcmgrDDOTMarkerOnExtensionRemove(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		packageType PackageType
		packagePath string
		want        bool
	}{
		{"oci_stable", PackageTypeOCI, "/opt/datadog-packages/datadog-agent/stable", true},
		{"deb", PackageTypeDEB, "/opt/datadog-agent/1.2.3", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := HookContext{PackageType: tc.packageType, PackagePath: tc.packagePath}
			if got := shouldRemoveProcmgrDDOTMarkerOnExtensionRemove(ctx); got != tc.want {
				t.Fatalf("shouldRemoveProcmgrDDOTMarkerOnExtensionRemove() = %v, want %v", got, tc.want)
			}
		})
	}
}
