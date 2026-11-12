// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestMetricExpressionEvent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		event      ir.EventKind
		role       ir.VariableRole
		kind       reflect.Kind
		atReturn   bool
		available  bool
		register   bool
		wantReason string
	}{
		{name: "entry_parameter", event: ir.EventKindEntry, role: ir.VariableRoleParameter},
		{name: "entry_float_register", event: ir.EventKindEntry, role: ir.VariableRoleParameter, kind: reflect.Float64, available: true, register: true, wantReason: "passed in a floating-point register"},
		{name: "entry_float_memory", event: ir.EventKindEntry, role: ir.VariableRoleParameter, kind: reflect.Float64, available: true},
		{name: "entry_local", event: ir.EventKindEntry, role: ir.VariableRoleLocal, wantReason: "not available at function entry"},
		{name: "line_local", event: ir.EventKindLine, role: ir.VariableRoleLocal, available: true},
		{name: "line_unavailable", event: ir.EventKindLine, role: ir.VariableRoleLocal, wantReason: "not available at this line"},
		{name: "line_float_register", event: ir.EventKindLine, role: ir.VariableRoleLocal, kind: reflect.Float32, available: true, register: true, wantReason: "floating-point register"},
		{name: "return_parameter", event: ir.EventKindReturn, role: ir.VariableRoleParameter, atReturn: true, available: true},
		{name: "return_local", event: ir.EventKindReturn, role: ir.VariableRoleLocal, atReturn: true, available: true},
		{name: "return_value", event: ir.EventKindReturn, role: ir.VariableRoleReturn, atReturn: true, available: true},
		{name: "return_unavailable", event: ir.EventKindReturn, role: ir.VariableRoleReturn, atReturn: true, wantReason: "not available at function return"},
		{name: "return_float_register", event: ir.EventKindReturn, role: ir.VariableRoleReturn, kind: reflect.Float64, atReturn: true, available: true, register: true, wantReason: "floating-point register"},
		{name: "duration", event: ir.EventKindReturn, role: ir.VariableRoleDuration, atReturn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &ir.Variable{
				Role: tc.role,
				Type: &ir.BaseType{GoTypeAttributes: ir.GoTypeAttributes{GoKind: tc.kind}},
			}
			if tc.available {
				var op ir.PieceOp = ir.Cfa{}
				if tc.register {
					op = ir.Register{}
				}
				v.Locations = []ir.Location{{Range: ir.PCRange{1, 3}, Pieces: []ir.Piece{{Size: 8, Op: op}}}}
			}
			inst := &ir.ProbeInstance{Events: []*ir.Event{{Kind: tc.event, InjectionPoints: []ir.InjectionPoint{{PC: 2}}}}}
			if tc.atReturn {
				inst.Events = append([]*ir.Event{{Kind: ir.EventKindEntry}}, inst.Events...)
			}
			kind, reason := metricExpressionEvent(inst, tc.atReturn, v)
			if tc.wantReason != "" {
				require.Contains(t, reason, tc.wantReason)
			} else {
				require.Empty(t, reason)
				require.Equal(t, tc.event, kind)
			}
		})
	}
}
