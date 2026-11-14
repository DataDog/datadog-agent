// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package compiler

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestMetricInstructionEncoding(t *testing.T) {
	check, ok := makeInstruction(nil, MetricCondErrorCheckOp{ProbeID: 0x01020304}).(staticInstruction)
	require.True(t, ok)
	require.Equal(t, OpcodeMetricCondErrorCheck, check.opcode)
	require.Equal(t, []byte{0x04, 0x03, 0x02, 0x01}, check.bytes)

	update, ok := makeInstruction(nil, MetricUpdateOp{
		ProbeID:     7,
		Kind:        MetricKindGauge,
		ValueKind:   MetricValueFloat,
		ValueSize:   8,
		ValueOffset: 0x10,
	}).(staticInstruction)
	require.True(t, ok)
	require.Equal(t, OpcodeMetricUpdate, update.opcode)
	require.Len(t, update.bytes, 11)
	require.Equal(t, uint32(7), binary.LittleEndian.Uint32(update.bytes[0:4]))
	require.Equal(t, []byte{byte(MetricKindGauge), byte(MetricValueFloat), 8}, update.bytes[4:7])
	require.Equal(t, uint32(0x10), binary.LittleEndian.Uint32(update.bytes[7:11]))
}

func TestMakeMetricUpdateOp(t *testing.T) {
	baseType := func(name string, size uint32, kind reflect.Kind) ir.Type {
		return &ir.BaseType{
			TypeCommon:       ir.TypeCommon{ID: 1, Name: name, ByteSize: size},
			GoTypeAttributes: ir.GoTypeAttributes{GoKind: kind},
		}
	}
	rootType := func(valueType ir.Type) *ir.EventRootType {
		rt := &ir.EventRootType{}
		if valueType != nil {
			rt.Expressions = []*ir.RootExpression{{
				Kind:       ir.RootExpressionKindMetricValue,
				Offset:     8,
				Expression: ir.Expression{Type: valueType},
			}}
		}
		return rt
	}
	probe := func(kind string) *ir.Probe {
		return &ir.Probe{ProbeDefinition: &rcjson.MetricProbe{
			ProbeCommon: rcjson.ProbeCommon{ID: "p"},
			Kind:        kind,
			MetricName:  "m",
		}}
	}
	for _, tc := range []struct {
		name      string
		kind      string
		valueType ir.Type
		want      MetricUpdateOp
		wantErr   string
	}{
		{
			name: "count without value",
			kind: "COUNT",
			want: MetricUpdateOp{ProbeID: 3, Kind: MetricKindCount, ValueKind: MetricValueNone},
		},
		{
			name:      "signed count",
			kind:      "COUNT",
			valueType: baseType("int32", 4, reflect.Int32),
			want: MetricUpdateOp{
				ProbeID: 3, Kind: MetricKindCount, ValueKind: MetricValueSigned,
				ValueSize: 4, ValueOffset: 8,
			},
		},
		{
			name:      "bool count",
			kind:      "COUNT",
			valueType: baseType("bool", 1, reflect.Bool),
			want: MetricUpdateOp{
				ProbeID: 3, Kind: MetricKindCount, ValueKind: MetricValueUnsigned,
				ValueSize: 1, ValueOffset: 8,
			},
		},
		{
			name:      "float gauge",
			kind:      "GAUGE",
			valueType: baseType("float64", 8, reflect.Float64),
			want: MetricUpdateOp{
				ProbeID: 3, Kind: MetricKindGauge, ValueKind: MetricValueFloat,
				ValueSize: 8, ValueOffset: 8,
			},
		},
		{
			name:    "gauge without value",
			kind:    "GAUGE",
			wantErr: "has no value",
		},
		{
			name:      "string value",
			kind:      "GAUGE",
			valueType: baseType("string", 16, reflect.String),
			wantErr:   "unsupported metric value type string",
		},
		{
			name:      "histogram",
			kind:      "HISTOGRAM",
			valueType: baseType("int", 8, reflect.Int),
			wantErr:   "unsupported kind Histogram",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op, err := makeMetricUpdateOp(probe(tc.kind), 3, rootType(tc.valueType))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, op)
		})
	}
}

// TestGenerateMetricProbeHandlers checks the shape of the event handlers that
// the compiler generates for metric probes.
func TestGenerateMetricProbeHandlers(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	bin := testprogs.MustGetBinary(t, "simple", cfg)
	obj, err := object.OpenElfFileWithDwarf(bin)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	parse := func(js string) ir.ProbeDefinition {
		p, err := rcjson.UnmarshalProbe([]byte(js))
		require.NoError(t, err)
		return p
	}
	probes := []ir.ProbeDefinition{
		parse(`{"id": "calls", "type": "METRIC_PROBE", "kind": "COUNT", "metricName": "calls",
			"where": {"methodName": "main.condInt64"}}`),
		parse(`{"id": "sum", "type": "METRIC_PROBE", "kind": "COUNT", "metricName": "sum",
			"where": {"methodName": "main.condInt64"},
			"value": {"dsl": "x", "json": {"ref": "x"}},
			"when": {"dsl": "tag == \"match\"", "json": {"eq": [{"ref": "tag"}, "match"]}}}`),
	}
	prog, err := irgen.GenerateIR(1, obj, probes, irgen.WithMetricProbes(true))
	require.NoError(t, err)
	require.Empty(t, prog.Issues)
	compiled, err := GenerateProgram(prog)
	require.NoError(t, err)

	probeIdx := make(map[string]uint32)
	for i, p := range prog.Probes {
		probeIdx[p.GetID()] = uint32(i)
	}
	handlers := make(map[uint32][]Op)
	for _, f := range compiled.Functions {
		pe, ok := f.ID.(ProcessEvent)
		if !ok {
			continue
		}
		require.True(t, pe.IsMetric)
		require.Equal(t, ThrottleNone, pe.ThrottleMode)
		require.False(t, pe.HasAssociatedReturn)
		handlers[pe.ProbeID] = f.Ops
	}
	require.Len(t, handlers, 2)

	// Without a condition or a value: prepare the root, count the hit.
	callsOps := handlers[probeIdx["calls"]]
	require.Len(t, callsOps, 3)
	require.IsType(t, PrepareEventRootOp{}, callsOps[0])
	require.Equal(t, MetricUpdateOp{
		ProbeID: probeIdx["calls"], Kind: MetricKindCount, ValueKind: MetricValueNone,
	}, callsOps[1])
	require.IsType(t, ReturnOp{}, callsOps[2])

	// With a condition and a value: check the condition, bail out on
	// condition errors, evaluate the value, record it.
	sumOps := handlers[probeIdx["sum"]]
	require.Len(t, sumOps, 6)
	require.IsType(t, CallOp{}, sumOps[0])
	require.Equal(t, MetricCondErrorCheckOp{ProbeID: probeIdx["sum"]}, sumOps[1])
	require.IsType(t, PrepareEventRootOp{}, sumOps[2])
	require.IsType(t, CallOp{}, sumOps[3])
	update, ok := sumOps[4].(MetricUpdateOp)
	require.True(t, ok)
	require.Equal(t, MetricKindCount, update.Kind)
	require.Equal(t, MetricValueSigned, update.ValueKind)
	require.Equal(t, uint8(8), update.ValueSize)
	require.IsType(t, ReturnOp{}, sumOps[5])
}
