// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package compiler

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// makeMetricUpdateOp builds the op that records a metric probe's value for an
// event of the probe. irgen puts the value expression, if any, first among
// the event's root expressions, which is where the BPF side looks for its
// status.
func makeMetricUpdateOp(
	probe *ir.Probe, probeID uint32, rootType *ir.EventRootType,
) (MetricUpdateOp, error) {
	mp, ok := probe.ProbeDefinition.(ir.MetricProbeDefinition)
	if !ok {
		return MetricUpdateOp{}, fmt.Errorf(
			"metric probe %s has no metric definition", probe.GetID(),
		)
	}
	op := MetricUpdateOp{ProbeID: probeID}
	switch kind := mp.GetMetricKind(); kind {
	case ir.MetricKindCount:
		op.Kind = MetricKindCount
	case ir.MetricKindGauge:
		op.Kind = MetricKindGauge
	default:
		return MetricUpdateOp{}, fmt.Errorf(
			"metric probe %s has unsupported kind %v", probe.GetID(), kind,
		)
	}
	switch len(rootType.Expressions) {
	case 0:
		if op.Kind != MetricKindCount {
			return MetricUpdateOp{}, fmt.Errorf(
				"%v metric probe %s has no value", mp.GetMetricKind(), probe.GetID(),
			)
		}
		op.ValueKind = MetricValueNone
		return op, nil
	case 1:
	default:
		return MetricUpdateOp{}, fmt.Errorf(
			"metric probe %s has %d expressions, expected at most 1",
			probe.GetID(), len(rootType.Expressions),
		)
	}
	expr := rootType.Expressions[0]
	if expr.Kind != ir.RootExpressionKindMetricValue {
		return MetricUpdateOp{}, fmt.Errorf(
			"metric probe %s has a %v expression instead of a metric value",
			probe.GetID(), expr.Kind,
		)
	}
	valueType := expr.Expression.Type
	valueKind, err := metricValueKind(valueType)
	if err != nil {
		return MetricUpdateOp{}, fmt.Errorf("metric probe %s: %w", probe.GetID(), err)
	}
	switch size := valueType.GetByteSize(); size {
	case 1, 2, 4, 8:
		op.ValueSize = uint8(size)
	default:
		return MetricUpdateOp{}, fmt.Errorf(
			"metric probe %s: value of type %s has unsupported size %d",
			probe.GetID(), valueType.GetName(), size,
		)
	}
	op.ValueKind = valueKind
	op.ValueOffset = expr.Offset
	return op, nil
}

// metricValueKind says how the BPF program should interpret a metric value
// of the given type.
func metricValueKind(t ir.Type) (MetricValueKind, error) {
	switch ir.MetricEncoding(t) {
	case ir.MetricValueSigned, ir.MetricValueDuration:
		return MetricValueSigned, nil
	case ir.MetricValueUnsigned:
		return MetricValueUnsigned, nil
	case ir.MetricValueFloat32, ir.MetricValueFloat64:
		return MetricValueFloat, nil
	default:
		return 0, fmt.Errorf("unsupported metric value type %s", t.GetName())
	}
}
