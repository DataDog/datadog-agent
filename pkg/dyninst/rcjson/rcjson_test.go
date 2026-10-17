// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package rcjson

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

type testCase struct {
	name  string
	input string
	want  Probe
	// Regular expression to match the error message from
	// unmarshalProbeWithoutValidation.
	unmarshalErr string
	// Regular expression to match the validation error message.
	validationErr string
}

func intPtr(v int) *int {
	return &v
}

var testCases = []testCase{
	{
		name: "log probe with file and lines",
		input: `{
				"id": "log-probe-1",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod",
					"sourceFile": "myfile.go",
					"lines": ["10"]
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"template": "Hello {name}",
				"segments": [{"str": "Hello "}, {"dsl": "name", "json": {"ref": "name"}}],
				"capture": {
					"maxReferenceDepth": 3,
					"maxLength": 123,
					"maxCollectionSize": 100
				},
				"sampling": {
					"snapshotsPerSecond": 1.0
				},
				"evaluateAt": "entry"
			}`,
		want: &LogProbe{
			LogProbeCommon: LogProbeCommon{
				ProbeCommon: ProbeCommon{
					ID:      "log-probe-1",
					Version: 1,
					Type:    TypeLogProbe.String(),
					Where: &Where{
						MethodName: "MyMethod",
						SourceFile: "myfile.go",
						Lines:      []string{"10"},
					},
					Tags:       []string{"tag1", "tag2"},
					Language:   "go",
					EvaluateAt: "entry",
				},
				Capture: &Capture{
					MaxReferenceDepth: intPtr(3),
					MaxLength:         intPtr(123),
					MaxCollectionSize: intPtr(100),
				},
				Sampling: &Sampling{
					SnapshotsPerSecond: 1.0,
				},
				Template: "Hello {name}",
				Segments: SegmentList{
					StringSegment("Hello "),
					JSONSegment{JSON: json.RawMessage(`{"ref": "name"}`), DSL: "name"},
				},
			},
		},
	},
	{
		name: "log probe with file and multiple lines",
		input: `{
				"id": "log-probe-1",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod",
					"sourceFile": "myfile.go",
					"lines": ["10", "20"]
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"template": "Hello {name}",
				"segments": [{"str": "Hello "}, {"dsl": "name", "json": {"ref": "name"}}],
				"capture": {
					"maxReferenceDepth": 3,
					"maxLength": 123,
					"maxCollectionSize": 100
				},
				"sampling": {
					"snapshotsPerSecond": 1.0
				},
				"evaluateAt": "entry"
			}`,
		want: &LogProbe{
			LogProbeCommon: LogProbeCommon{
				ProbeCommon: ProbeCommon{
					ID:      "log-probe-1",
					Version: 1,
					Type:    TypeLogProbe.String(),
					Where: &Where{
						MethodName: "MyMethod",
						SourceFile: "myfile.go",
						Lines:      []string{"10", "20"},
					},
					Tags:       []string{"tag1", "tag2"},
					Language:   "go",
					EvaluateAt: "entry",
				},
				Capture: &Capture{
					MaxReferenceDepth: intPtr(3),
					MaxLength:         intPtr(123),
					MaxCollectionSize: intPtr(100),
				},
				Sampling: &Sampling{
					SnapshotsPerSecond: 1.0,
				},
				Template: "Hello {name}",
				Segments: SegmentList{
					StringSegment("Hello "),
					JSONSegment{JSON: json.RawMessage(`{"ref": "name"}`), DSL: "name"},
				},
			},
		},
		validationErr: `lines must be a single line number`,
	},
	{
		name: "log probe with method and signature",
		input: `{
				"id": "log-probe-1",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod",
					"signature": "func()"
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"template": "Hello {name}",
				"segments": [{"str": "Hello "}, {"dsl": "name", "json": {"ref": "name"}}],
				"capture": {
					"maxReferenceDepth": 3,
					"maxFieldCount": 10,
					"maxLength": 123,
					"maxCollectionSize": 100
				},
				"sampling": {
					"snapshotsPerSecond": 1.0
				},
				"evaluateAt": "entry"
			}`,
		want: &LogProbe{
			LogProbeCommon: LogProbeCommon{
				ProbeCommon: ProbeCommon{
					ID:      "log-probe-1",
					Version: 1,
					Type:    TypeLogProbe.String(),
					Where: &Where{
						MethodName: "MyMethod",
						Signature:  "func()",
					},
					Tags:       []string{"tag1", "tag2"},
					Language:   "go",
					EvaluateAt: "entry",
				},
				Capture: &Capture{
					MaxReferenceDepth: intPtr(3),
					MaxFieldCount:     intPtr(10),
					MaxLength:         intPtr(123),
					MaxCollectionSize: intPtr(100),
				},
				Sampling: &Sampling{
					SnapshotsPerSecond: 1.0,
				},
				Template: "Hello {name}",
				Segments: SegmentList{
					StringSegment("Hello "),
					JSONSegment{JSON: json.RawMessage(`{"ref": "name"}`), DSL: "name"},
				},
			},
		},
	},
	{
		// System tests, which are shared across tracers, send log probes
		// like this one and expect them to be installed.
		name: "log probe without template",
		input: `{
				"id": "log-probe-1",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"segments": []
			}`,
		want: &LogProbe{
			LogProbeCommon: LogProbeCommon{
				ProbeCommon: ProbeCommon{
					ID:      "log-probe-1",
					Version: 1,
					Type:    TypeLogProbe.String(),
					Where: &Where{
						MethodName: "MyMethod",
					},
				},
			},
		},
	},
	{
		name: "valid metric probe",
		input: `{
				"id": "metric-probe-1",
				"type": "METRIC_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"kind": "count",
				"metricName": "my.metric",
				"value": {
					"dsl": "1",
					"json": "1"
				},
				"evaluateAt": "entry"
			}`,
		want: &MetricProbe{
			ProbeCommon: ProbeCommon{
				ID:      "metric-probe-1",
				Version: 1,
				Type:    TypeMetricProbe.String(),
				Where: &Where{
					MethodName: "MyMethod",
				},
				Tags:       []string{"tag1", "tag2"},
				Language:   "go",
				EvaluateAt: "entry",
			},
			Kind:       "count",
			MetricName: "my.metric",
			Value: &Value{
				DSL:  "1",
				JSON: json.RawMessage(`"1"`),
			},
		},
	},
	{
		name: "metric probe with condition",
		input: `{
				"id": "metric-probe-2",
				"type": "METRIC_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"kind": "GAUGE",
				"metricName": "my.gauge",
				"value": {"dsl": "x", "json": {"ref": "x"}},
				"when": {"dsl": "x > 0", "json": {"gt": [{"ref": "x"}, 0]}}
			}`,
		want: &MetricProbe{
			ProbeCommon: ProbeCommon{
				ID:      "metric-probe-2",
				Version: 1,
				Type:    TypeMetricProbe.String(),
				Where: &Where{
					MethodName: "MyMethod",
				},
			},
			When: &When{
				DSL:  "x > 0",
				JSON: json.RawMessage(`{"gt": [{"ref": "x"}, 0]}`),
			},
			Kind:       "GAUGE",
			MetricName: "my.gauge",
			Value: &Value{
				DSL:  "x",
				JSON: json.RawMessage(`{"ref": "x"}`),
			},
		},
	},
	{
		name: "metric probe without metric name",
		input: `{
				"id": "metric-probe-3",
				"type": "METRIC_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"kind": "COUNT"
			}`,
		want: &MetricProbe{
			ProbeCommon: ProbeCommon{
				ID:      "metric-probe-3",
				Version: 1,
				Type:    TypeMetricProbe.String(),
				Where: &Where{
					MethodName: "MyMethod",
				},
			},
			Kind: "COUNT",
		},
		validationErr: `metricName must be set`,
	},
	{
		name: "metric probe with unknown kind",
		input: `{
				"id": "metric-probe-4",
				"type": "METRIC_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"kind": "SUMMARY",
				"metricName": "my.metric"
			}`,
		want: &MetricProbe{
			ProbeCommon: ProbeCommon{
				ID:      "metric-probe-4",
				Version: 1,
				Type:    TypeMetricProbe.String(),
				Where: &Where{
					MethodName: "MyMethod",
				},
			},
			Kind:       "SUMMARY",
			MetricName: "my.metric",
		},
		validationErr: `unknown metric kind "SUMMARY"`,
	},
	{
		name: "capture expression probe",
		input: `{
				"id": "capture-expr-1",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"captureSnapshot": false,
				"captureExpressions": [
					{
						"name": "x_val",
						"expr": {"dsl": "x", "json": {"ref": "x"}}
					},
					{
						"name": "y_val",
						"expr": {"dsl": "y", "json": {"ref": "y"}},
						"capture": {"maxReferenceDepth": 2}
					}
				]
			}`,
		want: &CaptureExpressionProbe{
			LogProbeCommon: LogProbeCommon{
				ProbeCommon: ProbeCommon{
					ID:      "capture-expr-1",
					Version: 1,
					Type:    TypeLogProbe.String(),
					Where: &Where{
						MethodName: "MyMethod",
					},
				},
			},
			RawCaptureExpressions: []*CaptureExpressionEntry{
				{
					Name: "x_val",
					Expr: CaptureExprJSON{
						DSL:  "x",
						JSON: json.RawMessage(`{"ref": "x"}`),
					},
				},
				{
					Name: "y_val",
					Expr: CaptureExprJSON{
						DSL:  "y",
						JSON: json.RawMessage(`{"ref": "y"}`),
					},
					Capture: &Capture{MaxReferenceDepth: intPtr(2)},
				},
			},
		},
	},
	{
		name: "capture expression probe without expressions",
		input: `{
				"id": "capture-expr-2",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"methodName": "MyMethod"
				},
				"captureSnapshot": false,
				"captureExpressions": []
			}`,
		want: &CaptureExpressionProbe{
			LogProbeCommon: LogProbeCommon{
				ProbeCommon: ProbeCommon{
					ID:      "capture-expr-2",
					Version: 1,
					Type:    TypeLogProbe.String(),
					Where: &Where{
						MethodName: "MyMethod",
					},
				},
			},
			RawCaptureExpressions: []*CaptureExpressionEntry{},
		},
		validationErr: `captureExpressions must be non-empty`,
	},
	{
		name:         "invalid json",
		input:        `{invalid json}`,
		unmarshalErr: `failed to parse probe definition: .*`,
	},
	{
		name: "invalid probe type",
		input: `{
				"id": "invalid-probe",
				"type": "INVALID_TYPE"
			}`,
		unmarshalErr: `failed to parse probe definition: invalid config type: INVALID_TYPE`,
	},
	{
		name: "trigger probe",
		input: `{
				"id": "trigger-probe",
				"type": "TRIGGER_PROBE",
				"where": {"methodName": "MyMethod"}
			}`,
		unmarshalErr: `^probe type TRIGGER_PROBE is not supported$`,
	},
	{
		name: "span decoration probe",
		input: `{
				"id": "span-decoration-probe",
				"type": "SPAN_DECORATION_PROBE",
				"where": {"methodName": "MyMethod"}
			}`,
		unmarshalErr: `^probe type SPAN_DECORATION_PROBE is not supported$`,
	},
	{
		name: "log probe with a malformed segment",
		input: `{
				"id": "log-probe-1",
				"type": "LOG_PROBE",
				"where": {"methodName": "MyMethod"},
				"template": "Hello",
				"segments": [{"unknown": "Hello"}]
			}`,
		unmarshalErr: `^failed to parse LOG_PROBE definition: unknown segment type at index 0`,
	},
}

func TestMaxReferenceDepthZero(t *testing.T) {
	probe, err := UnmarshalProbe([]byte(`{
		"id": "snapshot-probe",
		"type": "LOG_PROBE",
		"captureSnapshot": true,
		"where": {"methodName": "main.f"},
		"capture": {"maxReferenceDepth": 0}
	}`))
	require.NoError(t, err)
	require.Equal(t, uint32(0), probe.GetCaptureConfig().GetMaxReferenceDepth())
}

func TestMetricProbeAccessors(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want ir.MetricKind
	}{
		{kind: "COUNT", want: ir.MetricKindCount},
		{kind: "count", want: ir.MetricKindCount},
		{kind: "Gauge", want: ir.MetricKindGauge},
		{kind: "HISTOGRAM", want: ir.MetricKindHistogram},
		{kind: "distribution", want: ir.MetricKindDistribution},
		{kind: "", want: 0},
		{kind: "SUMMARY", want: 0},
	} {
		p := &MetricProbe{Kind: tc.kind}
		assert.Equal(t, tc.want, p.GetMetricKind(), "kind %q", tc.kind)
	}

	// A probe without a value or a condition.
	p := &MetricProbe{Kind: "COUNT", MetricName: "calls"}
	dsl, valueJSON := p.GetValue()
	assert.Empty(t, dsl)
	assert.Nil(t, valueJSON)
	assert.Nil(t, p.GetWhen())
	assert.Empty(t, p.GetWhenDSL())
	assert.Equal(t, "calls", p.GetMetricName())
	assert.Equal(t, ir.ProbeKindMetric, p.GetKind())
}

func TestUnmarshalProbe(t *testing.T) {
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalProbe([]byte(tt.input))
			if tt.unmarshalErr != "" {
				require.Error(t, err)
				require.Regexp(t, tt.unmarshalErr, err.Error())
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, tt.want, got)
			validationErr := Validate(got)
			if tt.validationErr != "" {
				assert.Error(t, validationErr)
				assert.Regexp(t, tt.validationErr, validationErr.Error())
			} else {
				assert.NoError(t, validationErr)
			}
		})
	}
}
