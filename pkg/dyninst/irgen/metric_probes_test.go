// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// metricProbeJSON returns the RC definition of a metric probe. where and the
// optional extra fields are raw JSON.
func metricProbeJSON(id, kind, where, extra string) string {
	js := fmt.Sprintf(
		`{"id": %q, "type": "METRIC_PROBE", "version": 1, "kind": %q, "metricName": %q, "where": %s`,
		id, kind, id, where,
	)
	if extra != "" {
		js += ", " + extra
	}
	return js + "}"
}

func mustParseProbe(t *testing.T, js string) ir.ProbeDefinition {
	t.Helper()
	p, err := rcjson.UnmarshalProbe([]byte(js))
	require.NoError(t, err, js)
	return p
}

func onFunc(name string) string {
	return fmt.Sprintf(`{"methodName": %q}`, name)
}

func value(dsl, js string) string {
	return fmt.Sprintf(`"value": {"dsl": %q, "json": %s}`, dsl, js)
}

func TestMetricProbeTypeRedaction(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "simple", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			for _, tc := range []struct {
				name, function, expression, redactedType string
				condition                                bool
			}{
				{"scalar", "main.condInt64", `{"ref":"x"}`, "int64", false},
				{"struct", "main.condStructArg", `{"getmember":[{"ref":"x"},"F64"]}`, "main.condFields", false},
				{"pointer", "main.condPtrStructArg", `{"getmember":[{"ref":"x"},"F64"]}`, "*main.condFields", false},
				{"pointee", "main.condPtrStructArg", `{"getmember":[{"ref":"x"},"F64"]}`, "main.condFields", false},
				{"field", "main.condPtrStructArg", `{"getmember":[{"ref":"x"},"F64"]}`, "float64", false},
				{"wildcard", "main.condPtrStructArg", `{"getmember":[{"ref":"x"},"F64"]}`, "main.cond*", false},
				{"length", "main.lenSlice", `{"len":{"ref":"s"}}`, "[]int", false},
				{"index", "main.condSliceArg", `{"index":[{"ref":"x"},0]}`, "int", false},
				{"slice_element", "main.structSliceArg", `{"getmember":[{"index":[{"ref":"s"},0]},"Val"]}`, "main.indexMemberStruct", false},
				{"map_key", "main.mapIndexStringKey", `{"index":[{"ref":"m"},"one"]}`, "string", false},
				{"map_value", "main.mapIndexStringKey", `{"index":[{"ref":"m"},"one"]}`, "int", false},
				{"comparison", "main.condStructArg", `{"gt":[{"getmember":[{"ref":"x"},"I32"]},0]}`, "main.condFields", false},
				{"predicate", "main.anyAllStructSlice", `{"any":[{"ref":"xs"},{"gt":[{"getmember":[{"ref":"@it"},"I32"]},0]}]}`, "main.condFields", false},
				{"condition", "main.condInt64", `{"gt":[{"ref":"x"},0]}`, "int64", true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if strings.HasPrefix(tc.name, "map_") && strings.HasPrefix(cfg.GOTOOLCHAIN, "go1.23") {
						t.Skip("map indexing requires swiss maps")
					}
					kind, extra := "GAUGE", value(tc.name, tc.expression)
					if tc.condition {
						kind = "COUNT"
						extra = fmt.Sprintf(`"when":{"dsl":%q,"json":%s}`, tc.name, tc.expression)
					}
					probe := mustParseProbe(t, metricProbeJSON(tc.name, kind, onFunc(tc.function), extra))
					_, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true))
					require.NoError(t, err, "the probe must work without type redaction")
					policy := redaction.NewConfig(nil, []string{tc.redactedType}, nil)
					_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true), irgen.WithRedaction(policy))
					var noProbes *ir.NoSuccessfulProbesError
					require.ErrorAs(t, err, &noProbes)
					require.Len(t, noProbes.Issues, 1)
					require.Contains(t, noProbes.Issues[0].Message, "redacted type")
				})
			}
			policy := redaction.NewConfig(nil, []string{"int32"}, nil)
			probe := mustParseProbe(t, metricProbeJSON("unrelated", "GAUGE", onFunc("main.condPtrStructArg"), value("x.F64", `{"getmember":[{"ref":"x"},"F64"]}`)))
			_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true), irgen.WithRedaction(policy))
			require.NoError(t, err, "redacting an unread field must not reject the probe")
		})
	}
}

func TestGenericMetricProbeTypeRedaction(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "sample", cfg))
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	probe := mustParseProbe(t, metricProbeJSON("generic", "COUNT", onFunc("main.(*Pair[...]).SetValue"), value("v", `{"ref":"v"}`)))
	_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true))
	require.NoError(t, err)
	policy := redaction.NewConfig(nil, []string{"main.Credentials"}, nil)
	_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true), irgen.WithRedaction(policy))
	var noProbes *ir.NoSuccessfulProbesError
	require.ErrorAs(t, err, &noProbes)
	require.Contains(t, noProbes.Issues[0].Message, "cannot check redacted types for generic type")
	probe = mustParseProbe(t, metricProbeJSON("calls", "COUNT", onFunc("main.(*Pair[...]).SetValue"), ""))
	_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true), irgen.WithRedaction(policy))
	require.NoError(t, err, "counting calls does not read generic values")
}

func TestGenericGaugeIncompatibleTypes(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "sample", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			where := onFunc("main.(*Pair[...]).SetValue")
			probe := mustParseProbe(t, metricProbeJSON("generic", "GAUGE", where, value("v", `{"ref":"v"}`)))
			_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true))
			var noProbes *ir.NoSuccessfulProbesError
			require.ErrorAs(t, err, &noProbes)
			require.Len(t, noProbes.Issues, 1)
			require.Equal(t, ir.IssueKindUnsupportedFeature, noProbes.Issues[0].Kind)
			require.Contains(t, noProbes.Issues[0].Message, "incompatible gauge value types")

			probe = mustParseProbe(t, metricProbeJSON("generic", "COUNT", where, value("v", `{"ref":"v"}`)))
			_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true))
			require.NoError(t, err, "counts use the same encoding for all integer and boolean types")
		})
	}
}

func TestMetricProbesAtReturn(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "simple", cfg))
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			for _, tc := range []struct{ name, function, kind, extra string }{
				{"calls", "main.condReturnAndLocal", "COUNT", ""},
				{"return", "main.condReturnAndLocal", "GAUGE", value("@return", `{"ref":"@return"}`)},
				{"duration", "main.condReturnAndLocal", "GAUGE", value("@duration", `{"ref":"@duration"}`)},
				{"multiple", "main.condMultiReturn", "COUNT", value("@return.r0", `{"getmember":[{"ref":"@return"},"r0"]}`)},
				{"condition", "main.condReturnAndLocal", "COUNT", `"when":{"dsl":"@return > 0","json":{"gt":[{"ref":"@return"},0]}}`},
				{"duration_condition", "main.condReturnAndLocal", "COUNT", `"when":{"dsl":"@duration >= 0","json":{"ge":[{"ref":"@duration"},0]}}`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					extra := `"evaluateAt":"EXIT"`
					if tc.extra != "" {
						extra += "," + tc.extra
					}
					probe := mustParseProbe(t, metricProbeJSON(tc.name, tc.kind, onFunc(tc.function), extra))
					prog, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, irgen.WithMetricProbes(true))
					require.NoError(t, err)
					require.Empty(t, prog.Issues)
					require.Len(t, prog.Probes, 2)
					var metric *ir.Probe
					for _, p := range prog.Probes {
						if p.GetKind() == ir.ProbeKindMetric {
							metric = p
						} else {
							require.Equal(t, ir.ProbeKindRuntimeRecovery, p.GetKind())
						}
					}
					require.NotNil(t, metric)
					for _, inst := range metric.Instances {
						require.Len(t, inst.Events, 2)
						require.Equal(t, ir.EventKindEntry, inst.Events[0].Kind)
						require.Empty(t, inst.Events[0].Type.Expressions)
						require.Nil(t, inst.Events[0].Condition)
						for _, ip := range inst.Events[0].InjectionPoints {
							require.True(t, ip.HasAssociatedReturn)
						}
						require.Equal(t, ir.EventKindReturn, inst.Events[1].Kind)
					}
				})
			}
		})
	}
}

func TestMetricReturnValidation(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "simple", cfg))
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	for _, tc := range []struct {
		name, function, kind, extra, message string
		options                              []irgen.Option
	}{
		{"disabled_returns", "main.condReturnAndLocal", "COUNT", `"evaluateAt":"EXIT"`, "supported return locations", []irgen.Option{irgen.WithSkipReturnEvents(true)}},
		{"invalid_timing", "main.condReturnAndLocal", "COUNT", `"evaluateAt":"DURING"`, "evaluation timing", nil},
		{"duration_count", "main.condReturnAndLocal", "COUNT", `"evaluateAt":"EXIT",` + value("@duration", `{"ref":"@duration"}`), "use a gauge metric", nil},
		{"redacted_return_condition", "main.condReturnAndLocal", "COUNT", `"evaluateAt":"EXIT","when":{"dsl":"@return > 0","json":{"gt":[{"ref":"@return"},0]}}`, "redacted type", []irgen.Option{irgen.WithRedaction(redaction.NewConfig(nil, []string{"int"}, nil))}},
		{"redacted_return", "main.condReturnAndLocal", "GAUGE", `"evaluateAt":"EXIT",` + value("@return", `{"ref":"@return"}`), "redacted type", []irgen.Option{irgen.WithRedaction(redaction.NewConfig(nil, []string{"int"}, nil))}},
		{"redacted_return_field", "main.condReturnPtr", "GAUGE", `"evaluateAt":"EXIT",` + value("@return.F64", `{"getmember":[{"ref":"@return"},"F64"]}`), "redacted type", []irgen.Option{irgen.WithRedaction(redaction.NewConfig(nil, []string{"main.condFields"}, nil))}},
		{"entry_return", "main.condReturnAndLocal", "GAUGE", `"evaluateAt":"ENTRY",` + value("@return", `{"ref":"@return"}`), "only available at function return", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := mustParseProbe(t, metricProbeJSON(tc.name, tc.kind, onFunc(tc.function), tc.extra))
			_, err := irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe}, append(tc.options, irgen.WithMetricProbes(true))...)
			var noProbes *ir.NoSuccessfulProbesError
			require.ErrorAs(t, err, &noProbes)
			require.Len(t, noProbes.Issues, 1)
			require.Contains(t, noProbes.Issues[0].Message, tc.message)
		})
	}
}

func TestMetricProbesDisabled(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	bin := testprogs.MustGetBinary(t, "simple", cfg)
	obj, err := object.OpenElfFileWithDwarf(bin)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	probe := mustParseProbe(t, metricProbeJSON("calls", "COUNT", onFunc("main.condInt64"), ""))
	_, err = irgen.GenerateIR(1, obj, []ir.ProbeDefinition{probe})
	var noProbes *ir.NoSuccessfulProbesError
	require.True(t, errors.As(err, &noProbes), "unexpected error: %v", err)
	require.Len(t, noProbes.Issues, 1)
	require.Equal(t, ir.IssueKindUnsupportedFeature, noProbes.Issues[0].Kind)
	require.Equal(t, "probe kind ProbeKindMetric is not supported", noProbes.Issues[0].Message)
}

func TestMetricProbes(t *testing.T) {
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			bin := testprogs.MustGetBinary(t, "simple", cfg)
			obj, err := object.OpenElfFileWithDwarf(bin)
			require.NoError(t, err)
			defer func() { require.NoError(t, obj.Close()) }()
			testMetricProbes(t, obj, cfg)
		})
	}
}

func testMetricProbes(t *testing.T, obj object.FileWithDwarf, cfg testprogs.Config) {
	// go1.23's DWARF for main.condLine is incomplete: the location lists at
	// the line are bad and the local `result` is missing (the probe files skip
	// line probes on go1.23 too).
	goodCondLineDWARF := !strings.HasPrefix(cfg.GOTOOLCHAIN, "go1.23")

	type valid struct {
		js string
		// wantKind is the Go kind of the value, or Invalid if the probe has
		// no value.
		wantKind  reflect.Kind
		wantEvent ir.EventKind
		wantCond  bool
	}
	validProbes := map[string]valid{
		"calls": {
			js:        metricProbeJSON("calls", "COUNT", onFunc("main.condInt64"), ""),
			wantKind:  reflect.Invalid,
			wantEvent: ir.EventKindEntry,
		},
		"sum": {
			js:        metricProbeJSON("sum", "count", onFunc("main.condInt64"), value("x", `{"ref": "x"}`)),
			wantKind:  reflect.Int64,
			wantEvent: ir.EventKindEntry,
		},
		"sum_when": {
			js: metricProbeJSON("sum_when", "COUNT", onFunc("main.condInt64"),
				value("x", `{"ref": "x"}`)+
					`, "when": {"dsl": "tag == \"match\"", "json": {"eq": [{"ref": "tag"}, "match"]}}`),
			wantKind:  reflect.Int64,
			wantEvent: ir.EventKindEntry,
			wantCond:  true,
		},
		"gauge": {
			js: metricProbeJSON("gauge", "GAUGE", onFunc("main.condPtrStructArg"),
				value("x.F64", `{"getmember": [{"ref": "x"}, "F64"]}`)),
			wantKind:  reflect.Float64,
			wantEvent: ir.EventKindEntry,
		},
		"field": {
			js: metricProbeJSON("field", "COUNT", onFunc("main.condNilPtrStruct"),
				value("x.I32", `{"getmember": [{"ref": "x"}, "I32"]}`)),
			wantKind:  reflect.Int32,
			wantEvent: ir.EventKindEntry,
		},
	}
	if goodCondLineDWARF {
		validProbes["line"] = valid{
			js: metricProbeJSON("line", "COUNT",
				`{"methodName": "main.condLine", "sourceFile": "main.go", "lines": ["754"]}`,
				`"evaluateAt":"EXIT",`+value("n", `{"ref": "n"}`)),
			wantKind:  reflect.Int,
			wantEvent: ir.EventKindLine,
		}
	}

	type invalid struct {
		js       string
		wantKind ir.IssueKind
		wantMsg  string
	}
	invalidProbes := map[string]invalid{
		"histogram": {
			js:       metricProbeJSON("histogram", "HISTOGRAM", onFunc("main.condInt64"), value("x", `{"ref": "x"}`)),
			wantKind: ir.IssueKindUnsupportedFeature,
			wantMsg:  "histogram metric probes are not supported",
		},
		"unknown_kind": {
			js:       metricProbeJSON("unknown_kind", "SUMMARY", onFunc("main.condInt64"), ""),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  "metric kind is missing or unknown",
		},
		"gauge_without_value": {
			js:       metricProbeJSON("gauge_without_value", "GAUGE", onFunc("main.condInt64"), ""),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  "gauge metric probes require a value expression",
		},
		"unknown_variable": {
			js:       metricProbeJSON("unknown_variable", "COUNT", onFunc("main.condInt64"), value("nope", `{"ref": "nope"}`)),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  `metric value "nope" references unknown variable "nope"`,
		},
		"string_value": {
			js:       metricProbeJSON("string_value", "GAUGE", onFunc("main.condString"), value("x", `{"ref": "x"}`)),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  "which is not numeric",
		},
		"float_count": {
			js: metricProbeJSON("float_count", "COUNT", onFunc("main.condPtrStructArg"),
				value("x.F64", `{"getmember": [{"ref": "x"}, "F64"]}`)),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  "count metric probes need an integer or boolean value",
		},
		// Float arguments are passed in floating-point registers, which the
		// BPF program can't read.
		"float_register": {
			js:       metricProbeJSON("float_register", "GAUGE", onFunc("main.condFloat64"), value("x", `{"ref": "x"}`)),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  "passed in a floating-point register",
		},
		"duration": {
			js:       metricProbeJSON("duration", "GAUGE", onFunc("main.condInt64"), value("@duration", `{"ref": "@duration"}`)),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  "uses @duration",
		},
		"unsupported_operation": {
			js:       metricProbeJSON("unsupported_operation", "COUNT", onFunc("main.condInt64"), value("foo(x)", `{"foo": [{"ref": "x"}]}`)),
			wantKind: ir.IssueKindUnsupportedFeature,
			wantMsg:  `uses unsupported operation "foo"`,
		},
	}

	if goodCondLineDWARF {
		invalidProbes["local_at_entry"] = invalid{
			js:       metricProbeJSON("local_at_entry", "COUNT", onFunc("main.condLine"), value("result", `{"ref": "result"}`)),
			wantKind: ir.IssueKindInvalidProbeDefinition,
			wantMsg:  `references "result", which is not available at function entry`,
		}
	}

	var probes []ir.ProbeDefinition
	for _, p := range validProbes {
		probes = append(probes, mustParseProbe(t, p.js))
	}
	for _, p := range invalidProbes {
		probes = append(probes, mustParseProbe(t, p.js))
	}

	prog, err := irgen.GenerateIR(1, obj, probes, irgen.WithMetricProbes(true))
	require.NoError(t, err)

	gotIssues := make(map[string]ir.Issue)
	for _, issue := range prog.Issues {
		gotIssues[issue.ProbeDefinition.GetID()] = issue.Issue
	}
	for id, want := range invalidProbes {
		issue, ok := gotIssues[id]
		if !assert.True(t, ok, "expected an issue for probe %s", id) {
			continue
		}
		assert.Equal(t, want.wantKind, issue.Kind, "probe %s: %s", id, issue.Message)
		assert.Contains(t, issue.Message, want.wantMsg, "probe %s", id)
	}

	gotProbes := make(map[string]*ir.Probe)
	for _, p := range prog.Probes {
		// Metric probes don't leave in_progress_calls state behind, so the
		// runtime.recovery probe isn't needed.
		require.NotEqual(t, ir.ProbeKindRuntimeRecovery, p.GetKind())
		gotProbes[p.GetID()] = p
	}
	for id, want := range validProbes {
		p, ok := gotProbes[id]
		if !assert.True(t, ok, "probe %s is missing; issue: %v", id, gotIssues[id]) {
			continue
		}
		require.Len(t, p.Instances, 1, id)
		events := p.Instances[0].Events
		require.Len(t, events, 1, "probe %s should only have an entry or line event", id)
		event := events[0]
		assert.Equal(t, want.wantEvent, event.Kind, id)
		assert.Equal(t, want.wantCond, event.Condition != nil, id)
		for _, ip := range event.InjectionPoints {
			assert.False(t, ip.HasAssociatedReturn, id)
		}
		if want.wantKind == reflect.Invalid {
			assert.Empty(t, event.Type.Expressions, id)
			continue
		}
		require.Len(t, event.Type.Expressions, 1, id)
		expr := event.Type.Expressions[0]
		assert.Equal(t, ir.RootExpressionKindMetricValue, expr.Kind, id)
		goKind, ok := expr.Expression.Type.GetGoKind()
		assert.True(t, ok, id)
		assert.Equal(t, want.wantKind, goKind, id)
	}
}
