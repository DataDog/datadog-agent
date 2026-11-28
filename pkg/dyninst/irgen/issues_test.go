// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen_test

import (
	"debug/dwarf"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestZeroReferenceDepthDoesNotFailOtherProbes(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	bin := testprogs.MustGetBinary(t, "rc_tester", cfg)
	probes := testprogs.MustGetProbeDefinitions(t, "rc_tester")
	found := false
	for _, probe := range probes {
		if probe.GetID() == "http_handler" {
			depth := 0
			probe.(*rcjson.SnapshotProbe).Capture = &rcjson.Capture{MaxReferenceDepth: &depth}
			found = true
		}
	}
	require.True(t, found)
	obj, err := object.OpenElfFileWithDwarf(bin)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	program, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.Empty(t, program.Issues)
	for _, expected := range probes {
		require.True(t, slices.ContainsFunc(program.Probes, func(probe *ir.Probe) bool {
			return probe.ProbeDefinition.GetID() == expected.GetID()
		}))
	}
	verifyIR(t, program)
}

func TestMissingProbeIssue(t *testing.T) {
	const testProg = "simple"
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	bin := testprogs.MustGetBinary(t, testProg, cfg)
	probes := testprogs.MustGetProbeDefinitions(t, testProg)
	probes = slices.DeleteFunc(probes, func(p ir.ProbeDefinition) bool {
		return testprogs.HasIssueTag(p, cfg)
	})

	obj, err := object.OpenElfFileWithDwarf(bin)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	probes = probes[:len(probes):len(probes)] // so appends realloc

	t.Run("TargetNotFoundInBinary", func(t *testing.T) {
		probesWithNonExistent := append(probes,
			&rcjson.SnapshotProbe{
				LogProbeCommon: rcjson.LogProbeCommon{
					ProbeCommon: rcjson.ProbeCommon{
						ID: "non_existent_probe",
						Where: &rcjson.Where{
							MethodName: "main.doesNotExist",
						},
						Type: rcjson.TypeLogProbe.String(),
					},
				},
			})
		p, err := irgen.GenerateIR(1, obj, probesWithNonExistent)
		require.NoError(t, err)

		require.Len(t, p.Issues, 1)
		require.Equal(t, ir.IssueKindTargetNotFoundInBinary, p.Issues[0].Kind)
		require.Equal(t, "target for probe not found in binary", p.Issues[0].Message)
	})

	t.Run("InvalidProbeDefinition", func(t *testing.T) {
		probesWithInvalid := append(probes,
			&rcjson.SnapshotProbe{
				LogProbeCommon: rcjson.LogProbeCommon{
					ProbeCommon: rcjson.ProbeCommon{
						ID:   "invalid_probe_with_no_method",
						Type: rcjson.TypeLogProbe.String(),
					},
				},
			})
		p, err := irgen.GenerateIR(1, obj, probesWithInvalid)
		require.NoError(t, err)
		require.Len(t, p.Issues, 1)
		require.Equal(t, ir.IssueKindInvalidProbeDefinition, p.Issues[0].Kind)
		require.Equal(t, "no where clause specified", p.Issues[0].Message)
	})
}

func TestLineProbeSourceFile(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	bin := testprogs.MustGetBinary(t, "rc_tester", cfg)
	obj, err := object.OpenElfFileWithDwarf(bin)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	for _, tc := range []struct {
		sourceFile string
		wantIssue  string
	}{
		{sourceFile: "main.go"},
		{sourceFile: "rc_tester/main.go"},
		{sourceFile: "github.com/DataDog/datadog-agent/pkg/dyninst/testprogs/progs/rc_tester/main.go"},
		{sourceFile: "handler.go", wantIssue: "no code for line 85 of source file"},
		{sourceFile: "/srv/app/handler.go", wantIssue: "no code for line 85 of source file"},
		{sourceFile: "other/main.go", wantIssue: "no code for line 85 of source file"},
		{sourceFile: "main.go/handler.go/..", wantIssue: "invalid source file"},
	} {
		t.Run(tc.sourceFile, func(t *testing.T) {
			probe := &rcjson.SnapshotProbe{
				LogProbeCommon: rcjson.LogProbeCommon{
					ProbeCommon: rcjson.ProbeCommon{
						ID:   "line_probe",
						Type: rcjson.TypeLogProbe.String(),
						Where: &rcjson.Where{
							MethodName: "main.HandleHTTP",
							SourceFile: tc.sourceFile,
							Lines:      []string{"85"},
						},
					},
				},
			}
			program, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe})
			if tc.wantIssue == "" {
				require.NoError(t, err)
				require.Empty(t, program.Issues)
				require.Len(t, program.Probes, 1)
				for _, inst := range program.Probes[0].Instances {
					for _, ev := range inst.Events {
						for _, ip := range ev.InjectionPoints {
							file, line := lineAtPC(t, obj.DwarfData(), ip.PC)
							require.True(t, strings.HasSuffix(file, "/rc_tester/main.go"), file)
							require.Equal(t, 85, line)
						}
					}
				}
				return
			}
			var noProbes *ir.NoSuccessfulProbesError
			require.ErrorAs(t, err, &noProbes)
			require.Len(t, noProbes.Issues, 1)
			require.Equal(t, ir.IssueKindInvalidProbeDefinition, noProbes.Issues[0].Kind)
			require.Contains(t, noProbes.Issues[0].Message, tc.wantIssue)
		})
	}
}

func lineAtPC(t *testing.T, d *dwarf.Data, pc uint64) (string, int) {
	r := d.Reader()
	unit, err := r.SeekPC(pc)
	require.NoError(t, err)
	lr, err := d.LineReader(unit)
	require.NoError(t, err)
	var entry dwarf.LineEntry
	require.NoError(t, lr.SeekPC(pc, &entry))
	return entry.File.Name, entry.Line
}
