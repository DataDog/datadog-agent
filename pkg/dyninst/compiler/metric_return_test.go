// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestGenerateReturnMetricProbeHandlers(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "simple", cfg))
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	for _, value := range []string{"", `,"value":{"dsl":"@return","json":{"ref":"@return"}}`, `,"value":{"dsl":"@duration","json":{"ref":"@duration"}}`} {
		t.Run(value, func(t *testing.T) {
			kind := "GAUGE"
			if value == "" {
				kind = "COUNT"
			}
			probe, err := rcjson.UnmarshalProbe([]byte(`{"id":"p","type":"METRIC_PROBE","metricName":"m","kind":"` + kind + `","evaluateAt":"EXIT","where":{"methodName":"main.condReturnAndLocal"},"when":{"dsl":"@return > 0","json":{"gt":[{"ref":"@return"},0]}}` + value + `}`))
			require.NoError(t, err)
			prog, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true))
			require.NoError(t, err)
			compiled, err := GenerateProgram(prog)
			require.NoError(t, err)
			var entry, exit, recovery int
			for _, f := range compiled.Functions {
				pe, ok := f.ID.(ProcessEvent)
				if !ok {
					continue
				}
				if prog.Probes[pe.ProbeID].GetKind() == ir.ProbeKindRuntimeRecovery {
					require.False(t, pe.IsMetric)
					recovery++
					continue
				}
				require.True(t, pe.IsMetric)
				require.Equal(t, ThrottleNone, pe.ThrottleMode)
				var updates int
				for _, op := range f.Ops {
					if _, ok := op.(MetricUpdateOp); ok {
						updates++
					}
				}
				if pe.EventKind == ir.EventKindEntry {
					entry++
					require.True(t, pe.HasAssociatedReturn)
					require.Zero(t, updates)
				} else {
					exit++
					require.Equal(t, ir.EventKindReturn, pe.EventKind)
					require.Equal(t, 1, updates)
				}
			}
			require.Positive(t, entry)
			require.Positive(t, exit)
			require.Positive(t, recovery)
		})
	}
}
