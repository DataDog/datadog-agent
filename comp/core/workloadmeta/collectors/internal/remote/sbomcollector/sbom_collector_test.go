// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package sbomcollector

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// ---------------------------------------------------------------------------
// fakeStore is a minimal workloadmeta.Component used by the
// workloadmetaEventFromSBOMEventSet tests below. Only GetContainer, GetImage
// and ListImages are exercised by the code under test; the embedded interface
// keeps the type assignable to workloadmeta.Component without forcing us to
// stub the ~30 other methods on the interface.
// ---------------------------------------------------------------------------

type fakeStore struct {
	workloadmeta.Component

	containers map[string]*workloadmeta.Container
	images     map[string]*workloadmeta.ContainerImageMetadata
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		containers: make(map[string]*workloadmeta.Container),
		images:     make(map[string]*workloadmeta.ContainerImageMetadata),
	}
}

func (f *fakeStore) GetContainer(id string) (*workloadmeta.Container, error) {
	if c, ok := f.containers[id]; ok {
		return c, nil
	}
	return nil, errors.New("container not found")
}

func (f *fakeStore) GetImage(id string) (*workloadmeta.ContainerImageMetadata, error) {
	if i, ok := f.images[id]; ok {
		return i, nil
	}
	return nil, errors.New("image not found")
}

func (f *fakeStore) ListImages() []*workloadmeta.ContainerImageMetadata {
	out := make([]*workloadmeta.ContainerImageMetadata, 0, len(f.images))
	for _, i := range f.images {
		out = append(out, i)
	}
	return out
}

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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

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
	got := mergeRuntimeProperties(existing, nil)
	assert.Same(t, existing, got)

	// Empty components on newBom: returns existingBom unchanged.
	got = mergeRuntimeProperties(existing, &cyclonedx_v1_4.Bom{})
	assert.Same(t, existing, got)

	// nil entries in newBom.Components are skipped.
	merged := mergeRuntimeProperties(existing, &cyclonedx_v1_4.Bom{
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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

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

	merged := mergeRuntimeProperties(existing, newBom)

	require.Len(t, merged.Components, 1)
	c := merged.Components[0]

	require.NotNil(t, c.Purl)
	assert.Equal(t, trustedPurl, *c.Purl)
}

func TestWorkloadmetaEventFromSBOMEventSet_DoesNotOverridePurl(t *testing.T) {
	// End-to-end check: a system-probe SBOMMessage carrying a different Purl
	// for an already-known component must not change the Purl stored on the
	// image SBOM.
	const containerID = "container-purl"
	const imageID = "image-purl"
	const trustedPurl = "pkg:deb/openssl@1.1.1k?arch=amd64"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, workloadmeta.Success,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			Purl:    pointer.Ptr(trustedPurl),
		},
	)

	msg := systemProbeMessage(t, containerID,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			Purl:    pointer.Ptr("pkg:deb/openssl@1.1.1k?malicious=true"),
			Properties: []*cyclonedx_v1_4.Property{
				prop(LastAccessProperty, "1700000000"),
			},
		},
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)

	img, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	require.NotNil(t, img.SBOM)

	decompressed, err := sbomutil.UncompressSBOM(img.SBOM)
	require.NoError(t, err)
	require.NotNil(t, decompressed.CycloneDXBOM)
	require.Len(t, decompressed.CycloneDXBOM.Components, 1)

	merged := decompressed.CycloneDXBOM.Components[0]
	require.NotNil(t, merged.Purl)
	assert.Equal(t, trustedPurl, *merged.Purl, "system-probe must not be able to overwrite Purl on the image SBOM")

	// Runtime annotation still merged in.
	v, ok := findProp(merged, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
}

func TestWorkloadmetaEventFromSBOMEventSet_DoesNotAddPurlWhenAbsent(t *testing.T) {
	// End-to-end check: if the image SBOM has no Purl on a component,
	// system-probe must not be able to inject one.
	const containerID = "container-no-purl"
	const imageID = "image-no-purl"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, workloadmeta.Success,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			// Purl intentionally nil.
		},
	)

	msg := systemProbeMessage(t, containerID,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			Purl:    pointer.Ptr("pkg:deb/openssl@1.1.1k?injected=true"),
			Properties: []*cyclonedx_v1_4.Property{
				prop(LastAccessProperty, "1700000000"),
			},
		},
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)

	img, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	require.NotNil(t, img.SBOM)

	decompressed, err := sbomutil.UncompressSBOM(img.SBOM)
	require.NoError(t, err)
	require.Len(t, decompressed.CycloneDXBOM.Components, 1)

	merged := decompressed.CycloneDXBOM.Components[0]
	assert.Nil(t, merged.Purl, "system-probe must not be able to inject a Purl on a component that had none")
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

	mergedBom := mergeRuntimeProperties(existing, newBom)

	// Returned component must be a different struct pointer than the input,
	// so the merge does not leak runtime props back onto the source BOM.
	assert.NotSame(t, original, mergedBom.Components[0])

	// Original component is untouched.
	assert.Len(t, original.Properties, 1)
	assert.Equal(t, "trivy.layer", original.Properties[0].Name)
	_, hasLastSeen := findProp(original, LastAccessProperty)
	assert.False(t, hasLastSeen, "original component must not be enriched in place")
}

// ---------------------------------------------------------------------------
// workloadmetaEventFromSBOMEventSet
// ---------------------------------------------------------------------------

// seedImageSBOM constructs a ContainerImageMetadata with a compressed SBOM
// holding the given components and Success status.
func seedImageSBOM(t *testing.T, imageID string, status workloadmeta.SBOMStatus, components ...*cyclonedx_v1_4.Component) *workloadmeta.ContainerImageMetadata {
	t.Helper()

	sbom := &workloadmeta.SBOM{
		CycloneDXBOM: &cyclonedx_v1_4.Bom{
			Components: components,
		},
		Status:             status,
		GenerationTime:     time.Unix(1700000000, 0).UTC(),
		GenerationDuration: 250 * time.Millisecond,
		GenerationMethod:   "tarball",
	}
	compressed, err := sbomutil.CompressSBOM(sbom)
	require.NoError(t, err)

	return &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imageID,
		},
		SBOM: compressed,
	}
}

// systemProbeMessage marshals a CycloneDX BOM into the SBOMMessage shape
// produced by system-probe.
func systemProbeMessage(t *testing.T, containerID string, components ...*cyclonedx_v1_4.Component) *sbompb.SBOMMessage {
	t.Helper()
	data, err := proto.Marshal(&cyclonedx_v1_4.Bom{Components: components})
	require.NoError(t, err)
	return &sbompb.SBOMMessage{
		Kind: string(workloadmeta.KindContainer),
		ID:   containerID,
		Data: data,
	}
}

func TestWorkloadmetaEventFromSBOMEventSet_HappyPath(t *testing.T) {
	const containerID = "container-1"
	const imageID = "image-1"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	existingImage := seedImageSBOM(t, imageID, workloadmeta.Success,
		component("openssl", "1.1.1k", prop("trivy.layer", "sha256:abc")),
	)
	store.images[imageID] = existingImage

	msg := systemProbeMessage(t, containerID,
		component("openssl", "1.1.1k",
			prop(LastAccessProperty, "1700000000"),
			prop(HasSetSuidBitProperty, "true"),
			prop(RunningAsRootProperty, "false"),
		),
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)
	require.Equal(t, workloadmeta.EventTypeSet, event.Type)

	img, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok, "entity should be ContainerImageMetadata, got %T", event.Entity)
	assert.Equal(t, imageID, img.ID)
	require.NotNil(t, img.SBOM)

	// Scan metadata is preserved from the existing image SBOM.
	assert.Equal(t, existingImage.SBOM.Status, img.SBOM.Status)
	assert.Equal(t, existingImage.SBOM.GenerationTime, img.SBOM.GenerationTime)
	assert.Equal(t, existingImage.SBOM.GenerationDuration, img.SBOM.GenerationDuration)
	assert.Equal(t, existingImage.SBOM.GenerationMethod, img.SBOM.GenerationMethod)

	// Decompress and verify runtime properties were merged in.
	decompressed, err := sbomutil.UncompressSBOM(img.SBOM)
	require.NoError(t, err)
	require.NotNil(t, decompressed.CycloneDXBOM)
	require.Len(t, decompressed.CycloneDXBOM.Components, 1)
	merged := decompressed.CycloneDXBOM.Components[0]

	v, ok := findProp(merged, LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
	v, ok = findProp(merged, HasSetSuidBitProperty)
	assert.True(t, ok)
	assert.Equal(t, "true", v)
	v, ok = findProp(merged, RunningAsRootProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)

	// Original Trivy property is preserved.
	v, ok = findProp(merged, "trivy.layer")
	assert.True(t, ok)
	assert.Equal(t, "sha256:abc", v)
}

func TestWorkloadmetaEventFromSBOMEventSet_FallsBackToRepoDigest(t *testing.T) {
	const containerID = "container-2"
	const repoDigest = "docker.io/foo@sha256:9fb3"
	const configDigest = "sha256:configdigest"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		// Kubelet-style ID that does not match the store key.
		Image: workloadmeta.ContainerImage{ID: repoDigest},
	}

	img := seedImageSBOM(t, configDigest, workloadmeta.Success, component("bash", "5.1"))
	img.RepoDigests = []string{repoDigest}
	store.images[configDigest] = img

	msg := systemProbeMessage(t, containerID,
		component("bash", "5.1", prop(LastAccessProperty, "1700000000")),
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)
	require.Equal(t, workloadmeta.EventTypeSet, event.Type)

	got, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	// The enriched SBOM must be keyed by the resolved image entity's own
	// (config-digest) ID, not the kubelet repo digest. Keying it by the repo
	// digest would create a separate, metadata-less image entity that the SBOM
	// check cannot ship; using the config digest lands it on the same entity the
	// runtime collector populates so the two sources merge and the enriched SBOM
	// is published.
	assert.Equal(t, configDigest, got.ID)
}

func TestWorkloadmetaEventFromSBOMEventSet_PendingSBOMSkipped(t *testing.T) {
	const containerID = "container-3"
	const imageID = "image-3"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, workloadmeta.Pending, component("bash", "5.1"))

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.Error(t, err)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_MissingExistingSBOM(t *testing.T) {
	const containerID = "container-4"
	const imageID = "image-4"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: imageID},
		// No SBOM.
	}

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.Error(t, err)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_UnknownImage(t *testing.T) {
	const containerID = "container-5"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: "unknown-image"},
	}

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	// No error - just an empty event so the handler drops it (see HandleResponse loop).
	assert.NoError(t, err)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_MissingContainer(t *testing.T) {
	store := newFakeStore()

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, "unknown-container"))
	assert.Error(t, err)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_ContainerWithoutImageID(t *testing.T) {
	const containerID = "container-no-image"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		// Image.ID left empty.
	}

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.Error(t, err)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_BadInput(t *testing.T) {
	store := newFakeStore()

	t.Run("nil event", func(t *testing.T) {
		event, err := workloadmetaEventFromSBOMEventSet(store, nil)
		assert.NoError(t, err)
		assert.Nil(t, event.Entity)
	})

	t.Run("wrong kind", func(t *testing.T) {
		msg := &sbompb.SBOMMessage{Kind: "kubernetes_pod", ID: "x"}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		assert.Error(t, err)
		assert.Nil(t, event.Entity)
	})

	t.Run("empty ID", func(t *testing.T) {
		msg := &sbompb.SBOMMessage{Kind: string(workloadmeta.KindContainer)}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		assert.Error(t, err)
		assert.Nil(t, event.Entity)
	})

	t.Run("invalid protobuf data", func(t *testing.T) {
		msg := &sbompb.SBOMMessage{
			Kind: string(workloadmeta.KindContainer),
			ID:   "container-1",
			Data: []byte{0xff, 0xff, 0xff, 0xff},
		}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		assert.Error(t, err)
		assert.Nil(t, event.Entity)
	})
}
