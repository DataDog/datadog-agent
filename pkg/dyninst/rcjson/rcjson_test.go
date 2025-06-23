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
					"type_name": "MyType",
					"source_file": "myfile.go",
					"method_name": "MyMethod",
					"lines": ["10", "20"],
					"signature": "func()"
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"template": "Hello {{.name}}",
				"segments": [],
				"capture_snapshot": true,
				"capture": {
					"max_reference_depth": 3,
					"max_field_count": 10,
					"max_collection_size": 100
				},
				"sampling": {
					"snapshots_per_second": 1.0
				},
				"evaluate_at": "entry"
			}`,
		want: &LogProbe{
			ID:      "log-probe-1",
			Version: 1,
			Where: &Where{
				TypeName:   "MyType",
				SourceFile: "myfile.go",
				MethodName: "MyMethod",
				Lines:      []string{"10", "20"},
				Signature:  "func()",
			},
			Tags:            []string{"tag1", "tag2"},
			Language:        "go",
			Template:        "Hello {{.name}}",
			Segments:        []json.RawMessage{},
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
					"type_name": "MyType",
					"source_file": "myfile.go",
					"method_name": "MyMethod",
					"lines": ["10", "20"],
					"signature": "func()"
				},
				"tags": ["tag1", "tag2"],
				"language": "go",
				"kind": "count",
				"metric_name": "my.metric",
				"value": {
					"dsl": "1",
					"json": "1"
				},
				"evaluate_at": "entry"
			}`,
		want: &MetricProbe{
			ID:      "metric-probe-1",
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
