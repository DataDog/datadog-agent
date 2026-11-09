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
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

func TestMetricGaugeTypes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kinds      []reflect.Kind
		compatible bool
	}{
		{"signed", []reflect.Kind{reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int}, true},
		{"unsigned", []reflect.Kind{reflect.Bool, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint, reflect.Uintptr}, true},
		{"float32", []reflect.Kind{reflect.Float32, reflect.Float32}, true},
		{"float64", []reflect.Kind{reflect.Float64, reflect.Float64}, true},
		{"signed_unsigned", []reflect.Kind{reflect.Int64, reflect.Uint64}, false},
		{"bool_signed", []reflect.Kind{reflect.Bool, reflect.Int}, false},
		{"signed_float", []reflect.Kind{reflect.Int64, reflect.Float64}, false},
		{"unsigned_float", []reflect.Kind{reflect.Uint64, reflect.Float64}, false},
		{"float_widths", []reflect.Kind{reflect.Float32, reflect.Float64}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := &ir.Probe{ProbeDefinition: &rcjson.MetricProbe{Kind: "GAUGE"}}
			for _, kind := range tc.kinds {
				probe.Instances = append(probe.Instances, ir.ProbeInstance{Events: []*ir.Event{{
					Type: &ir.EventRootType{Expressions: []*ir.RootExpression{{
						Kind: ir.RootExpressionKindMetricValue,
						Expression: ir.Expression{Type: &ir.BaseType{
							TypeCommon:       ir.TypeCommon{Name: kind.String()},
							GoTypeAttributes: ir.GoTypeAttributes{GoKind: kind},
						}},
					}}},
				}}})
			}
			issue := checkMetricGaugeTypes(probe)
			require.Equal(t, tc.compatible, issue.IsNone(), "%v", issue)
			if !tc.compatible {
				require.Equal(t, ir.IssueKindUnsupportedFeature, issue.Kind)
				require.Contains(t, issue.Message, "incompatible gauge value types")
			}
		})
	}
}
