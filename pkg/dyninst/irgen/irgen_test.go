// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"encoding/json"
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

func TestPopulateProbeExpressionTemplateIssues(t *testing.T) {
	// Segments are expected to be populated with the event expression index and kind
	// as well as the segment itself can be replaced in the case of an issue.
	type testCase struct {
		name                  string
		probe                 ir.Probe
		expectedReturnedIssue ir.Issue
		expectedSegments      []ir.TemplateSegment
	}
	tests := []testCase{
		{
			name: "good template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "good-template"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "foobar",
						},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "foobar"}`),
							DSL:                  "foobar",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "foobar"}`),
					DSL:                  "foobar",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
			},
		},
		{
			name: "missing variable in template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "non-existent-variable-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "foobar",
						},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "nonexistentVariable"}`),
							DSL:                  "nonexistentVariable",
							EventKind:            0,
							EventExpressionIndex: -1,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.IssueSegment("could not evaluate template segment: nonexistentVariable (probe: non-existent-variable-probe)"),
			},
		},
		{
			name: "malformed template",
			probe: ir.Probe{
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						Type: &ir.EventRootType{},
					},
				},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "foobar",
						},
					},
				},
				ProbeDefinition: probeDef{id: "bad-jsons"},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"" foo " }}}}`),
							DSL:                  "bad-json",
							EventKind:            0,
							EventExpressionIndex: -1,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.IssueSegment("malformed template segment: bad-json (internal error)"),
			},
		},
		{
			name: "multiple variables in template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "multi-var-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{Name: "var1"},
						{Name: "var2"},
						{Name: "var3"},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "var1"}`),
							DSL:                  "var1",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
						ir.StringSegment(" "),
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "var2"}`),
							DSL:                  "var2",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
						ir.StringSegment(" "),
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "var3"}`),
							DSL:                  "var3",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "var1"}`),
					DSL:                  "var1",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
				ir.StringSegment(" "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "var2"}`),
					DSL:                  "var2",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
				ir.StringSegment(" "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "var3"}`),
					DSL:                  "var3",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
			},
		},
		{
			name: "template with only string segments",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "string-only-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{Name: "somevar"},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.StringSegment("This is a static string"),
						ir.StringSegment(" with multiple segments"),
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.StringSegment("This is a static string"),
				ir.StringSegment(" with multiple segments"),
			},
		},
		{
			name: "empty template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "empty-template-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{Name: "var1"},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments:      []ir.TemplateSegment{},
		},
		{
			name: "multiple reference same var",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "repeated-var-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{Name: "foo"},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "foo"}`),
							DSL:                  "foo",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
						ir.StringSegment(" items, "),
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "foo"}`),
							DSL:                  "foo",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
						ir.StringSegment(" total"),
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "foo"}`),
					DSL:                  "foo",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
				ir.StringSegment(" items, "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "foo"}`),
					DSL:                  "foo",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
				ir.StringSegment(" total"),
			},
		},
		{
			name: "partial match",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "partial-match-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{Name: "exists"},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "exists"}`),
							DSL:                  "exists",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
						ir.StringSegment(" "),
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "missing"}`),
							DSL:                  "missing",
							EventKind:            0,
							EventExpressionIndex: -1,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "exists"}`),
					DSL:                  "exists",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
				ir.StringSegment(" "),
				ir.IssueSegment("could not evaluate template segment: missing (probe: partial-match-probe)"),
			},
		},
		{
			name: "template with no variables in subprogram",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "no-vars-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "anything"}`),
							DSL:                  "anything",
							EventKind:            0,
							EventExpressionIndex: -1,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.IssueSegment("could not evaluate template segment: anything (probe: no-vars-probe)"),
			},
		},
		{
			name: "string and ref expression in template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "complex-expr-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{Name: "data"},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.StringSegment("Result: "),
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "data"}`),
							DSL:                  "data",
							EventKind:            1,
							EventExpressionIndex: 0,
						},
					},
				},
			},
			expectedReturnedIssue: ir.Issue{},
			expectedSegments: []ir.TemplateSegment{
				ir.StringSegment("Result: "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "data"}`),
					DSL:                  "data",
					EventKind:            1,
					EventExpressionIndex: 0,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			issue := populateProbeExpressions(&tc.probe, &typeCatalog{
				typesByID: make(map[ir.TypeID]ir.Type),
				idAlloc:   idAllocator[ir.TypeID]{},
			})
			require.Equal(t, tc.expectedReturnedIssue, issue)
			require.Equal(t, tc.expectedSegments, tc.probe.Template.Segments)
		})
	}
}
