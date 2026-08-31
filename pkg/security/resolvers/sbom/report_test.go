// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"testing"
	"time"

	cyclonedx_v1_4 "github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
)

func propertyValue(comp *cyclonedx_v1_4.Component, name string) (string, bool) {
	for _, p := range comp.Properties {
		if p != nil && p.Name == name && p.Value != nil {
			return *p.Value, true
		}
	}
	return "", false
}

// TestToCycloneDXRuntimeProperties checks that all three runtime properties are
// always emitted, including LastSeenRunning "0" for a package never seen
// running. The merge in the core agent overwrites only properties present in
// the forwarded report, so an omitted "0" would leave a stale timestamp behind
// when a refresh resets a package's usage.
func TestToCycloneDXRuntimeProperties(t *testing.T) {
	seen := time.Unix(1700000000, 0)

	tests := []struct {
		name         string
		pkg          sbomtypes.Package
		wantLastSeen string
		wantSuidBit  string
		wantAsRoot   string
	}{
		{
			name:         "never seen running",
			pkg:          sbomtypes.Package{Name: "gzip", Version: "1.12"},
			wantLastSeen: "0",
			wantSuidBit:  "false",
			wantAsRoot:   "false",
		},
		{
			name:         "seen running as root with setuid bit",
			pkg:          sbomtypes.Package{Name: "util-linux", Version: "2.37", LastAccess: seen, AccessedByRoot: true, SuidBit: true},
			wantLastSeen: "1700000000",
			wantSuidBit:  "true",
			wantAsRoot:   "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := NewPackagesReport([]sbomtypes.Package{tt.pkg}, "")
			bom := report.ToCycloneDX()
			if len(bom.Components) != 1 {
				t.Fatalf("got %d components, want 1", len(bom.Components))
			}
			comp := bom.Components[0]

			for _, p := range []struct {
				name string
				want string
			}{
				{LastAccessProperty, tt.wantLastSeen},
				{HasSetSuidBitProperty, tt.wantSuidBit},
				{RunningAsRootProperty, tt.wantAsRoot},
			} {
				got, ok := propertyValue(comp, p.name)
				if !ok {
					t.Errorf("%s missing, want %q", p.name, p.want)
					continue
				}
				if got != p.want {
					t.Errorf("%s = %q, want %q", p.name, got, p.want)
				}
			}
		})
	}
}

// TestHostReportIsMarkedAsHost checks that the host report says so and carries the
// same runtime properties as a container report. The gRPC server reads the mark to
// set the message kind, so a report that lost it would be merged onto an image.
func TestHostReportIsMarkedAsHost(t *testing.T) {
	report := NewHostPackagesReport([]sbomtypes.Package{{
		Name:           "shadow-utils",
		Version:        "4.15.1",
		SuidBit:        true,
		AccessedByRoot: true,
		LastAccess:     time.Unix(1770000000, 0),
	}})

	if !report.IsHost() {
		t.Errorf("host report is not marked as the host's")
	}
	if container := NewPackagesReport(nil, "container-id"); container.IsHost() {
		t.Errorf("container report is marked as the host's")
	}
	if got, want := report.ID(), NewHostPackagesReport(nil).ID(); got != want {
		t.Errorf("ID = %q, want the host id %q regardless of the packages", got, want)
	}

	bom := report.ToCycloneDX()
	if len(bom.Components) != 1 {
		t.Fatalf("bom has %d components, want 1", len(bom.Components))
	}
	want := map[string]string{
		LastAccessProperty:    "1770000000",
		HasSetSuidBitProperty: "true",
		RunningAsRootProperty: "true",
	}
	for _, prop := range bom.Components[0].Properties {
		if expected, ok := want[prop.Name]; ok {
			if prop.GetValue() != expected {
				t.Errorf("%s = %q, want %q", prop.Name, prop.GetValue(), expected)
			}
			delete(want, prop.Name)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing properties: %v", want)
	}
}
