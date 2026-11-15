// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestGenericClosureLookup(t *testing.T) {
	const target = "crypto/internal/fips140hash.UnwrapNew[...].func1"
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		if cfg.GOTOOLCHAIN != "go1.27.3" {
			continue
		}
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "sample", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			symbols, err := obj.Symbols()
			require.NoError(t, err)
			var bodies []ir.PCRange
			var exactTarget string
			for _, symbol := range symbols {
				if strings.HasPrefix(symbol.Name, "crypto/internal/fips140hash.UnwrapNew[") && strings.HasSuffix(symbol.Name, "].func1") {
					bodies = append(bodies, ir.PCRange{symbol.Value, symbol.Value + symbol.Size})
					if exactTarget == "" {
						exactTarget = symbol.Name
					}
					require.Equal(t, exactTarget, symbol.Name)
				}
			}
			require.Len(t, bodies, 7)
			probes := []ir.ProbeDefinition{
				&rcjson.SnapshotProbe{LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{ID: "method", Where: &rcjson.Where{MethodName: target}}}},
				&rcjson.SnapshotProbe{LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{ID: "line", Where: &rcjson.Where{MethodName: target, SourceFile: "crypto/internal/fips140hash/hash.go", Lines: []string{"33"}}}}},
			}
			probes = append(probes,
				&rcjson.SnapshotProbe{LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{ID: "exactMethod", Where: &rcjson.Where{MethodName: exactTarget}}}},
				&rcjson.SnapshotProbe{LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{ID: "exactLine", Where: &rcjson.Where{MethodName: exactTarget, SourceFile: "crypto/internal/fips140hash/hash.go", Lines: []string{"33"}}}}},
			)
			program, err := irgen.GenerateIR(1, obj, probes)
			require.NoError(t, err)
			require.Empty(t, program.Issues)
			for _, id := range []string{"method", "line", "exactMethod", "exactLine"} {
				found := false
				for _, probe := range program.Probes {
					if probe.GetID() != id {
						continue
					}
					found = true
					require.Len(t, probe.Instances, len(bodies))
					for _, body := range bodies {
						points := 0
						for _, instance := range probe.Instances {
							for _, event := range instance.Events {
								if event.Kind != ir.EventKindEntry && event.Kind != ir.EventKindLine {
									continue
								}
								for _, point := range event.InjectionPoints {
									if body[0] <= point.PC && point.PC < body[1] {
										points++
									}
								}
							}
						}
						require.Equal(t, 1, points, "%s body %#x", id, body)
					}
				}
				require.True(t, found, "probe %s", id)
			}
		})
	}
}

func TestGenericShapeInstances(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "simple", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			probe := &rcjson.SnapshotProbe{LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{ID: "identity", Where: &rcjson.Where{MethodName: "main.genericIdentity[...]"}}}}
			program, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe})
			require.NoError(t, err)
			require.Empty(t, program.Issues)
			found := false
			for _, p := range program.Probes {
				if p.GetID() != "identity" {
					continue
				}
				found = true
				require.Len(t, p.Instances, 2)
			}
			require.True(t, found)
		})
	}
}
