// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbomutil

import (
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func component(name, version string, props ...*cyclonedx_v1_4.Property) *cyclonedx_v1_4.Component {
	return &cyclonedx_v1_4.Component{
		Name:       name,
		Version:    version,
		Properties: props,
	}
}

func prop(name, value string) *cyclonedx_v1_4.Property {
	return &cyclonedx_v1_4.Property{Name: name, Value: pointer.Ptr(value)}
}

// findProp returns the value of the first property matching name (or "" if absent).
func findProp(comp *cyclonedx_v1_4.Component, name string) (string, bool) {
	for _, p := range comp.Properties {
		if p != nil && p.Name == name {
			if p.Value == nil {
				return "", true
			}
			return *p.Value, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// normalizeVersion
// ---------------------------------------------------------------------------

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		in           string
		wantOut      string
		wantHasEpoch bool
	}{
		{"1:4.4.36-4build1", "4.4.36-4build1", true},
		{"4.4.36-4build1", "4.4.36-4build1", false},
		{"", "", false},
		// Only the first colon is treated as the epoch delimiter.
		{"2:1.2.3:extra", "1.2.3:extra", true},
		// Leading colon is not an epoch (idx must be > 0).
		{":foo", ":foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, hasEpoch := normalizeVersion(tt.in)
			assert.Equal(t, tt.wantOut, got)
			assert.Equal(t, tt.wantHasEpoch, hasEpoch)
		})
	}
}

func TestHasOSPackage(t *testing.T) {
	withPurl := func(purl string) *cyclonedx_v1_4.Component {
		return &cyclonedx_v1_4.Component{Name: "pkg", Version: "1", Purl: pointer.Ptr(purl)}
	}

	tests := []struct {
		name string
		bom  *cyclonedx_v1_4.Bom
		want bool
	}{
		{name: "deb", bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{withPurl("pkg:deb/debian/bash@5.1")}}, want: true},
		{name: "rpm", bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{withPurl("pkg:rpm/redhat/bash@5.2")}}, want: true},
		{name: "apk", bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{withPurl("pkg:apk/alpine/busybox@1.36")}}, want: true},
		{name: "npm alone", bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{withPurl("pkg:npm/lodash@4.17.21")}}, want: false},
		{name: "no purl", bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{component("bash", "5.1")}}, want: false},
		{name: "OS component", bom: &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{{Name: "debian", Version: "12", Type: cyclonedx_v1_4.Classification_CLASSIFICATION_OPERATING_SYSTEM}}}, want: false},
		{name: "nil", bom: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasOSPackage(tt.bom))
		})
	}
}

func TestMergeRuntimeProperties_AddsRuntimeProperties(t *testing.T) {
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k"),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k",
				prop(LastAccessProperty, "1700000000"),
				prop(HasSetSuidBitProperty, "true"),
				prop(RunningAsRootProperty, "false"),
			),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]
	assert.Equal(t, "openssl", c.Name)
	assert.Equal(t, "1.1.1k", c.Version)

	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)

	v, ok = findProp(c, HasSetSuidBitProperty)
	assert.True(t, ok)
	assert.Equal(t, "true", v)

	v, ok = findProp(c, RunningAsRootProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)
}

func TestMergeRuntimeProperties_UpdatesExistingProperty(t *testing.T) {
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "100")),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "200")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	// Property is updated to the new value, not duplicated.
	count := 0
	for _, p := range c.Properties {
		if p != nil && p.Name == LastAccessProperty {
			count++
		}
	}
	assert.Equal(t, 1, count, "LastAccessProperty must appear exactly once")

	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "200", v)
}

func TestMergeRuntimeProperties_EpochNormalization(t *testing.T) {
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("e2fsprogs", "1:4.4.36-4build1"),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("e2fsprogs", "4.4.36-4build1",
				prop(LastAccessProperty, "1700000000"),
			),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	// Original version string (with epoch) is preserved on the existing entry.
	assert.Equal(t, "1:4.4.36-4build1", c.Version)

	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok, "runtime property from newBom must be merged across epoch")
	assert.Equal(t, "1700000000", v)
}

func TestMergeRuntimeProperties_DefaultsLastSeenRunningToZero(t *testing.T) {
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "bash",
				Version: "5.1",
				// An OS package: in the runtime scanner's scope, so the runtime
				// properties are defaulted even without a match in newBom.
				Purl: pointer.Ptr("pkg:deb/debian/bash@5.1?arch=amd64"),
			},
		},
	}
	// No match in newBom.
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("zsh", "5.9", prop(LastAccessProperty, "1700000000")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]
	assert.Equal(t, "bash", c.Name)

	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok, "LastAccessProperty must be defaulted to 0 when neither side supplies it")
	assert.Equal(t, "0", v)

	// HasSetSuidBit / RunningAsRoot are defaulted to "false" too, so consumers can
	// distinguish "not in use" from "unknown".
	v, ok = findProp(c, HasSetSuidBitProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)
	v, ok = findProp(c, RunningAsRootProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)
}

func TestMergeRuntimeProperties_LeavesOutOfScopeComponentsUnset(t *testing.T) {
	// The runtime scanner reads the dpkg, rpm and apk databases, so these
	// components stay out of its scope and keep the properties absent.
	tests := []struct {
		name      string
		component *cyclonedx_v1_4.Component
	}{
		{
			name: "language library",
			component: &cyclonedx_v1_4.Component{
				Type:    cyclonedx_v1_4.Classification_CLASSIFICATION_LIBRARY,
				Name:    "lodash",
				Version: "4.17.21",
				Purl:    pointer.Ptr("pkg:npm/lodash@4.17.21"),
			},
		},
		{
			name: "operating system",
			component: &cyclonedx_v1_4.Component{
				Type:    cyclonedx_v1_4.Classification_CLASSIFICATION_OPERATING_SYSTEM,
				Name:    "debian",
				Version: "12.5",
			},
		},
		{
			name: "lockfile application",
			component: &cyclonedx_v1_4.Component{
				Type: cyclonedx_v1_4.Classification_CLASSIFICATION_APPLICATION,
				Name: "app/Gemfile.lock",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// An OS package alongside it, which the merge must still default.
			osPackage := &cyclonedx_v1_4.Component{
				Name:    "bash",
				Version: "5.1",
				Purl:    pointer.Ptr("pkg:deb/debian/bash@5.1?arch=amd64"),
			}
			existing := &cyclonedx_v1_4.Bom{
				Components: []*cyclonedx_v1_4.Component{test.component, osPackage},
			}
			newBom := &cyclonedx_v1_4.Bom{
				Components: []*cyclonedx_v1_4.Component{
					component("zsh", "5.9", prop(LastAccessProperty, "1700000000")),
				},
			}

			merged := MergeRuntimeProperties(existing, newBom)

			// The merge rebuilds the component list in order.
			require.Len(t, merged.Components, 2)
			require.Equal(t, test.component.Name, merged.Components[0].Name)
			require.Equal(t, osPackage.Name, merged.Components[1].Name)

			for _, name := range []string{LastAccessProperty, HasSetSuidBitProperty, RunningAsRootProperty} {
				_, ok := findProp(merged.Components[0], name)
				assert.Falsef(t, ok, "%s must stay absent on an out-of-scope component", name)
			}

			v, ok := findProp(merged.Components[1], LastAccessProperty)
			assert.True(t, ok, "the OS package in the same BOM must still be defaulted")
			assert.Equal(t, "0", v)
		})
	}
}

func TestMergeRuntimeProperties_DefaultsReportedComponentWithPartialProperties(t *testing.T) {
	// The report puts a component in scope whatever its purl says, so the
	// properties it left out are defaulted too.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k"),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "1700000000")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
	v, ok = findProp(c, HasSetSuidBitProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)
	v, ok = findProp(c, RunningAsRootProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)
}

func TestMergeRuntimeProperties_KeepsTheReportOffForeignPurls(t *testing.T) {
	// The report reads the dpkg, rpm and apk databases, so a gem sharing an OS
	// package's name and version is not the package it saw running.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				BomRef:  pointer.Ptr("1"),
				Name:    "openssl",
				Version: "3.0.11",
				Purl:    pointer.Ptr("pkg:gem/openssl@3.0.11"),
			},
			{
				BomRef:  pointer.Ptr("2"),
				Name:    "openssl",
				Version: "3.0.11",
				Purl:    pointer.Ptr("pkg:deb/debian/openssl@3.0.11"),
			},
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "3.0.11",
				prop(LastAccessProperty, "1700000000"),
				prop(HasSetSuidBitProperty, "true"),
				prop(RunningAsRootProperty, "true"),
			),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 2)
	gem, deb := merged.Components[0], merged.Components[1]
	require.Equal(t, "pkg:gem/openssl@3.0.11", gem.GetPurl())

	for _, name := range []string{LastAccessProperty, HasSetSuidBitProperty, RunningAsRootProperty} {
		_, ok := findProp(gem, name)
		assert.Falsef(t, ok, "%s reached the gem", name)
	}

	// The OS package of the same name and version still takes the report.
	v, ok := findProp(deb, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
	v, ok = findProp(deb, HasSetSuidBitProperty)
	assert.True(t, ok)
	assert.Equal(t, "true", v)
}

func TestMergeRuntimeProperties_CollapsesDuplicateBomRefs(t *testing.T) {
	// A stored image SBOM carrying the same component twice: one bom-ref is one
	// component, so the first occurrence stands for both.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				BomRef:  pointer.Ptr("1"),
				Name:    "openssl",
				Version: "1:1.1.1k",
			},
			{
				BomRef:     pointer.Ptr("1"),
				Name:       "openssl",
				Version:    "1:1.1.1k",
				Properties: []*cyclonedx_v1_4.Property{prop(LastAccessProperty, "100")},
			},
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "200")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1, "components sharing a bom-ref must be collapsed")
	c := merged.Components[0]
	assert.Equal(t, "1:1.1.1k", c.Version)

	// Runtime property is still updated from newBom, across the epoch difference.
	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "200", v)
}

func TestMergeRuntimeProperties_KeepsDistinctComponentsSharingNameAndVersion(t *testing.T) {
	// Trivy reports one entry per install location, so a library version pinned by
	// two lockfiles appears twice with distinct bom-refs. Both are real components
	// that the dependency graph references, so both must survive. Their purls are
	// foreign to the report, so neither carries a runtime property.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				BomRef:  pointer.Ptr("5"),
				Name:    "actionpack",
				Version: "7.0.0",
				Purl:    pointer.Ptr("pkg:gem/actionpack@7.0.0"),
			},
			{
				BomRef:  pointer.Ptr("8"),
				Name:    "actionpack",
				Version: "7.0.0",
				Purl:    pointer.Ptr("pkg:gem/actionpack@7.0.0"),
			},
		},
		Dependencies: []*cyclonedx_v1_4.Dependency{{Ref: "8"}},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("actionpack", "7.0.0", prop(LastAccessProperty, "1700000000")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 2, "distinct components sharing a name and version must both survive")
	refs := make(map[string]struct{}, len(merged.Components))
	for _, c := range merged.Components {
		refs[c.GetBomRef()] = struct{}{}

		_, ok := findProp(c, LastAccessProperty)
		assert.Falsef(t, ok, "the report reached %s", c.GetPurl())
	}

	// Every dependency reference still resolves to a component of the merged BOM.
	for _, dep := range merged.Dependencies {
		assert.Containsf(t, refs, dep.Ref, "dependency ref %q no longer resolves to a component", dep.Ref)
	}
}

func TestMergeRuntimeProperties_ReportReachesDuplicateOSPackages(t *testing.T) {
	// A multilib rpm is installed once per architecture, so it appears with one
	// name and version, one bom-ref per entry, and the report covers both.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				BomRef:  pointer.Ptr("5"),
				Name:    "glibc",
				Version: "2.34-1.el9",
				Purl:    pointer.Ptr("pkg:rpm/rhel/glibc@2.34-1.el9?arch=i686"),
			},
			{
				BomRef:  pointer.Ptr("8"),
				Name:    "glibc",
				Version: "2.34-1.el9",
				Purl:    pointer.Ptr("pkg:rpm/rhel/glibc@2.34-1.el9?arch=x86_64"),
			},
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("glibc", "2.34-1.el9", prop(LastAccessProperty, "1700000000")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 2)
	for _, c := range merged.Components {
		v, ok := findProp(c, LastAccessProperty)
		assert.Truef(t, ok, "%s missed the report", c.GetPurl())
		assert.Equal(t, "1700000000", v)
	}
}

func TestMergeRuntimeProperties_KeepsComponentsWithoutBomRef(t *testing.T) {
	// The bom-ref is what identifies a component, so deduplication applies to the
	// components that carry one and the others are emitted in turn.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k"),
			component("openssl", "1.1.1k"),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "1700000000")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 2)
}

func TestMergeRuntimeProperties_NilSafeInputs(t *testing.T) {
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("bash", "5.1"),
			nil, // nil entry must be skipped, not panic
		},
	}

	// nil newBom: returns existingBom unchanged.
	got := MergeRuntimeProperties(existing, nil)
	assert.Same(t, existing, got)

	// Empty components on newBom: returns existingBom unchanged.
	got = MergeRuntimeProperties(existing, &cyclonedx_v1_4.Bom{})
	assert.Same(t, existing, got)

	// nil entries in newBom.Components are skipped.
	merged := MergeRuntimeProperties(existing, &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{nil},
	})
	assert.Len(t, merged.Components, 1)
}

func TestMergeRuntimeProperties_PreservesEnvelope(t *testing.T) {
	serial := "urn:uuid:1234"
	existing := &cyclonedx_v1_4.Bom{
		SpecVersion:  "1.4",
		SerialNumber: &serial,
		Metadata:     &cyclonedx_v1_4.Metadata{Component: component("debian", "12")},
		Dependencies: []*cyclonedx_v1_4.Dependency{{Ref: "ref-1"}},
		Components: []*cyclonedx_v1_4.Component{
			component("bash", "5.1"),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Metadata:   &cyclonedx_v1_4.Metadata{Properties: []*cyclonedx_v1_4.Property{prop(UsageObservedSinceProperty, "1700000000")}},
		Components: []*cyclonedx_v1_4.Component{component("bash", "5.1", prop(LastAccessProperty, "1700000100"))},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	assert.Equal(t, existing.SpecVersion, merged.SpecVersion)
	assert.Equal(t, existing.SerialNumber, merged.SerialNumber)
	assert.Same(t, existing.Metadata.Component, merged.Metadata.Component)
	assert.Equal(t, existing.Dependencies, merged.Dependencies)
}

// TestMergeRuntimeProperties_CarriesUsageObservedSince checks that the merged BOM
// holds the start of usage observation of the report merged into it.
func TestMergeRuntimeProperties_CarriesUsageObservedSince(t *testing.T) {
	report := func(since ...string) *cyclonedx_v1_4.Bom {
		bom := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{component("bash", "5.1", prop(LastAccessProperty, "1700000100"))}}
		if len(since) > 0 {
			bom.Metadata = &cyclonedx_v1_4.Metadata{Properties: []*cyclonedx_v1_4.Property{prop(UsageObservedSinceProperty, since[0])}}
		}
		return bom
	}
	scanned := func(metadata *cyclonedx_v1_4.Metadata) *cyclonedx_v1_4.Bom {
		return &cyclonedx_v1_4.Bom{Metadata: metadata, Components: []*cyclonedx_v1_4.Component{component("bash", "5.1")}}
	}
	since := func(bom *cyclonedx_v1_4.Bom) string {
		value, _ := findMetadataProp(bom, UsageObservedSinceProperty)
		return value
	}

	t.Run("the start of the report", func(t *testing.T) {
		existing := scanned(&cyclonedx_v1_4.Metadata{
			Component:  component("debian", "12"),
			Properties: []*cyclonedx_v1_4.Property{prop("aquasecurity:trivy:SchemaVersion", "2")},
		})

		merged := MergeRuntimeProperties(existing, report("1700000000"))

		assert.Equal(t, "1700000000", since(merged))
		assert.Same(t, existing.Metadata.Component, merged.Metadata.Component)
		value, _ := findMetadataProp(merged, "aquasecurity:trivy:SchemaVersion")
		assert.Equal(t, "2", value, "the other metadata properties stay")
		assert.Empty(t, since(existing), "the existing BOM stays as it was")
	})

	t.Run("a later start replaces the earlier one", func(t *testing.T) {
		existing := MergeRuntimeProperties(scanned(nil), report("1700000000"))

		merged := MergeRuntimeProperties(existing, report("1700003600"))

		assert.Equal(t, "1700003600", since(merged))
		assert.Len(t, merged.Metadata.Properties, 1)
	})

	t.Run("a report without a start drops the earlier one", func(t *testing.T) {
		existing := MergeRuntimeProperties(scanned(nil), report("1700000000"))

		merged := MergeRuntimeProperties(existing, report())

		assert.Empty(t, since(merged))
	})

	t.Run("no start on either side", func(t *testing.T) {
		existing := scanned(&cyclonedx_v1_4.Metadata{Component: component("debian", "12")})

		merged := MergeRuntimeProperties(existing, report())

		assert.Same(t, existing.Metadata, merged.Metadata)
	})
}

// findMetadataProp returns the value of the metadata property name of bom.
func findMetadataProp(bom *cyclonedx_v1_4.Bom, name string) (string, bool) {
	for _, p := range bom.GetMetadata().GetProperties() {
		if p.GetName() == name {
			return p.GetValue(), true
		}
	}
	return "", false
}

func TestUsageObservedSince(t *testing.T) {
	withSince := func(value string) *cyclonedx_v1_4.Bom {
		return &cyclonedx_v1_4.Bom{Metadata: &cyclonedx_v1_4.Metadata{Properties: []*cyclonedx_v1_4.Property{prop(UsageObservedSinceProperty, value)}}}
	}

	since, ok := UsageObservedSince(withSince("1700000000"))
	assert.True(t, ok)
	assert.Equal(t, time.Unix(1700000000, 0), since)

	for name, bom := range map[string]*cyclonedx_v1_4.Bom{
		"nil":       nil,
		"no start":  {Components: []*cyclonedx_v1_4.Component{component("bash", "5.1")}},
		"malformed": withSince("yesterday"),
	} {
		t.Run(name, func(t *testing.T) {
			_, ok := UsageObservedSince(bom)
			assert.False(t, ok)
		})
	}
}

func TestUsageWindowOpen(t *testing.T) {
	now := time.Unix(1700003600, 0)
	withSince := func(since time.Time) *cyclonedx_v1_4.Bom {
		return &cyclonedx_v1_4.Bom{Metadata: &cyclonedx_v1_4.Metadata{Properties: []*cyclonedx_v1_4.Property{
			prop(UsageObservedSinceProperty, strconv.FormatInt(since.Unix(), 10)),
		}}}
	}

	tests := []struct {
		name string
		bom  *cyclonedx_v1_4.Bom
		want bool
	}{
		{name: "recorded for less than the window", bom: withSince(now.Add(-59 * time.Minute)), want: true},
		{name: "recorded for the window", bom: withSince(now.Add(-time.Hour)), want: false},
		{name: "start ahead of the clock", bom: withSince(now.Add(time.Minute)), want: true},
		{name: "no start", bom: &cyclonedx_v1_4.Bom{}, want: false},
		{name: "nil", bom: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UsageWindowOpen(tt.bom, now, time.Hour))
		})
	}
}

// TestHideUnobserved checks that the unseen components lose their runtime
// properties, and the others keep theirs.
func TestHideUnobserved(t *testing.T) {
	unseenProps := []*cyclonedx_v1_4.Property{
		prop("aquasecurity:trivy:PkgType", "debian"),
		prop(LastAccessProperty, "0"),
		prop(HasSetSuidBitProperty, "false"),
		prop(RunningAsRootProperty, "false"),
	}
	unseen := component("gzip", "1.12", unseenProps...)
	unseen.Purl = pointer.Ptr("pkg:deb/debian/gzip@1.12")
	reported := component("zstd", "1.5", prop(LastAccessProperty, "0"))
	seen := component("bash", "5.1", prop(LastAccessProperty, "1700000000"), prop(HasSetSuidBitProperty, "false"), prop(RunningAsRootProperty, "true"))
	outOfScope := component("lodash", "4.17.21")
	bom := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{unseen, reported, seen, outOfScope}}

	HideUnobserved(bom)

	require.Len(t, unseen.Properties, 1)
	assert.Equal(t, "aquasecurity:trivy:PkgType", unseen.Properties[0].Name, "the other properties stay")
	assert.Equal(t, LastAccessProperty, unseenProps[1].Name, "the property slice the component shared stays as it was")
	assert.Empty(t, reported.Properties, "a reported component without a purl loses them too")
	assert.Len(t, seen.Properties, 3)
	assert.Empty(t, outOfScope.Properties)
}

// TestHideUnreported checks that the OS packages a usage report lacks lose
// their runtime properties, and the others keep theirs.
func TestHideUnreported(t *testing.T) {
	merged := func(name, version, purl string) *cyclonedx_v1_4.Component {
		comp := component(name, version,
			prop("aquasecurity:trivy:PkgType", "redhat"),
			prop(LastAccessProperty, "0"),
			prop(HasSetSuidBitProperty, "false"),
			prop(RunningAsRootProperty, "false"),
		)
		comp.Purl = pointer.Ptr(purl)
		return comp
	}
	report := &cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{
		component("bash", "5.2.26-6.el10"),
		component("openssl", "1:3.2.2-6.el10"),
		component("gzip", "1.12-1.el10"),
	}}

	tests := []struct {
		name   string
		comp   *cyclonedx_v1_4.Component
		report *cyclonedx_v1_4.Bom
		want   int
	}{
		{name: "reported", comp: merged("bash", "5.2.26-6.el10", "pkg:rpm/redhat/bash@5.2.26-6.el10"), report: report, want: 4},
		{name: "reported with an epoch", comp: merged("openssl", "3.2.2-6.el10", "pkg:rpm/redhat/openssl@3.2.2-6.el10"), report: report, want: 4},
		{name: "installed after the report", comp: merged("fish", "3.7.1-2.el10", "pkg:rpm/redhat/fish@3.7.1-2.el10"), report: report, want: 1},
		{name: "upgraded after the report", comp: merged("gzip", "1.13-1.el10", "pkg:rpm/redhat/gzip@1.13-1.el10"), report: report, want: 1},
		{name: "out of scope", comp: merged("lodash", "4.17.21", "pkg:npm/lodash@4.17.21"), report: report, want: 4},
		{name: "empty report", comp: merged("fish", "3.7.1-2.el10", "pkg:rpm/redhat/fish@3.7.1-2.el10"), report: &cyclonedx_v1_4.Bom{}, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			properties := tt.comp.Properties
			HideUnreported(&cyclonedx_v1_4.Bom{Components: []*cyclonedx_v1_4.Component{tt.comp}}, tt.report)

			assert.Len(t, tt.comp.Properties, tt.want)
			assert.Equal(t, LastAccessProperty, properties[1].Name, "the property slice the component shared stays as it was")
			if tt.want == 1 {
				assert.Equal(t, "aquasecurity:trivy:PkgType", tt.comp.Properties[0].Name, "the other properties stay")
			}
		})
	}
}

func TestMergeRuntimeProperties_DoesNotOverridePurl(t *testing.T) {
	// system-probe MUST NOT be able to overwrite a Purl set by the
	// authoritative (Trivy) image SBOM. Trivy produces the canonical purl;
	// anything supplied by system-probe is purely a runtime annotation.
	trustedPurl := "pkg:deb/openssl@1.1.1k?arch=amd64"
	attackerPurl := "pkg:deb/openssl@1.1.1k?arch=amd64&malicious=true"

	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "openssl",
				Version: "1.1.1k",
				Purl:    pointer.Ptr(trustedPurl),
			},
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "openssl",
				Version: "1.1.1k",
				Purl:    pointer.Ptr(attackerPurl),
				Properties: []*cyclonedx_v1_4.Property{
					prop(LastAccessProperty, "1700000000"),
				},
			},
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	// Purl must be the trusted one, not the system-probe one.
	require.NotNil(t, c.Purl)
	assert.Equal(t, trustedPurl, *c.Purl)

	// Runtime property must still be merged in.
	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
}

func TestMergeRuntimeProperties_DoesNotAddPurlWhenAbsent(t *testing.T) {
	// If the existing image SBOM has no Purl for a component, system-probe
	// must not be able to inject one — the absence is itself a signal that
	// Trivy could not derive a package URL.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "openssl",
				Version: "1.1.1k",
				// Purl intentionally nil.
			},
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "openssl",
				Version: "1.1.1k",
				Purl:    pointer.Ptr("pkg:deb/openssl@1.1.1k?arch=amd64&injected=true"),
				Properties: []*cyclonedx_v1_4.Property{
					prop(LastAccessProperty, "1700000000"),
				},
			},
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	// Purl must remain absent.
	assert.Nil(t, c.Purl, "system-probe must not be able to inject a Purl when existing component has none")
}

func TestMergeRuntimeProperties_DoesNotOverridePurlAcrossEpoch(t *testing.T) {
	// Version-with-epoch and version-without-epoch must dedupe to the same
	// component, and the existing Purl must survive a system-probe value
	// supplied alongside the normalised version.
	trustedPurl := "pkg:deb/e2fsprogs@1:4.4.36-4build1?arch=amd64"

	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "e2fsprogs",
				Version: "1:4.4.36-4build1",
				Purl:    pointer.Ptr(trustedPurl),
			},
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			{
				Name:    "e2fsprogs",
				Version: "4.4.36-4build1",
				Purl:    pointer.Ptr("pkg:deb/e2fsprogs@bogus"),
				Properties: []*cyclonedx_v1_4.Property{
					prop(LastAccessProperty, "1700000000"),
				},
			},
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	require.NotNil(t, c.Purl)
	assert.Equal(t, trustedPurl, *c.Purl)
}

func TestMergeRuntimeProperties_DoesNotMutateInput(t *testing.T) {
	original := component("openssl", "1.1.1k", prop("trivy.layer", "sha256:abc"))
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{original},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "1700000000")),
		},
	}

	mergedBom := MergeRuntimeProperties(existing, newBom)

	// Returned component must be a different struct pointer than the input,
	// so the merge does not leak runtime props back onto the source BOM.
	assert.NotSame(t, original, mergedBom.Components[0])

	// Original component is untouched.
	assert.Len(t, original.Properties, 1)
	assert.Equal(t, "trivy.layer", original.Properties[0].Name)
	_, hasLastSeen := findProp(original, LastAccessProperty)
	assert.False(t, hasLastSeen, "original component must not be enriched in place")
}

// A component that already carries the runtime properties, as it does on every
// merge round after the first, takes the in-place update path of updateProperty.
func TestMergeRuntimeProperties_DoesNotMutateEnrichedInput(t *testing.T) {
	original := component("openssl", "1.1.1k",
		prop("trivy.layer", "sha256:abc"),
		prop(LastAccessProperty, "1700000000"),
		prop(HasSetSuidBitProperty, "true"),
	)
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{original},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k",
				prop(LastAccessProperty, "1800000000"),
				prop(HasSetSuidBitProperty, "false"),
			),
		},
	}

	mergedBom := MergeRuntimeProperties(existing, newBom)

	got, ok := findProp(original, LastAccessProperty)
	require.True(t, ok)
	assert.Equal(t, "1700000000", got, "merge overwrote LastSeenRunning on the input BOM")
	got, ok = findProp(original, HasSetSuidBitProperty)
	require.True(t, ok)
	assert.Equal(t, "true", got, "merge overwrote HasSetSuidBit on the input BOM")

	// The merged copy carries the new values.
	got, ok = findProp(mergedBom.Components[0], LastAccessProperty)
	require.True(t, ok)
	assert.Equal(t, "1800000000", got)
	got, ok = findProp(mergedBom.Components[0], HasSetSuidBitProperty)
	require.True(t, ok)
	assert.Equal(t, "false", got)
}
