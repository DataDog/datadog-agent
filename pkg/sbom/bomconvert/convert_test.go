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

func TestConvertBOMWithRefs(t *testing.T) {
	components := []cyclonedx.Component{{BOMRef: "pkg:deb/debian/bash@5.2.15-2", Name: "bash"}, {BOMRef: "pkg:npm/express@4.18.2", Name: "express"}}
	for _, simplify := range []bool{false, true} {
		bom, ref := ConvertBOMWithRefs(&cyclonedx.BOM{SpecVersion: cyclonedx.SpecVersion1_6, Components: &components}, simplify)
		for i, c := range bom.GetComponents() {
			assert.Equal(t, c.GetBomRef(), ref(components[i].BOMRef), "simplified %v", simplify)
		}
		if simplify {
			assert.Equal(t, "", ref("pkg:npm/left-out@1.0.0"))
		}
	}
	bom, ref := ConvertBOMWithRefs(nil, true)
	assert.Nil(t, bom)
	assert.Equal(t, "", ref("x"))
}
