// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// makeProp builds a cyclonedx Property pointer for use in tests.
func makeProp(name, value string) *cyclonedx_v1_4.Property {
	return &cyclonedx_v1_4.Property{Name: name, Value: &value}
}

// makeComp builds a cyclonedx Component for use in tests.
func makeComp(name, version string, props ...*cyclonedx_v1_4.Property) *cyclonedx_v1_4.Component {
	return &cyclonedx_v1_4.Component{Name: name, Version: version, Properties: props}
}

// makeBOMBytes compresses the given components into a valid gzip+proto BOM.
func makeBOMBytes(t *testing.T, comps ...*cyclonedx_v1_4.Component) []byte {
	t.Helper()
	data, err := compressSBOMBom(&cyclonedx_v1_4.Bom{Components: comps})
	require.NoError(t, err)
	return data
}

// propValue returns the string value of a Property, or "" if nil.
func propValue(p *cyclonedx_v1_4.Property) string {
	if p == nil || p.Value == nil {
		return ""
	}
	return *p.Value
}

// findProp returns the first property with the given name, or nil.
func findProp(props []*cyclonedx_v1_4.Property, name string) *cyclonedx_v1_4.Property {
	for _, p := range props {
		if p != nil && p.Name == name {
			return p
		}
	}
	return nil
}

func container1(testTime time.Time) Container {
	return Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo1",
		},
		EntityMeta: EntityMeta{
			Name:      "foo1-name",
			Namespace: "",
		},
		Ports: []ContainerPort{
			{
				Name:     "port1",
				Port:     42000,
				Protocol: "tcp",
			},
			{
				Port:     42001,
				Protocol: "udp",
			},
			{
				Port: 42002,
			},
			{
				Port: 42004,
			},
		},
		State: ContainerState{
			Running:    true,
			CreatedAt:  testTime,
			StartedAt:  testTime,
			FinishedAt: time.Time{},
			Health:     ContainerHealthUnknown,
		},
		CollectorTags: []string{"tag1", "tag2"},
		EnvVars: map[string]string{
			"DD_SERVICE-partial": "my-svc",
		},
	}
}

func container2() Container {
	return Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo1",
		},
		EntityMeta: EntityMeta{
			Name:      "foo1-name",
			Namespace: "",
		},
		Ports: []ContainerPort{
			{
				Port:     42000,
				Protocol: "tcp",
			},
			{
				Port:     42001,
				Protocol: "udp",
			},
			{
				Port:     42002,
				Protocol: "tcp",
			},
			{
				Port: 42003,
			},
			{
				Port:     42004,
				HostPort: 42004,
			},
		},
		State: ContainerState{
			CreatedAt:  time.Time{},
			StartedAt:  time.Time{},
			FinishedAt: time.Time{},
			ExitCode:   pointer.Ptr(int64(100)),
			Health:     ContainerHealthHealthy,
		},
		CollectorTags: []string{"tag3"},
		EnvVars: map[string]string{
			"DD_SERVICE-partial": "my-svc",
			"DD_ENV-extra":       "prod",
		},
	}
}

func TestMerge(t *testing.T) {
	testTime := time.Now()

	expectedContainer := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "foo1",
		},
		EntityMeta: EntityMeta{
			Name:      "foo1-name",
			Namespace: "",
		},
		State: ContainerState{
			Running:    true,
			CreatedAt:  testTime,
			StartedAt:  testTime,
			FinishedAt: time.Time{},
			ExitCode:   pointer.Ptr(int64(100)),
			Health:     ContainerHealthHealthy,
		},
		EnvVars: map[string]string{
			"DD_SERVICE-partial": "my-svc",
			"DD_ENV-extra":       "prod",
		},
	}

	expectedPorts := []ContainerPort{
		{
			Name:     "port1",
			Port:     42000,
			Protocol: "tcp",
		},
		{
			Port:     42001,
			Protocol: "udp",
		},
		{
			Port: 42002,
		},
		{
			Port:     42002,
			Protocol: "tcp",
		},
		{
			Port: 42003,
		},
		{
			Port:     42004,
			HostPort: 42004,
		},
	}

	expectedTags := []string{"tag1", "tag2", "tag3"}

	// Test merging both ways
	fromSource1 := container1(testTime)
	fromSource2 := container2()
	err := merge(&fromSource1, &fromSource2)
	assert.NoError(t, err)
	assert.ElementsMatch(t, expectedPorts, fromSource1.Ports)
	assert.ElementsMatch(t, expectedTags, fromSource1.CollectorTags)
	fromSource1.Ports = nil
	fromSource1.CollectorTags = nil
	assert.Equal(t, expectedContainer, fromSource1)

	fromSource1 = container1(testTime)
	fromSource2 = container2()
	err = merge(&fromSource2, &fromSource1)
	assert.NoError(t, err)
	assert.ElementsMatch(t, expectedPorts, fromSource2.Ports)
	assert.ElementsMatch(t, expectedTags, fromSource2.CollectorTags)
	fromSource2.Ports = nil
	fromSource2.CollectorTags = nil
	assert.Equal(t, expectedContainer, fromSource2)

	// Test merging nil slice in src/dst
	fromSource1 = container1(testTime)
	fromSource2 = container2()
	fromSource2.Ports = nil
	err = merge(&fromSource1, &fromSource2)
	assert.NoError(t, err)
	assert.ElementsMatch(t, container1(testTime).Ports, fromSource1.Ports)

	fromSource1 = container1(testTime)
	fromSource2 = container2()
	fromSource2.Ports = nil
	err = merge(&fromSource2, &fromSource1)
	assert.NoError(t, err)
	assert.ElementsMatch(t, container1(testTime).Ports, fromSource2.Ports)
}

// TestMergeCompressedSBOMs verifies the BOM-level merge logic used by
// ContainerImageMetadata.Merge.
//
// computeCache() always merges sources alphabetically: remote_sbom_collector
// (dst) before runtime/Trivy (src). mergeCompressedSBOMs must therefore:
//   - Select scan metadata (Status, GenerationMethod, …) from the strictly
//     higher-priority source; dst wins on a tie.
//   - Build the component list from the primary BOM, enriched with runtime
//     properties (e.g. LastSeenRunning) from matching secondary components.
func TestMergeCompressedSBOMs(t *testing.T) {
	t.Run("nil handling", func(t *testing.T) {
		assert.Nil(t, mergeCompressedSBOMs(nil, nil))

		src := &CompressedSBOM{Bom: makeBOMBytes(t), Status: Success}
		assert.Same(t, src, mergeCompressedSBOMs(nil, src), "dst nil: src returned as-is")

		dst := &CompressedSBOM{Bom: makeBOMBytes(t), Status: Success}
		assert.Same(t, dst, mergeCompressedSBOMs(dst, nil), "src nil: dst returned as-is")
	})

	t.Run("src strictly higher status wins metadata", func(t *testing.T) {
		// Mirrors the "system-probe fires first" scenario:
		// dst=remote_sbom_collector (Status=""), src=runtime/Trivy (Status=Success).
		dst := &CompressedSBOM{Bom: makeBOMBytes(t), Status: ""}
		src := &CompressedSBOM{
			Bom:              makeBOMBytes(t),
			Status:           Success,
			GenerationMethod: "trivy",
		}
		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		assert.Equal(t, Success, result.Status)
		assert.Equal(t, "trivy", result.GenerationMethod)
	})

	t.Run("dst wins on equal status", func(t *testing.T) {
		// Both sources at Success — dst is the alphabetically-first source and
		// already holds the previously-merged result; it must keep its metadata.
		dst := &CompressedSBOM{
			Bom:              makeBOMBytes(t),
			Status:           Success,
			GenerationMethod: "dst-method",
		}
		src := &CompressedSBOM{
			Bom:              makeBOMBytes(t),
			Status:           Success,
			GenerationMethod: "src-method",
		}
		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		assert.Equal(t, "dst-method", result.GenerationMethod)
	})

	t.Run("runtime properties merged into primary component list", func(t *testing.T) {
		// Primary (Trivy, Status=Success): libc@2.31 — no runtime props.
		// Secondary (system-probe, Status=""): libc@2.31 — has LastSeenRunning.
		// dst = remote_sbom_collector (system-probe), src = runtime (Trivy).
		// src priority > dst priority, so src is primary and dst is secondary.
		libcTrivy := makeComp("libc", "2.31")
		libcRuntime := makeComp("libc", "2.31", makeProp("LastSeenRunning", "1234567890"))

		dst := &CompressedSBOM{Bom: makeBOMBytes(t, libcRuntime), Status: ""}
		src := &CompressedSBOM{Bom: makeBOMBytes(t, libcTrivy), Status: Success}

		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		assert.Equal(t, Success, result.Status)

		resultBOM, err := decompressSBOMBom(result.Bom)
		require.NoError(t, err)
		require.Len(t, resultBOM.Components, 1)
		assert.Equal(t, "1234567890", propValue(findProp(resultBOM.Components[0].Properties, "LastSeenRunning")))
	})

	t.Run("epoch-prefixed version normalised for matching", func(t *testing.T) {
		// Primary has libc@2.31, secondary has libc@1:2.31.
		// normalizeComponentVersion must strip the epoch so they match.
		libcTrivy := makeComp("libc", "2.31")
		libcRuntime := makeComp("libc", "1:2.31", makeProp("LastSeenRunning", "999"))

		dst := &CompressedSBOM{Bom: makeBOMBytes(t, libcRuntime), Status: ""}
		src := &CompressedSBOM{Bom: makeBOMBytes(t, libcTrivy), Status: Success}

		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		resultBOM, err := decompressSBOMBom(result.Bom)
		require.NoError(t, err)
		require.Len(t, resultBOM.Components, 1)
		assert.Equal(t, "999", propValue(findProp(resultBOM.Components[0].Properties, "LastSeenRunning")),
			"epoch normalisation should allow version matching")
	})

	t.Run("secondary-only components not added to result", func(t *testing.T) {
		// Primary has only libc; secondary has libc + bash.
		// Result must only contain primary's component set.
		libcTrivy := makeComp("libc", "2.31")
		libcRuntime := makeComp("libc", "2.31")
		bashRuntime := makeComp("bash", "5.0")

		dst := &CompressedSBOM{Bom: makeBOMBytes(t, libcRuntime, bashRuntime), Status: ""}
		src := &CompressedSBOM{Bom: makeBOMBytes(t, libcTrivy), Status: Success}

		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		resultBOM, err := decompressSBOMBom(result.Bom)
		require.NoError(t, err)
		require.Len(t, resultBOM.Components, 1, "secondary-only components must not be added")
		assert.Equal(t, "libc", resultBOM.Components[0].Name)
	})

	t.Run("existing non-runtime primary property not overridden by secondary", func(t *testing.T) {
		libcWithPrimary := makeComp("libc", "2.31", makeProp("CustomProp", "primary-value"))
		libcWithSecondary := makeComp("libc", "2.31", makeProp("CustomProp", "secondary-value"))

		dst := &CompressedSBOM{Bom: makeBOMBytes(t, libcWithSecondary), Status: ""}
		src := &CompressedSBOM{Bom: makeBOMBytes(t, libcWithPrimary), Status: Success}

		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		resultBOM, err := decompressSBOMBom(result.Bom)
		require.NoError(t, err)
		assert.Equal(t, "primary-value", propValue(findProp(resultBOM.Components[0].Properties, "CustomProp")),
			"non-runtime primary property must not be overridden by secondary")
	})

	t.Run("runtime properties always overridden by secondary", func(t *testing.T) {
		// Simulate a stale merged SBOM reused as the primary (docker/runtime source):
		// primary already carries LastSeenRunning from a previous merge, but system-probe
		// has sent a newer value. The secondary (system-probe) must win for runtime props.
		libcStale := makeComp("libc", "2.31",
			makeProp(SBOMLastSeenRunningProperty, "old-timestamp"),
			makeProp(SBOMRunningAsRootProperty, "false"),
			makeProp(SBOMHasSetSuidBitProperty, "false"),
		)
		libcUpdated := makeComp("libc", "2.31",
			makeProp(SBOMLastSeenRunningProperty, "new-timestamp"),
			makeProp(SBOMRunningAsRootProperty, "true"),
			makeProp(SBOMHasSetSuidBitProperty, "true"),
		)

		dst := &CompressedSBOM{Bom: makeBOMBytes(t, libcUpdated), Status: ""}
		src := &CompressedSBOM{Bom: makeBOMBytes(t, libcStale), Status: Success}

		result := mergeCompressedSBOMs(dst, src)
		require.NotNil(t, result)
		resultBOM, err := decompressSBOMBom(result.Bom)
		require.NoError(t, err)
		require.Len(t, resultBOM.Components, 1)
		props := resultBOM.Components[0].Properties
		assert.Equal(t, "new-timestamp", propValue(findProp(props, SBOMLastSeenRunningProperty)),
			"secondary LastSeenRunning must override stale primary value")
		assert.Equal(t, "true", propValue(findProp(props, SBOMRunningAsRootProperty)),
			"secondary RunningAsRoot must override stale primary value")
		assert.Equal(t, "true", propValue(findProp(props, SBOMHasSetSuidBitProperty)),
			"secondary HasSetSuidBit must override stale primary value")
	})
}

// TestContainerImageMetadataMerge_SBOM verifies that ContainerImageMetadata.Merge
// correctly delegates SBOM merging and preserves shallow-copy safety.
func TestContainerImageMetadataMerge_SBOM(t *testing.T) {
	trivySBOM := &CompressedSBOM{
		Bom:              makeBOMBytes(t, makeComp("libc", "2.31")),
		Status:           Success,
		GenerationMethod: "trivy",
	}

	// dst has no SBOM — src metadata must propagate.
	dst := &ContainerImageMetadata{
		EntityID: EntityID{Kind: KindContainerImageMetadata, ID: "img1"},
		SBOM:     nil,
	}
	src := &ContainerImageMetadata{
		EntityID: EntityID{Kind: KindContainerImageMetadata, ID: "img1"},
		SBOM:     trivySBOM,
	}
	err := dst.Merge(src)
	require.NoError(t, err)
	require.NotNil(t, dst.SBOM)
	assert.Equal(t, Success, dst.SBOM.Status)
	assert.Equal(t, "trivy", dst.SBOM.GenerationMethod)

	// src must not be modified by the merge (shallow-copy safety).
	assert.Equal(t, trivySBOM, src.SBOM, "src SBOM must not be modified by merge")
}
