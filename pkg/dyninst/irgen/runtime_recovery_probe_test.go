// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

func TestMaybeAddRuntimeRecoveryProbe(t *testing.T) {
	function := &rcjson.Where{MethodName: "main.f"}
	line := &rcjson.Where{SourceFile: "main.go", Lines: []string{"10"}}
	metric := func(where *rcjson.Where, evaluateAt string) ir.ProbeDefinition {
		return &rcjson.MetricProbe{ProbeCommon: rcjson.ProbeCommon{
			Where: where, EvaluateAt: evaluateAt,
		}}
	}
	logProbe := &rcjson.LogProbe{LogProbeCommon: rcjson.LogProbeCommon{
		ProbeCommon: rcjson.ProbeCommon{Where: function},
	}}
	for _, tc := range []struct {
		name  string
		defs  []ir.ProbeDefinition
		added bool
	}{
		{"empty", nil, false},
		{"default_metric", []ir.ProbeDefinition{metric(function, "")}, false},
		{"entry_metric", []ir.ProbeDefinition{metric(function, "ENTRY")}, false},
		{"exit_metric", []ir.ProbeDefinition{metric(function, "EXIT")}, true},
		{"lowercase_exit_metric", []ir.ProbeDefinition{metric(function, "exit")}, true},
		{"line_metric", []ir.ProbeDefinition{metric(line, "EXIT")}, false},
		{"function_log", []ir.ProbeDefinition{logProbe}, true},
		{"entry_metric_and_log", []ir.ProbeDefinition{metric(function, "ENTRY"), logProbe}, true},
		{"entry_and_exit_metrics", []ir.ProbeDefinition{metric(function, "ENTRY"), metric(function, "EXIT")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := maybeAddRuntimeRecoveryProbe(tc.defs)
			if !tc.added {
				require.Equal(t, tc.defs, got)
				return
			}
			require.Len(t, got, len(tc.defs)+1)
			require.Equal(t, tc.defs, got[:len(tc.defs)])
			require.Equal(t, ir.ProbeKindRuntimeRecovery, got[len(tc.defs)].GetKind())
		})
	}
}
