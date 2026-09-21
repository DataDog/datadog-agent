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
)

// ctxIfaceType builds a context.Context interface IR type for tests.
func ctxIfaceType() *ir.GoInterfaceType {
	return &ir.GoInterfaceType{
		TypeCommon: ir.TypeCommon{ID: 1, Name: "context.Context", ByteSize: 16},
	}
}

func ctxParam(locs []ir.Location) *ir.Variable {
	return &ir.Variable{
		Name:      "ctx",
		Type:      ctxIfaceType(),
		Role:      ir.VariableRoleParameter,
		Locations: locs,
	}
}

func TestResolveTraceIDSource(t *testing.T) {
	const pc = 0x1000

	t.Run("nil subprogram", func(t *testing.T) {
		require.Equal(t, traceIDSource{}, resolveTraceIDSource(nil, pc))
	})

	t.Run("register pair", func(t *testing.T) {
		sub := &ir.Subprogram{Variables: []*ir.Variable{ctxParam([]ir.Location{{
			Range: ir.PCRange{0x0, 0x2000},
			Pieces: []ir.Piece{
				{Size: 8, Op: ir.Register{RegNo: 3}},
				{Size: 8, Op: ir.Register{RegNo: 4}},
			},
		}})}}
		got := resolveTraceIDSource(sub, pc)
		require.Equal(t, traceIDSource{kind: 1, regTab: 3, regData: 4}, got)
	})

	t.Run("stack (CFA-relative)", func(t *testing.T) {
		sub := &ir.Subprogram{Variables: []*ir.Variable{ctxParam([]ir.Location{{
			Range:  ir.PCRange{0x0, 0x2000},
			Pieces: []ir.Piece{{Size: 16, Op: ir.Cfa{CfaOffset: -24}}},
		}})}}
		got := resolveTraceIDSource(sub, pc)
		require.Equal(t, traceIDSource{kind: 2, stackOffset: -24}, got)
	})

	t.Run("pc outside range falls back to none", func(t *testing.T) {
		sub := &ir.Subprogram{Variables: []*ir.Variable{ctxParam([]ir.Location{{
			Range: ir.PCRange{0x0, 0x100},
			Pieces: []ir.Piece{
				{Size: 8, Op: ir.Register{RegNo: 3}},
				{Size: 8, Op: ir.Register{RegNo: 4}},
			},
		}})}}
		require.Equal(t, traceIDSource{}, resolveTraceIDSource(sub, pc))
	})

	t.Run("register with shift is unsupported", func(t *testing.T) {
		sub := &ir.Subprogram{Variables: []*ir.Variable{ctxParam([]ir.Location{{
			Range: ir.PCRange{0x0, 0x2000},
			Pieces: []ir.Piece{
				{Size: 8, Op: ir.Register{RegNo: 3, Shift: 8}},
				{Size: 8, Op: ir.Register{RegNo: 4}},
			},
		}})}}
		require.Equal(t, traceIDSource{}, resolveTraceIDSource(sub, pc))
	})

	t.Run("non-parameter context is ignored", func(t *testing.T) {
		v := ctxParam([]ir.Location{{
			Range: ir.PCRange{0x0, 0x2000},
			Pieces: []ir.Piece{
				{Size: 8, Op: ir.Register{RegNo: 3}},
				{Size: 8, Op: ir.Register{RegNo: 4}},
			},
		}})
		v.Role = ir.VariableRoleLocal
		sub := &ir.Subprogram{Variables: []*ir.Variable{v}}
		require.Equal(t, traceIDSource{}, resolveTraceIDSource(sub, pc))
	})

	t.Run("non-context interface is ignored", func(t *testing.T) {
		v := ctxParam(nil)
		v.Type = &ir.GoInterfaceType{TypeCommon: ir.TypeCommon{Name: "io.Reader", ByteSize: 16}}
		v.Locations = []ir.Location{{
			Range: ir.PCRange{0x0, 0x2000},
			Pieces: []ir.Piece{
				{Size: 8, Op: ir.Register{RegNo: 3}},
				{Size: 8, Op: ir.Register{RegNo: 4}},
			},
		}}
		sub := &ir.Subprogram{Variables: []*ir.Variable{v}}
		require.Equal(t, traceIDSource{}, resolveTraceIDSource(sub, pc))
	})
}
