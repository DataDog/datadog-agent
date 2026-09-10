// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy || windows

package bomconvert

import (
	"testing"

	"github.com/CycloneDX/cyclonedx-go"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

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

func TestConvertBOMWithBOMRefMapping(t *testing.T) {
	for _, simplify := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserved", true: "simplified"}[simplify], func(t *testing.T) {
			components := []cyclonedx.Component{
				{Name: "first", BOMRef: "raw-first"},
				{Name: "second", BOMRef: "raw-second"},
			}
			dependencies := []cyclonedx.Dependency{
				{Ref: "raw-first", Dependencies: &[]string{"raw-second"}},
			}
			assemblies := []cyclonedx.BOMReference{"raw-first"}
			compositionDependencies := []cyclonedx.BOMReference{"raw-second"}
			compositions := []cyclonedx.Composition{{
				Assemblies: &assemblies, Dependencies: &compositionDependencies,
			}}
			affects := []cyclonedx.Affects{{Ref: "raw-first"}}
			vulnerabilities := []cyclonedx.Vulnerability{{BOMRef: "raw-vulnerability", Affects: &affects}}
			bom := &cyclonedx.BOM{
				Components:      &components,
				Dependencies:    &dependencies,
				Compositions:    &compositions,
				Vulnerabilities: &vulnerabilities,
			}

			converted, mapping := ConvertBOMWithBOMRefMapping(bom, simplify)
			if converted == nil {
				t.Fatal("converted BOM is nil")
			}
			if len(converted.GetComponents()) != 2 || len(converted.GetDependencies()) != 1 {
				t.Fatalf("converted BOM has components/dependencies %d/%d, want 2/1", len(converted.GetComponents()), len(converted.GetDependencies()))
			}

			want := map[string]string{
				"raw-first":         "raw-first",
				"raw-second":        "raw-second",
				"raw-vulnerability": "raw-vulnerability",
			}
			if simplify {
				want = map[string]string{"raw-first": "1", "raw-second": "2", "raw-vulnerability": "3"}
			}
			assert.Equal(t, want, mapping)
			assert.Equal(t, mapping["raw-first"], converted.GetComponents()[0].GetBomRef())
			assert.Equal(t, mapping["raw-second"], converted.GetComponents()[1].GetBomRef())
			assert.Equal(t, mapping["raw-first"], converted.GetDependencies()[0].GetRef())
			if len(converted.GetDependencies()[0].GetDependencies()) != 1 {
				t.Fatalf("nested dependencies = %d, want 1", len(converted.GetDependencies()[0].GetDependencies()))
			}
			assert.Equal(t, mapping["raw-second"], converted.GetDependencies()[0].GetDependencies()[0].GetRef())
			assert.Equal(t, []string{mapping["raw-first"]}, converted.GetCompositions()[0].GetAssemblies())
			assert.Equal(t, []string{mapping["raw-second"]}, converted.GetCompositions()[0].GetDependencies())
			assert.Equal(t, mapping["raw-vulnerability"], converted.GetVulnerabilities()[0].GetBomRef())
			assert.Equal(t, mapping["raw-first"], converted.GetVulnerabilities()[0].GetAffects()[0].GetRef())
		})
	}
}
