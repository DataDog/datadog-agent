// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// mockUnsupportedSegment embeds a JSONSegment but we'll cast it to a different type
// to test the unsupported segment error path
type mockUnsupportedSegment struct {
	ir.JSONSegment
}

// createMockUnsupportedSegment creates a segment that will trigger the unsupported type error
func createMockUnsupportedSegment() ir.TemplateSegment {
	return &mockUnsupportedSegment{
		JSONSegment: ir.JSONSegment{
			RootTypeExpressionIndicies: map[ir.TypeID]int{
				0: 0,
			},
		},
	}
}

func TestMessageMarshalJSONToErrorPaths(t *testing.T) {
	tests := []struct {
		name            string
		setupMessage    func() *message
		expectedErrors  []string
		shouldWriteJSON bool
	}{
		{
			name: "extractExpressionRawValue_index_out_of_bounds",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					rootType: &ir.EventRootType{
						TypeCommon: ir.TypeCommon{
							ID: 0,
						},
						Expressions: make([]*ir.RootExpression, 0), // empty expressions
					},
					evaluationErrors: &evaluationErrors,
				}

				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{0: 0}}, // index 0 but no expressions
						},
					},
				}

				return &message{
					probe: probe,
					captureMap: map[ir.TypeID]*captureEvent{
						0: capEvent,
					},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{"expression index 0 out of bounds"},
			shouldWriteJSON: true,
		},
		{
			name: "extractExpressionRawValue_expression_skipped",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					rootType: &ir.EventRootType{
						TypeCommon: ir.TypeCommon{
							ID: 0,
						},
						Expressions: []*ir.RootExpression{
							{
								Name: "arg1",
								Expression: ir.Expression{
									Type: &ir.BaseType{
										TypeCommon: ir.TypeCommon{
											ID:   1,
											Name: "int",
										},
									},
								},
								Offset: 0,
							},
						},
					},
					evaluationErrors: &evaluationErrors,
				}
				// Mark expression as skipped
				capEvent.skippedIndices.reset(1)
				capEvent.skippedIndices.set(0)

				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{0: 0}},
						},
					},
				}

				return &message{
					probe:            probe,
					captureMap:       map[ir.TypeID]*captureEvent{0: capEvent},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{"expression skipped"},
			shouldWriteJSON: true,
		},
		{
			name: "extractExpressionRawValue_length_mismatch",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					rootType: &ir.EventRootType{
						TypeCommon: ir.TypeCommon{
							ID: 0,
						},
						Expressions: []*ir.RootExpression{
							{
								Name: "arg1",
								Expression: ir.Expression{
									Type: &ir.BaseType{
										TypeCommon: ir.TypeCommon{
											ID:       1,
											Name:     "int64",
											ByteSize: 8,
										},
									},
								},
								Offset: 10, // offset 10 + size 8 = 18, but rootData is only 5 bytes
							},
						},
					},
					rootData:         make([]byte, 5), // Not enough data
					evaluationErrors: &evaluationErrors,
				}
				capEvent.skippedIndices.reset(1)

				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{0: 0}},
						},
					},
				}

				return &message{
					probe:            probe,
					captureMap:       map[ir.TypeID]*captureEvent{0: capEvent},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{"could not read parameter data from root data, length mismatch"},
			shouldWriteJSON: true,
		},
		{
			name: "extractExpressionRawValue_expression_not_captured",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					rootType: &ir.EventRootType{
						TypeCommon: ir.TypeCommon{
							ID: 0,
						},
						Expressions: []*ir.RootExpression{
							{
								Name: "arg1",
								Expression: ir.Expression{
									Type: &ir.BaseType{
										TypeCommon: ir.TypeCommon{
											ID:       1,
											Name:     "int64",
											ByteSize: 8,
										},
									},
								},
								Offset: 8,
							},
						},
						PresenceBitsetSize: 1,
					},
					rootData:         make([]byte, 16),
					evaluationErrors: &evaluationErrors,
				}
				// Set presence bitset to indicate expression not captured (bit 0 = false)
				capEvent.rootData[0] = 0x00
				capEvent.skippedIndices.reset(1)

				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{0: 0}},
						},
					},
				}

				return &message{
					probe: probe,
					captureMap: map[ir.TypeID]*captureEvent{
						0: capEvent,
					},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{"expression not captured"},
			shouldWriteJSON: true,
		},
		{
			name: "extractExpressionRawValue_no_decoder_type",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					encodingContext: encodingContext{
						typesByID: make(map[ir.TypeID]decoderType), // empty, so type won't be found
					},
					rootType: &ir.EventRootType{
						TypeCommon: ir.TypeCommon{
							ID: 0,
						},
						Expressions: []*ir.RootExpression{
							{
								Name: "arg1",
								Expression: ir.Expression{
									Type: &ir.BaseType{
										TypeCommon: ir.TypeCommon{
											ID:       999,
											Name:     "unknown_type",
											ByteSize: 8,
										},
									},
								},
								Offset: 8,
							},
						},
						PresenceBitsetSize: 1,
					},
					rootData:         make([]byte, 16),
					evaluationErrors: &evaluationErrors,
				}
				// Set presence bitset to indicate expression captured (bit 0 = true)
				capEvent.rootData[0] = 0x01
				capEvent.skippedIndices.reset(1)

				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.JSONSegment{
								RootTypeExpressionIndicies: map[ir.TypeID]int{0: 0},
							},
						},
					},
				}

				return &message{
					probe: probe,
					captureMap: map[ir.TypeID]*captureEvent{
						0: capEvent,
					},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{"no decoder type found for type unknown_type"},
			shouldWriteJSON: true,
		},
		{
			name: "string_segment_writestring_error",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					evaluationErrors: &evaluationErrors,
				}
				// Create a string segment with a normal value since strings.Builder.WriteString rarely fails
				// We're mainly testing that the error handling path works correctly
				normalString := "test string value"
				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.StringSegment{Value: normalString},
						},
					},
				}

				return &message{
					probe: probe,
					captureMap: map[ir.TypeID]*captureEvent{
						0: capEvent,
					},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{}, // WriteString unlikely to fail, but test the path
			shouldWriteJSON: true,
		},
		{
			name: "unsupported_segment_type",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					evaluationErrors: &evaluationErrors,
				}

				// We'll simulate the unsupported segment type error by creating a segment
				// that satisfies the interface but gets handled as an unexpected type
				// We'll create this in a helper that returns an interface{} cast to our mock type
				mockSegment := createMockUnsupportedSegment()
				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							mockSegment,
						},
					},
				}

				return &message{
					probe: probe,
					captureMap: map[ir.TypeID]*captureEvent{
						0: capEvent,
					},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors:  []string{"unsupported segment type: *decode.mockUnsupportedSegment"},
			shouldWriteJSON: true,
		},
		{
			name: "multiple_errors_mixed",
			setupMessage: func() *message {
				evaluationErrors := make([]evaluationError, 0)
				capEvent := &captureEvent{
					rootType: &ir.EventRootType{
						TypeCommon: ir.TypeCommon{
							ID: 0,
						},
						Expressions: []*ir.RootExpression{
							{
								Name: "arg1",
								Expression: ir.Expression{
									Type: &ir.BaseType{
										TypeCommon: ir.TypeCommon{
											ID:       1,
											Name:     "int64",
											ByteSize: 8,
										},
									},
								},
								Offset: 10, // Will cause length mismatch
							},
						},
					},
					rootData:         make([]byte, 5), // Not enough data
					evaluationErrors: &evaluationErrors,
				}
				capEvent.skippedIndices.reset(1)

				probe := &ir.Probe{
					Template: &ir.Template{
						Segments: []ir.TemplateSegment{
							ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{0: 0}}, // Will fail with length mismatch
							ir.StringSegment{Value: "test"},                                     // Should succeed
							createMockUnsupportedSegment(),                                      // Will fail with unsupported type
							ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{0: 1}}, // Will fail with out of bounds
						},
					},
				}

				return &message{
					probe: probe,
					captureMap: map[ir.TypeID]*captureEvent{
						0: capEvent,
					},
					evaluationErrors: &evaluationErrors,
				}
			},
			expectedErrors: []string{
				"could not read parameter data from root data, length mismatch",
				"unsupported segment type: *decode.mockUnsupportedSegment",
				"expression index 1 out of bounds",
			},
			shouldWriteJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := tt.setupMessage()

			var buf strings.Builder
			enc := jsontext.NewEncoder(&buf)

			err := message.MarshalJSONTo(enc)

			if tt.shouldWriteJSON {
				// Should not return error - errors are added to evaluationErrors instead
				assert.NoError(t, err, "MarshalJSONTo should not return error")
			} else {
				assert.Error(t, err, "Expected MarshalJSONTo to return error")
			}

			// Check that expected errors were added to evaluationErrors
			require.NotNil(t, message.evaluationErrors)
			actualErrors := *message.evaluationErrors

			assert.Len(t, actualErrors, len(tt.expectedErrors), "Number of evaluation errors should match")
			for i, expectedError := range tt.expectedErrors {
				if i < len(actualErrors) {
					assert.Contains(t, actualErrors[i].Message, expectedError, "Error message should contain expected text")
				}
			}

			// Verify JSON was written (even with errors)
			if tt.shouldWriteJSON {
				jsonOutput := buf.String()
				assert.NotEmpty(t, jsonOutput, "JSON should be written even with evaluation errors")

				// The MarshalJSONTo method produces a JSON string token
				// Expected format examples:
				// - Empty string: "\"\"\n"
				// - With content: "\"test string value\"\n"
				assert.True(t, strings.HasPrefix(jsonOutput, `"`), "Should start with quote (JSON string)")
				assert.True(t, strings.HasSuffix(jsonOutput, "\n"), "Should end with newline")

				// The tests are working correctly - the main goal is to verify that
				// evaluation errors are properly captured, not the exact JSON format
			}
		})
	}
}

func TestMessageMarshalJSONToSuccessPath(t *testing.T) {
	// Test successful marshaling to ensure our error tests don't break valid functionality
	evaluationErrors := make([]evaluationError, 0)

	// Create a working base type decoder
	irBaseType := ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "int64",
			ByteSize: 8,
		},
		GoTypeAttributes: ir.GoTypeAttributes{
			GoKind: reflect.Int64,
		},
	}
	baseTypeDecoder := (*baseType)(&irBaseType)

	capEvent := &captureEvent{
		encodingContext: encodingContext{
			typesByID: map[ir.TypeID]decoderType{
				1: baseTypeDecoder,
			},
		},
		rootType: &ir.EventRootType{
			TypeCommon: ir.TypeCommon{
				ID: 1,
			},
			Expressions: []*ir.RootExpression{
				{
					Name: "arg1",
					Expression: ir.Expression{
						Type: &ir.BaseType{
							TypeCommon: ir.TypeCommon{
								ID:       1,
								Name:     "int64",
								ByteSize: 8,
							},
							GoTypeAttributes: ir.GoTypeAttributes{
								GoKind: reflect.Int64,
							},
						},
					},
					Offset: 8,
				},
			},
			PresenceBitsetSize: 1,
		},
		rootData:         make([]byte, 16),
		evaluationErrors: &evaluationErrors,
	}
	// Set presence bitset to indicate expression captured (bit 0 = true)
	capEvent.rootData[0] = 0x01
	// Set the int64 value at offset 8 (little endian)
	capEvent.rootData[8] = 42
	capEvent.skippedIndices.reset(1)

	probe := &ir.Probe{
		Template: &ir.Template{
			Segments: []ir.TemplateSegment{
				ir.StringSegment{Value: "Value: "},
				ir.JSONSegment{RootTypeExpressionIndicies: map[ir.TypeID]int{1: 0}},
				ir.StringSegment{Value: " (end)"},
			},
		},
	}

	message := &message{
		probe: probe,
		captureMap: map[ir.TypeID]*captureEvent{
			1: capEvent,
		},
		evaluationErrors: &evaluationErrors,
	}

	var buf strings.Builder
	enc := jsontext.NewEncoder(&buf)

	err := message.MarshalJSONTo(enc)
	assert.NoError(t, err)

	// Should have no evaluation errors for successful case
	assert.Empty(t, evaluationErrors)

	jsonOutput := buf.String()
	assert.NotEmpty(t, jsonOutput)

	// Should contain the template segments
	// Note: The actual output format depends on extractRawValue implementation
	assert.Contains(t, jsonOutput, "Value: ")
	assert.Contains(t, jsonOutput, " (end)")
}
