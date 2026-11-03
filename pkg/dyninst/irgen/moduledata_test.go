// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestModuledataTypeBounds(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "simple", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			probes := testprogs.MustGetProbeDefinitions(t, "simple")
			program, err := irgen.GenerateIR(1, obj, probes[:1])
			require.NoError(t, err)
			require.Empty(t, program.Issues)
			require.NotZero(t, program.GoMapHashInfo.UseAeshashAddr)
			require.NotZero(t, program.GoMapHashInfo.AeskeyschedAddr)
			info := program.GoModuledataInfo
			require.Equal(t, uint32(296), info.TypesOffset)
			expectedEnd := uint32(304)
			if obj.Section(".go.type") != nil {
				expectedEnd = 312
			}
			require.Equal(t, expectedEnd, info.ETypesOffset)
			compiled, err := compiler.GenerateProgram(program)
			require.NoError(t, err)
			require.Equal(t, info, compiled.GoModuledataInfo)
			md, err := object.ParseModuleData(obj)
			require.NoError(t, err)
			require.Equal(t, md.FirstModuleData, info.FirstModuledataAddr)
			for _, field := range []struct {
				offset uint32
				value  uint64
			}{
				{info.TypesOffset, md.Types}, {info.ETypesOffset, md.ETypes},
			} {
				addr := info.FirstModuledataAddr + uint64(field.offset)
				found := false
				for _, section := range obj.SectionHeaders() {
					if section.Addr <= addr && addr-section.Addr < section.Size {
						data, err := obj.SectionDataRange(section, addr-section.Addr, 8)
						require.NoError(t, err)
						require.Equal(t, field.value, binary.LittleEndian.Uint64(data.Data()))
						require.NoError(t, data.Close())
						found = true
						break
					}
				}
				require.True(t, found)
			}
		})
	}
}

func TestSourceErrorLocal(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "sample", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			probes := testprogs.MustGetProbeDefinitions(t, "sample")
			foundProbe := false
			for _, probe := range probes {
				if probe.GetID() != "testErrorAtLine" {
					continue
				}
				foundProbe = true
				program, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe})
				require.NoError(t, err)
				require.Empty(t, program.Issues)
				found := false
				for _, subprogram := range program.Subprograms {
					if subprogram.Name != "main.testErrorAtLine" {
						continue
					}
					for _, variable := range subprogram.Variables {
						require.NotContains(t, variable.Name, ".")
						if variable.Name == "err" {
							require.Equal(t, "error", variable.Type.GetName())
							found = true
						}
					}
				}
				require.True(t, found)
			}
			require.True(t, foundProbe, "testErrorAtLine probe must be defined")
		})
	}
}
