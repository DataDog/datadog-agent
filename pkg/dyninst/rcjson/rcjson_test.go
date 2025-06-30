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
)

type testCase struct {
	name  string
	input string
	want  Probe
	// regular expression to match the error message
	wantErr string
}

var testCases = []testCase{
	{
		name: "valid log probe",
		input: `{
				"id": "log-probe-1",
				"type": "LOG_PROBE",
				"version": 1,
				"where": {
					"typeName": "MyType",
					"sourceFile": "myfile.go",
					"methodName": "MyMethod",
					"lines": ["10", "20"],
					"signature": "func()"
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"template": "Hello {name}",
				"segments": [{"str": "Hello "}, {"dsl": "name", "json": {"ref": "name"}}],
				"captureSnapshot": true,
				"capture": {
					"maxReferenceDepth": 3,
					"maxFieldCount": 10,
					"maxCollectionSize": 100
				},
				"sampling": {
					"snapshotsPerSecond": 1.0
				},
				"evaluateAt": "entry"
			}`,
		want: &LogProbe{
			ID:      "log-probe-1",
			Version: 1,
			Type:    TypeLogProbe.String(),
			Where: &Where{
				TypeName:   "MyType",
				SourceFile: "myfile.go",
				MethodName: "MyMethod",
				Lines:      []string{"10", "20"},
				Signature:  "func()",
			},
			Tags:     []string{"tag1", "tag2"},
			Language: "go",
			Template: "Hello {name}",
			Segments: []json.RawMessage{
				json.RawMessage(`{"str": "Hello "}`),
				json.RawMessage(`{"dsl": "name", "json": {"ref": "name"}}`),
			},
			CaptureSnapshot: true,
			Capture: &Capture{
				MaxReferenceDepth: 3,
				MaxFieldCount:     10,
				MaxCollectionSize: 100,
			},
			Sampling: &Sampling{
				SnapshotsPerSecond: 1.0,
			},
			EvaluateAt: "entry",
		},
	},
	{
		name: "valid metric probe",
		input: `{
				"id": "metric-probe-1",
				"type": "METRIC_PROBE",
				"version": 1,
				"where": {
					"typeName": "MyType",
					"sourceFile": "myfile.go",
					"methodName": "MyMethod",
					"lines": ["10", "20"],
					"signature": "func()"
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
			ID:      "metric-probe-1",
			Type:    TypeMetricProbe.String(),
			Version: 1,
			Where: &Where{
				TypeName:   "MyType",
				SourceFile: "myfile.go",
				MethodName: "MyMethod",
				Lines:      []string{"10", "20"},
				Signature:  "func()",
			},
			Tags:       []string{"tag1", "tag2"},
			Language:   "go",
			Kind:       "count",
			MetricName: "my.metric",
			Value: &Value{
				DSL:  "1",
				JSON: json.RawMessage(`"1"`),
			},
			EvaluateAt: "entry",
		},
	},
	{
		name:    "invalid json",
		input:   `{invalid json}`,
		wantErr: `failed to parse json: .*`,
	},
	{
		name: "invalid probe type",
		input: `{
				"id": "invalid-probe",
				"type": "INVALID_TYPE"
			}`,
		wantErr: `failed to parse json: invalid config type: INVALID_TYPE`,
	},
}

func TestUnmarshalProbe(t *testing.T) {
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshalProbe([]byte(tt.input))
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Regexp(t, tt.wantErr, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.EqualValues(t, tt.want, got)
		})
	}
}
