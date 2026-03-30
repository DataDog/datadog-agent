// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcjson

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		validationErr: `signature is not supported`,
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
		unmarshalErr: `failed to parse json: .*`,
	},
	{
		name: "invalid probe type",
		input: `{
				"id": "invalid-probe",
				"type": "INVALID_TYPE"
			}`,
		unmarshalErr: `failed to parse json: invalid config type: INVALID_TYPE`,
	},
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
