// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestCompileUnitFromNameCases(t *testing.T) {
	type testCase struct {
		testName string
		symbol   string
		want     string
	}
	tc := func(symbol, want string) testCase {
		return testCase{
			testName: symbol[:min(len(symbol), 32)],
			symbol:   symbol,
			want:     want,
		}
	}
	testCases := []testCase{
		tc(
			"github.com/DataDog/datadog-agent/pkg/dyninst/irgen.Foo",
			"github.com/DataDog/datadog-agent/pkg/dyninst/irgen",
		),
		tc(
			"a/b.Foo.Bar.Baz",
			"a/b",
		),
		tc(
			"github.com/pkg/errors.(*withStack).Format",
			"github.com/pkg/errors",
		),
		tc("int", "runtime"),
		{
			testName: "long generic type",
			symbol:   "sync/atomic.(*Pointer[go.shape.struct { gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.point gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.statsPoint; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.kafkaOffset gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.kafkaOffset; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.typ gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.pointType; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.queuePos int64 }]).Swap",
			want:     "sync/atomic",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			require.Equal(t, tc.want, compileUnitFromName(tc.symbol))
		})
	}

}

type probeDef struct {
	id string
}

func (probeDef) GetCaptureConfig() ir.CaptureConfig   { return nil }
func (p probeDef) GetID() string                      { return p.id }
func (probeDef) GetKind() ir.ProbeKind                { return 0 }
func (probeDef) GetTags() []string                    { return nil }
func (probeDef) GetTemplate() ir.TemplateDefinition   { return nil }
func (probeDef) GetThrottleConfig() ir.ThrottleConfig { return nil }
func (probeDef) GetVersion() int                      { return 0 }
func (probeDef) GetWhere() ir.Where                   { return nil }

func TestPopulateProbeExpression(t *testing.T) {
	type testCase struct {
		name          string
		probe         ir.Probe
		wantIssueKind ir.IssueKind
		wantIssueMsg  string
	}
	tests := []testCase{
		{
			name: "missing variable in template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "non-existent-variable-probe"},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							DSL:                  "nonexistentVariable",
							EventKind:            0,
							EventExpressionIndex: -1,
						},
					},
				},
			},
			wantIssueKind: ir.IssueKindTargetNotFoundInBinary,
			wantIssueMsg:  "variable referenced in template missing: nonexistentVariable (probe: non-existent-variable-probe)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issue := populateProbeExpressions(&tc.probe, nil)
			require.Equal(t, tc.wantIssueKind, issue.Kind)
			require.Equal(t, tc.wantIssueMsg, issue.Message)
		})
	}
}

func TestCollectAllSegmentVariables(t *testing.T) {
	tests := []struct {
		name         string
		segments     []ir.TemplateSegment
		expectedVars variableToSegmentIndexes
		expectedErr  error
	}{
		{
			name: "single JSONSegment with ref",
			segments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON: json.RawMessage(`{"ref":"foobar"}`),
					DSL:  "foobar",
				},
				ir.StringSegment("oh yea this is a string heehee"),
			},
			expectedVars: variableToSegmentIndexes{
				"foobar": []int{0},
			},
			expectedErr: nil,
		},
		{
			name: "multiple JSONSegment with ref to different variables",
			segments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON: json.RawMessage(`{"ref":"foobar"}`),
					DSL:  "foobar",
				},
				ir.JSONSegment{
					JSON: json.RawMessage(`{"ref":"bazbuz"}`),
					DSL:  "bazbuz",
				},
				ir.StringSegment("oh yea this is a string heehee"),
			},
			expectedVars: variableToSegmentIndexes{
				"foobar": []int{0},
				"bazbuz": []int{1},
			},
			expectedErr: nil,
		},
		{
			name: "multiple JSONSegment with ref to same variable",
			segments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON: json.RawMessage(`{"ref":"foobar"}`),
					DSL:  "foobar",
				},
				ir.JSONSegment{
					JSON: json.RawMessage(`{"ref":"foobar"}`),
					DSL:  "bazbuz",
				},
				ir.StringSegment("oh yea this is a string heehee"),
			},
			expectedVars: variableToSegmentIndexes{
				"foobar": []int{0, 1},
			},
			expectedErr: nil,
		},
		{
			name: "no JSONSegments",
			segments: []ir.TemplateSegment{
				ir.StringSegment("just a string segment"),
			},
			expectedVars: variableToSegmentIndexes{},
			expectedErr:  nil,
		},
		{
			name: "invalid JSONSegment",
			segments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON: json.RawMessage(`not-valid-json`),
					DSL:  "bad",
				},
			},
			expectedVars: nil,
			expectedErr:  errors.New("invalid template: bad: not-valid-json"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vars, err := collectAllSegmentVariables(tc.segments)
			require.Equal(t, err, tc.expectedErr)
			require.Equal(t, tc.expectedVars, vars)

		})
	}
}
