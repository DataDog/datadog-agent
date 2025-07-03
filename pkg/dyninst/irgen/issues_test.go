// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestMissingProbeIssue(t *testing.T) {
	const testProg = "simple"
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	bin := testprogs.MustGetBinary(t, testProg, cfg)
	probes := testprogs.MustGetProbeDefinitions(t, testProg)

	f, err := safeelf.Open(bin)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	obj, err := object.NewElfObject(f)
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
