// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func BenchmarkGenerateIR(b *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(b)
	binary := testprogs.MustGetBinary(b, "sample", cfgs[0])
	gen := irGenerator{}

	for b.Loop() {
		_, err := gen.GenerateIR(1, binary, []ir.ProbeDefinition{
			probeDefinitionV1{},
			probeDefinitionV2{},
			symdbProbeDefinition{},
		})
		require.NoError(b, err)
	}
}
