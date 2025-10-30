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
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "foobar"}`),
							DSL:                  "foobar",
							EventKind:            1,
							EventExpressionIndex: -1,
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
					EventExpressionIndex: 1,
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
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
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
				ir.IssueSegment("unknown variable referenced by template: nonexistentVariable (probe: non-existent-variable-probe )"),
			},
		},
		{
			name: "malformed template",
			probe: ir.Probe{
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "foobar",
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				ProbeDefinition: probeDef{id: "bad-jsons"},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
				},
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
				ir.IssueSegment("invalid json for template segment (unexpected internal error)"),
			},
		},
		{
			name: "multiple variables in template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "multi-var-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "var1",
							Role: ir.VariableRoleParameter,
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							}},
						{
							Name: "var2",
							Role: ir.VariableRoleParameter,
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							}},
						{
							Name: "var3",
							Role: ir.VariableRoleParameter,
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							}},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
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
					EventExpressionIndex: 3,
				},

				ir.StringSegment(" "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "var2"}`),
					DSL:                  "var2",
					EventKind:            1,
					EventExpressionIndex: 4,
				},
				ir.StringSegment(" "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "var3"}`),
					DSL:                  "var3",
					EventKind:            1,
					EventExpressionIndex: 5,
				},
			},
		},
		{
			name: "template with only string segments",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "string-only-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "somevar",
							Role: ir.VariableRoleParameter,
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							}},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
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
						{
							Name: "var1",
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
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
						{
							Name: "foo",
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
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
							EventExpressionIndex: 1,
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
					EventExpressionIndex: 1,
				},
				ir.StringSegment(" items, "),
				ir.JSONSegment{
					JSON:                 json.RawMessage(`{"ref": "foo"}`),
					DSL:                  "foo",
					EventKind:            1,
					EventExpressionIndex: 2,
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
						{
							Name: "exists",
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "exists"}`),
							DSL:                  "exists",
							EventKind:            1,
							EventExpressionIndex: 1,
						},
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
					EventExpressionIndex: 1,
				},
				ir.IssueSegment("unknown variable referenced by template: missing (probe: partial-match-probe )"),
			},
		},
		{
			name: "template with no variables in subprogram",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "no-vars-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{PC: 6},
						}},
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
				ir.IssueSegment("unknown variable referenced by template: anything (probe: no-vars-probe )"),
			},
		},
		{
			name: "string and ref expression in template",
			probe: ir.Probe{
				ProbeDefinition: probeDef{id: "complex-expr-probe"},
				Subprogram: &ir.Subprogram{
					Variables: []*ir.Variable{
						{
							Name: "data",
							Role: ir.VariableRoleParameter,
							Locations: []ir.Location{
								{
									Range: ir.PCRange([2]uint64{0, 12}),
								},
							},
							Type: &ir.BaseType{
								TypeCommon: ir.TypeCommon{
									ByteSize: 8,
								},
							},
						},
					},
				},
				Events: []*ir.Event{
					{
						Kind: ir.EventKindEntry,
						InjectionPoints: []ir.InjectionPoint{
							{
								PC: 6,
							},
						},
					},
				},
				Template: &ir.Template{
					Segments: []ir.TemplateSegment{
						ir.StringSegment("Result: "),
						ir.JSONSegment{
							JSON:                 json.RawMessage(`{"ref": "data"}`),
							DSL:                  "data",
							EventKind:            ir.EventKindEntry,
							EventExpressionIndex: -1,
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
					EventExpressionIndex: 1,
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

func FuzzPopulateProbeExpressionsJSON(f *testing.F) {
	// Seed corpus with various PC ranges, injection points, and JSON inputs

	// Injection point inside range
	f.Add(uint64(0), uint64(10), uint64(5), []byte(`{"ref": "foobar"}`))      // injectionPoint in middle
	f.Add(uint64(100), uint64(200), uint64(150), []byte(`{"ref": "foobar"}`)) // injectionPoint in middle

	// Injection point at boundaries
	f.Add(uint64(0), uint64(10), uint64(0), []byte(`{"ref": "foobar"}`))   // injectionPoint == pclow
	f.Add(uint64(0), uint64(10), uint64(10), []byte(`{"ref": "foobar"}`))  // injectionPoint == pchigh
	f.Add(uint64(50), uint64(50), uint64(50), []byte(`{"ref": "foobar"}`)) // All equal (point range)

	// Injection point outside range (before)
	f.Add(uint64(10), uint64(20), uint64(5), []byte(`{"ref": "foobar"}`))   // injectionPoint < pclow
	f.Add(uint64(100), uint64(200), uint64(0), []byte(`{"ref": "foobar"}`)) // injectionPoint far before

	// Injection point outside range (after)
	f.Add(uint64(10), uint64(20), uint64(25), []byte(`{"ref": "foobar"}`))     // injectionPoint > pchigh
	f.Add(uint64(100), uint64(200), uint64(1000), []byte(`{"ref": "foobar"}`)) // injectionPoint far after

	// Boundary values with injection points
	f.Add(uint64(0), uint64(0), uint64(0), []byte(`{"ref": "foobar"}`))                                                    // All zero
	f.Add(uint64(0xFFFFFFFFFFFFFFFF), uint64(0xFFFFFFFFFFFFFFFF), uint64(0xFFFFFFFFFFFFFFFF), []byte(`{"ref": "foobar"}`)) // All max
	f.Add(uint64(0), uint64(0xFFFFFFFFFFFFFFFF), uint64(0x8000000000000000), []byte(`{"ref": "foobar"}`))                  // Full range, mid injection

	// Invalid/reversed ranges with various injection points
	f.Add(uint64(20), uint64(10), uint64(15), []byte(`{"ref": ""}`)) // Reversed range, injection in "between"
	f.Add(uint64(20), uint64(10), uint64(5), []byte(`{"ref": ""}`))  // Reversed range, injection before both
	f.Add(uint64(20), uint64(10), uint64(25), []byte(`{"ref": ""}`)) // Reversed range, injection after both
	f.Add(uint64(5), uint64(3), uint64(4), []byte(`{"ref": ""}`))    // Reversed range, injection in numerical middle

	// Empty/minimal JSON with injection point variations
	f.Add(uint64(0), uint64(100), uint64(50), []byte(`{}`))  // Inside range
	f.Add(uint64(0), uint64(100), uint64(0), []byte(`{}`))   // At pclow
	f.Add(uint64(0), uint64(100), uint64(100), []byte(`{}`)) // At pchigh
	f.Add(uint64(0), uint64(100), uint64(200), []byte(`{}`)) // Outside range

	// Unknown/malformed operations with injection points
	f.Add(uint64(0), uint64(50), uint64(25), []byte(`{"unknown": "op"}`))                                          // Inside range
	f.Add(uint64(0xFFFFFFFF), uint64(0xFFFFFFFF00000000), uint64(0xFFFFFFFF80000000), []byte(`{"unknown": "op"}`)) // Large range, mid injection

	// Malformed JSON with injection point edge cases
	f.Add(uint64(0), uint64(10), uint64(5), []byte(`{"" foo " }}}}`)) // Inside range
	f.Add(uint64(10), uint64(5), uint64(7), []byte(`{`))              // Reversed range with partial JSON
	f.Add(uint64(0), uint64(10), uint64(15), []byte(`}`))             // Outside range

	// Non-object JSON with various injection points
	f.Add(uint64(0), uint64(10), uint64(5), []byte(`[]`))                        // Inside range
	f.Add(uint64(0), uint64(0), uint64(1), []byte(`null`))                       // Outside point range
	f.Add(uint64(1000000), uint64(1000001), uint64(1000000), []byte(`"string"`)) // At pclow
	f.Add(uint64(0), uint64(0xFFFFFFFFFFFFFFFF), uint64(0), []byte(`123`))       // At start of full range

	f.Fuzz(func(t *testing.T, pclow, pchigh, injectionPoint uint64, jsonInput []byte) {
		// Create a probe with a JSONSegment containing the fuzzy JSON input
		probe := &ir.Probe{
			ProbeDefinition: probeDef{id: "fuzz-test-probe"},
			Subprogram: &ir.Subprogram{
				Variables: []*ir.Variable{
					{
						Name: "foobar",
						Role: ir.VariableRoleParameter,
						Locations: []ir.Location{
							{
								Range: ir.PCRange([2]uint64{pclow, pchigh}),
							},
						},
						Type: &ir.BaseType{
							TypeCommon: ir.TypeCommon{
								ByteSize: 8,
							},
						},
					},
				},
			},
			Events: []*ir.Event{
				{
					Kind: ir.EventKindEntry,
					InjectionPoints: []ir.InjectionPoint{
						{PC: injectionPoint},
					},
				},
			},
			Template: &ir.Template{
				Segments: []ir.TemplateSegment{
					ir.JSONSegment{
						JSON:                 json.RawMessage(jsonInput),
						DSL:                  string(jsonInput),
						EventKind:            1,
						EventExpressionIndex: -1,
					},
				},
			},
		}

		// Call populateProbeExpressions with fuzzy input
		// The function should not panic regardless of input
		_ = populateProbeExpressions(probe, &typeCatalog{
			typesByID: make(map[ir.TypeID]ir.Type),
			idAlloc:   idAllocator[ir.TypeID]{},
		})

		// Verify that the function either:
		// 1. Successfully processed the segment, or
		// 2. Replaced it with an IssueSegment
		// Either way, it should not panic
		if len(probe.Template.Segments) > 0 {
			// Just verify we can access the segment without panic
			seg := probe.Template.Segments[0]
			switch seg.(type) {
			case ir.JSONSegment:
			case ir.IssueSegment:
			case ir.StringSegment:
			default:
				t.Error("segment type is invalid")
			}
		}
	})
}
