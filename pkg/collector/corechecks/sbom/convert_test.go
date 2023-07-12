// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build trivy

package sbom

import (
	"testing"

	"github.com/CycloneDX/cyclonedx-go"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func FuzzConvertBOM(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		f := fuzz.NewFromGoFuzz(data).NilChance(0.8).NumElements(0, 2)

		var bom cyclonedx.BOM
		f.Fuzz(&bom)

		pb := convertBOM(&bom)
		_, err := proto.Marshal(pb)
		assert.Nil(t, err)
	})
}
