// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package bomconvert

import (
	"testing"

	"github.com/CycloneDX/cyclonedx-go"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestConvertBOMNil(t *testing.T) {
	assert.Nil(t, ConvertBOM(nil, false))
}

func TestConvertBOMBasic(t *testing.T) {
	hashes := []cyclonedx.Hash{
		{Algorithm: cyclonedx.HashAlgoSHA256, Value: "abc123"},
	}
	components := []cyclonedx.Component{
		{
			Type:       cyclonedx.ComponentTypeLibrary,
			Name:       "my-lib",
			Version:    "1.0.0",
			PackageURL: "pkg:golang/my-lib@1.0.0",
			BOMRef:     "ref-1",
			Hashes:     &hashes,
		},
		{
			Type:    cyclonedx.ComponentTypeApplication,
			Name:    "my-app",
			Version: "2.0.0",
			BOMRef:  "ref-2",
		},
	}
	deps := []cyclonedx.Dependency{
		{Ref: "ref-2", Dependencies: &[]string{"ref-1"}},
	}
	bom := &cyclonedx.BOM{
		Version:      1,
		Components:   &components,
		Dependencies: &deps,
	}

	result := ConvertBOM(bom, false)
	assert.NotNil(t, result)
	assert.Equal(t, cyclonedx.SpecVersion1_4.String(), result.SpecVersion)
	assert.Len(t, result.Components, 2)
	assert.Equal(t, "my-lib", result.Components[0].Name)
	assert.Equal(t, "1.0.0", result.Components[0].Version)
	assert.Equal(t, "pkg:golang/my-lib@1.0.0", *result.Components[0].Purl)
	assert.Len(t, result.Components[0].Hashes, 1)

	assert.Len(t, result.Dependencies, 1)
	assert.Equal(t, "ref-2", result.Dependencies[0].Ref)
}

func TestConvertBOMWithSimplifiedBomRefs(t *testing.T) {
	components := []cyclonedx.Component{
		{
			Type:    cyclonedx.ComponentTypeLibrary,
			Name:    "lib-a",
			Version: "1.0",
			BOMRef:  "very-long-bomref-identifier-lib-a",
		},
		{
			Type:    cyclonedx.ComponentTypeLibrary,
			Name:    "lib-b",
			Version: "2.0",
			BOMRef:  "very-long-bomref-identifier-lib-b",
		},
	}
	deps := []cyclonedx.Dependency{
		{Ref: "very-long-bomref-identifier-lib-b", Dependencies: &[]string{"very-long-bomref-identifier-lib-a"}},
	}
	bom := &cyclonedx.BOM{
		Version:      1,
		Components:   &components,
		Dependencies: &deps,
	}

	result := ConvertBOM(bom, true)
	assert.NotNil(t, result)

	// BOM refs should be simplified to short numeric strings
	ref1 := *result.Components[0].BomRef
	ref2 := *result.Components[1].BomRef
	assert.NotEqual(t, "very-long-bomref-identifier-lib-a", ref1)
	assert.NotEqual(t, "very-long-bomref-identifier-lib-b", ref2)

	// Dependencies should use the same simplified refs
	assert.Equal(t, ref2, result.Dependencies[0].Ref)
	assert.Equal(t, ref1, result.Dependencies[0].Dependencies[0].Ref)
}

func FuzzConvertBOM(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte, simplifyBomRefMapping bool) {
		f := fuzz.NewFromGoFuzz(data).NilChance(0.8).NumElements(0, 2)

		var bom cyclonedx.BOM
		f.Fuzz(&bom)
		bom.SpecVersion = cyclonedx.SpecVersion1_6

		pb := ConvertBOM(&bom, simplifyBomRefMapping)
		_, err := proto.Marshal(pb)

		assert.Nil(t, err)
		assert.Equal(t, pb.SpecVersion, cyclonedx.SpecVersion1_4.String())
	})
}
