// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// TestResolveExpressionMultipleFieldsOnSameStruct tests that resolving
// multiple field accesses on the same struct variable produces distinct
// LocationOp offsets for each field.
//
// This is a regression test for a bug where template segments like
// {re.kind} and {re.message} would both resolve to the same field offset,
// causing both to display the same value in the log message.
func TestResolveExpressionMultipleFieldsOnSameStruct(t *testing.T) {
	// Create a struct type with two string fields at different offsets.
	// Simulating a struct like:
	//   type richError struct {
	//       kind    string  // at offset 0
	//       message string  // at offset 16
	//   }
	// We use BaseType to represent string fields for simplicity in this test.
	stringType := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "string",
			ByteSize: 16,
		},
	}

	structType := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "richError",
			ByteSize: 32,
		},
		RawFields: []ir.Field{
			{Name: "kind", Offset: 0, Type: stringType},
			{Name: "message", Offset: 16, Type: stringType},
		},
	}

	// Create a variable of type richError.
	variable := &ir.Variable{
		Name: "re",
		Type: structType,
	}

	// Create a type catalog with our types.
	tc := &typeCatalog{
		typesByID: map[ir.TypeID]ir.Type{
			stringType.ID: stringType,
			structType.ID: structType,
		},
	}

	// Test case 1: re.kind
	kindExpr := &exprlang.GetMemberExpr{
		Base:   &exprlang.RefExpr{Ref: "re"},
		Member: "kind",
	}

	resolvedKind, err := resolveExpression(kindExpr, variable, tc)
	require.NoError(t, err, "failed to resolve re.kind")
	require.Len(t, resolvedKind.Operations, 1, "expected one operation for re.kind")

	kindLocOp, ok := resolvedKind.Operations[0].(*ir.LocationOp)
	require.True(t, ok, "expected LocationOp for re.kind")
	require.Equal(t, uint32(0), kindLocOp.Offset, "re.kind should have offset 0")
	require.Equal(t, uint32(16), kindLocOp.ByteSize, "re.kind should have byte size 16")

	// Test case 2: re.message
	messageExpr := &exprlang.GetMemberExpr{
		Base:   &exprlang.RefExpr{Ref: "re"},
		Member: "message",
	}

	resolvedMessage, err := resolveExpression(messageExpr, variable, tc)
	require.NoError(t, err, "failed to resolve re.message")
	require.Len(t, resolvedMessage.Operations, 1, "expected one operation for re.message")

	messageLocOp, ok := resolvedMessage.Operations[0].(*ir.LocationOp)
	require.True(t, ok, "expected LocationOp for re.message")
	require.Equal(t, uint32(16), messageLocOp.Offset, "re.message should have offset 16")
	require.Equal(t, uint32(16), messageLocOp.ByteSize, "re.message should have byte size 16")

	// CRITICAL: Verify that the two LocationOps are different objects with different offsets.
	// This is the main assertion - both should NOT have the same offset.
	require.NotEqual(t, kindLocOp.Offset, messageLocOp.Offset,
		"BUG: re.kind and re.message have the same offset! "+
			"kindLocOp.Offset=%d, messageLocOp.Offset=%d",
		kindLocOp.Offset, messageLocOp.Offset)

	// Verify the types are correct.
	require.Equal(t, stringType, resolvedKind.Type, "re.kind should have string type")
	require.Equal(t, stringType, resolvedMessage.Type, "re.message should have string type")
}

// TestAnalyzeTemplateSegmentsMultipleFieldAccess tests that when a template
// has multiple segments accessing different fields of the same struct variable,
// each segment gets a unique EventExpressionIndex.
func TestAnalyzeTemplateSegmentsMultipleFieldAccess(t *testing.T) {
	// Create types
	stringType := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "string",
			ByteSize: 16,
		},
	}

	structType := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "richError",
			ByteSize: 32,
		},
		RawFields: []ir.Field{
			{Name: "kind", Offset: 0, Type: stringType},
			{Name: "message", Offset: 16, Type: stringType},
		},
	}

	// Create expressions for re.kind and re.message
	kindExpr := &exprlang.GetMemberExpr{
		Base:   &exprlang.RefExpr{Ref: "re"},
		Member: "kind",
	}
	messageExpr := &exprlang.GetMemberExpr{
		Base:   &exprlang.RefExpr{Ref: "re"},
		Member: "message",
	}

	// Create template segments
	kindSegment := &ir.JSONSegment{
		JSON: kindExpr,
		DSL:  "re.kind",
	}
	messageSegment := &ir.JSONSegment{
		JSON: messageExpr,
		DSL:  "re.message",
	}

	// Create a template with both segments
	template := &ir.Template{
		TemplateString: "kind={re.kind} message={re.message}",
		Segments: []ir.TemplateSegment{
			ir.StringSegment("kind="),
			kindSegment,
			ir.StringSegment(" message="),
			messageSegment,
		},
	}

	// Create variable
	variable := &ir.Variable{
		Name: "re",
		Type: structType,
	}

	// Create type catalog
	tc := &typeCatalog{
		typesByID: map[ir.TypeID]ir.Type{
			stringType.ID: stringType,
			structType.ID: structType,
		},
	}

	// Create analyzed expressions (simulating what analyzeAllProbes does)
	expressions := []analyzedExpression{
		{
			expr:         kindExpr,
			dsl:          "re.kind",
			rootVariable: variable,
			eventKind:    ir.EventKindLine,
			exprKind:     ir.RootExpressionKindTemplateSegment,
			segment:      kindSegment,
			segmentIdx:   1,
		},
		{
			expr:         messageExpr,
			dsl:          "re.message",
			rootVariable: variable,
			eventKind:    ir.EventKindLine,
			exprKind:     ir.RootExpressionKindTemplateSegment,
			segment:      messageSegment,
			segmentIdx:   3,
		},
	}

	// Simulate what populateEventExpressions does
	var rootExpressions []*ir.RootExpression
	for _, expr := range expressions {
		resolvedExpr, err := resolveExpression(expr.expr, expr.rootVariable, tc)
		require.NoError(t, err)

		// Update segment with expression index
		if seg := expr.segment; seg != nil {
			seg.EventKind = expr.eventKind
			seg.EventExpressionIndex = len(rootExpressions)
		}

		rootExpressions = append(rootExpressions, &ir.RootExpression{
			Name:       expr.dsl,
			Kind:       expr.exprKind,
			Expression: resolvedExpr,
		})
	}

	// Verify that each segment has a unique EventExpressionIndex
	require.Equal(t, 0, kindSegment.EventExpressionIndex,
		"kindSegment should have EventExpressionIndex 0")
	require.Equal(t, 1, messageSegment.EventExpressionIndex,
		"messageSegment should have EventExpressionIndex 1")

	// Verify that the EventExpressionIndex values are different
	require.NotEqual(t, kindSegment.EventExpressionIndex, messageSegment.EventExpressionIndex,
		"BUG: kindSegment and messageSegment have the same EventExpressionIndex!")

	// Verify that the resolved expressions have different LocationOp offsets
	kindLocOp := rootExpressions[0].Expression.Operations[0].(*ir.LocationOp)
	messageLocOp := rootExpressions[1].Expression.Operations[0].(*ir.LocationOp)

	require.Equal(t, uint32(0), kindLocOp.Offset)
	require.Equal(t, uint32(16), messageLocOp.Offset)
	require.NotEqual(t, kindLocOp.Offset, messageLocOp.Offset,
		"BUG: kind and message expressions have the same LocationOp offset!")

	// Verify the segments in the template still point to the correct objects
	seg1, ok := template.Segments[1].(*ir.JSONSegment)
	require.True(t, ok)
	require.Same(t, kindSegment, seg1, "template segment 1 should be kindSegment")

	seg3, ok := template.Segments[3].(*ir.JSONSegment)
	require.True(t, ok)
	require.Same(t, messageSegment, seg3, "template segment 3 should be messageSegment")
}

// TestResolveExpressionIndependentLocations verifies that each call to
// resolveExpression creates independent LocationOp objects that don't
// share state.
func TestResolveExpressionIndependentLocations(t *testing.T) {
	stringType := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "string",
			ByteSize: 16,
		},
	}

	structType := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "testStruct",
			ByteSize: 48,
		},
		RawFields: []ir.Field{
			{Name: "field1", Offset: 0, Type: stringType},
			{Name: "field2", Offset: 16, Type: stringType},
			{Name: "field3", Offset: 32, Type: stringType},
		},
	}

	variable := &ir.Variable{
		Name: "s",
		Type: structType,
	}

	tc := &typeCatalog{
		typesByID: map[ir.TypeID]ir.Type{
			stringType.ID: stringType,
			structType.ID: structType,
		},
	}

	// Resolve all three fields.
	fields := []string{"field1", "field2", "field3"}
	expectedOffsets := []uint32{0, 16, 32}
	var resolvedLocOps []*ir.LocationOp

	for i, fieldName := range fields {
		expr := &exprlang.GetMemberExpr{
			Base:   &exprlang.RefExpr{Ref: "s"},
			Member: fieldName,
		}

		resolved, err := resolveExpression(expr, variable, tc)
		require.NoError(t, err, "failed to resolve s.%s", fieldName)
		require.Len(t, resolved.Operations, 1)

		locOp, ok := resolved.Operations[0].(*ir.LocationOp)
		require.True(t, ok)
		require.Equal(t, expectedOffsets[i], locOp.Offset,
			"s.%s should have offset %d, got %d", fieldName, expectedOffsets[i], locOp.Offset)

		resolvedLocOps = append(resolvedLocOps, locOp)
	}

	// Verify all LocationOps are distinct objects (not sharing pointers).
	for i := 0; i < len(resolvedLocOps); i++ {
		for j := i + 1; j < len(resolvedLocOps); j++ {
			require.NotSame(t, resolvedLocOps[i], resolvedLocOps[j],
				"LocationOps for field%d and field%d should not be the same object",
				i+1, j+1)
		}
	}
}
