// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbomutil

import (
	"testing"

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

// ---------------------------------------------------------------------------
// mergeRuntimeProperties
// ---------------------------------------------------------------------------

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
			component("bash", "5.1"),
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

func TestMergeRuntimeProperties_DeduplicatesAcrossRounds(t *testing.T) {
	// Simulates an already-merged image SBOM that carries both the raw Trivy
	// entry and a previously-runtime-enriched copy of the same package.
	existing := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1:1.1.1k"),
			component("openssl", "1.1.1k", prop(LastAccessProperty, "100")),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{
		Components: []*cyclonedx_v1_4.Component{
			component("openssl", "1.1.1k", prop(LastAccessProperty, "200")),
		},
	}

	merged := MergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1, "duplicates by name+normalised version must be collapsed")
	c := merged.Components[0]

	// First occurrence wins, so the version retains the epoch form.
	assert.Equal(t, "1:1.1.1k", c.Version)

	// Runtime property is still updated from newBom.
	v, ok := findProp(c, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "200", v)
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
		Metadata:     &cyclonedx_v1_4.Metadata{},
		Dependencies: []*cyclonedx_v1_4.Dependency{{Ref: "ref-1"}},
		Components: []*cyclonedx_v1_4.Component{
			component("bash", "5.1"),
		},
	}
	newBom := &cyclonedx_v1_4.Bom{}

	merged := MergeRuntimeProperties(existing, newBom)

	assert.Equal(t, existing.SpecVersion, merged.SpecVersion)
	assert.Equal(t, existing.SerialNumber, merged.SerialNumber)
	assert.Same(t, existing.Metadata, merged.Metadata)
	assert.Equal(t, existing.Dependencies, merged.Dependencies)
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
