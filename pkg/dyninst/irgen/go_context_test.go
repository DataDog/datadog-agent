// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestAnnotateSpecialGoTypesContextValueCtx(t *testing.T) {
	contextIface := &ir.GoInterfaceType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "context.Context",
			ByteSize: 16,
		},
	}
	anyIface := &ir.GoEmptyInterfaceType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "any",
			ByteSize: 16,
		},
	}
	valueCtx := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       3,
			Name:     "context.valueCtx",
			ByteSize: 48,
		},
		RawFields: []ir.Field{
			{Name: "Context", Offset: 0, Type: contextIface},
			{Name: "key", Offset: 16, Type: anyIface},
			{Name: "val", Offset: 32, Type: anyIface},
		},
	}
	tc := &typeCatalog{typesByID: map[ir.TypeID]ir.Type{
		contextIface.ID: contextIface,
		anyIface.ID:     anyIface,
		valueCtx.ID:     valueCtx,
	}}

	annotateSpecialGoTypes(tc, true, map[ir.TypeID]struct{}{
		valueCtx.ID: {},
	})

	wrapped, ok := tc.typesByID[valueCtx.ID].(*ir.GoContextImplementationType)
	require.True(t, ok, "valueCtx should be wrapped as GoContextImplementationType")
	require.Same(t, valueCtx, wrapped.StructureType)
	require.Equal(t, int32(0), wrapped.ContextOffset)
	require.Equal(t, int32(16), wrapped.KeyOffset)
	require.Equal(t, int32(32), wrapped.ValueOffset)
}

// TestAnnotateSpecialGoTypesDelegatingContextImpl covers a struct that
// implements context.Context through methods that delegate elsewhere (e.g.
// rapid.Context, whose methods forward to the request's context) and therefore
// has no context.Context field of its own. It is not a link in a walkable
// chain, so it is left as a plain StructureType and captured as an ordinary
// struct rather than being wrapped as GoContextImplementationType and run
// through the chain walk.
func TestAnnotateSpecialGoTypesDelegatingContextImpl(t *testing.T) {
	writerIface := &ir.GoInterfaceType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "io.Writer",
			ByteSize: 16,
		},
	}
	intType := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "int",
			ByteSize: 8,
		},
	}
	// A context implementation with no context.Context field and no key/value
	// payload: its context-ness comes only from delegating methods.
	wrapper := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       3,
			Name:     "example.RequestContext",
			ByteSize: 24,
		},
		RawFields: []ir.Field{
			{Name: "w", Offset: 0, Type: writerIface},
			{Name: "n", Offset: 16, Type: intType},
		},
	}
	tc := &typeCatalog{typesByID: map[ir.TypeID]ir.Type{
		writerIface.ID: writerIface,
		intType.ID:     intType,
		wrapper.ID:     wrapper,
	}}

	annotateSpecialGoTypes(tc, true, map[ir.TypeID]struct{}{
		wrapper.ID: {},
	})

	got, ok := tc.typesByID[wrapper.ID].(*ir.StructureType)
	require.True(t, ok,
		"a delegating impl with no parent context and no key/value payload has "+
			"no chain data, so it is left as a plain struct rather than wrapped "+
			"as GoContextImplementationType")
	require.Same(t, wrapper, got)
}

func TestAnnotateSpecialGoTypesDDTraceSpan(t *testing.T) {
	u64 := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "uint64",
			ByteSize: 8,
		},
	}
	traceID := &ir.ArrayType{
		TypeCommon: ir.TypeCommon{
			ID:       2,
			Name:     "[16]uint8",
			ByteSize: 16,
		},
	}
	spanContext := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       3,
			Name:     "github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.SpanContext",
			ByteSize: 32,
		},
		RawFields: []ir.Field{
			{Name: "traceID", Offset: 8, Type: traceID},
			{Name: "spanID", Offset: 24, Type: u64},
		},
	}
	spanContextPtr := &ir.PointerType{
		TypeCommon: ir.TypeCommon{
			ID:       4,
			Name:     "*github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.SpanContext",
			ByteSize: 8,
		},
		Pointee: spanContext,
	}
	span := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       5,
			Name:     "github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span",
			ByteSize: 80,
		},
		RawFields: []ir.Field{
			{Name: "spanID", Offset: 16, Type: u64},
			{Name: "traceID", Offset: 24, Type: u64},
			{Name: "parentID", Offset: 32, Type: u64},
			{Name: "context", Offset: 56, Type: spanContextPtr},
		},
	}
	tc := &typeCatalog{typesByID: map[ir.TypeID]ir.Type{
		u64.ID:            u64,
		traceID.ID:        traceID,
		spanContext.ID:    spanContext,
		spanContextPtr.ID: spanContextPtr,
		span.ID:           span,
	}}

	annotateSpecialGoTypes(tc, true, nil)

	spanWrapped, ok := tc.typesByID[span.ID].(*ir.DDTraceSpanType)
	require.True(t, ok, "tracer.Span should be wrapped as DDTraceSpanType")
	require.Same(t, span, spanWrapped.StructureType)
	require.Equal(t, ir.DDTraceSpanV2, spanWrapped.SpanKind)
	require.Equal(t, int32(24), spanWrapped.TraceIDOffset)
	require.Equal(t, int32(16), spanWrapped.SpanIDOffset)
	require.Equal(t, int32(32), spanWrapped.ParentIDOffset)
	require.Equal(t, int32(56), spanWrapped.SpanContextOffset)
	require.Equal(t, int32(8), spanWrapped.SpanContextTraceIDOffset)
}

func TestTypeContainsGoContextHandlesPlaceholders(t *testing.T) {
	st := &ir.StructureType{
		TypeCommon: ir.TypeCommon{
			ID:       1,
			Name:     "partial",
			ByteSize: 8,
		},
		RawFields: []ir.Field{{
			Name:   "field",
			Offset: 0,
			Type:   &placeHolderType{id: 2},
		}},
	}

	require.False(t, typeContainsGoContext(st))
}
