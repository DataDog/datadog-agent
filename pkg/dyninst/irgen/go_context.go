// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// ddTraceSpanTypeNames are dd-trace-go span types whose layout the BPF chain
// walk needs to know about for trace-correlation. These are not
// context.Context implementations in their own right; they get the
// DDTraceSpanType annotation. They're listed explicitly because they live
// outside the standard library, are not discoverable as
// context.Context implementations, and carry per-version layout that the
// runtime needs.
var ddTraceSpanTypeNames = []string{
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.span",
	"*gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.span",
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.spanContext",
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span",
	"*github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span",
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.SpanContext",
}

func addDDTraceSpanTypeNames(typeNames map[string]struct{}) {
	for _, name := range ddTraceSpanTypeNames {
		typeNames[name] = struct{}{}
	}
}

func analyzedProbesContainGoContext(analyzedProbes []analyzedProbe) bool {
	for i := range analyzedProbes {
		for _, expr := range analyzedProbes[i].expressions {
			if expr.rootVariable != nil && typeContainsGoContext(expr.rootVariable.Type) {
				return true
			}
		}
	}
	return false
}

// annotateSpecialGoTypes replaces certain *StructureType entries in the
// typeCatalog with dedicated wrapper IR types that carry the metadata the
// BPF chain walk needs:
//
//   - Every concrete context.Context implementation → GoContextImplementationType
//   - dd-trace-go tracer.span (v1) / tracer.Span (v2) → DDTraceSpanType
//
// contextImplIRTypeIDs is the set of IR type IDs known to be concrete
// context.Context implementations, discovered dynamically by walking the
// method index for implementors of the context.Context interface.
//
// Wrapping (rather than mutating GoTypeAttributes on every IR type) keeps
// the metadata off types that don't need it.
func annotateSpecialGoTypes(
	tc *typeCatalog,
	enabled bool,
	contextImplIRTypeIDs map[ir.TypeID]struct{},
) {
	if !enabled {
		return
	}
	for id, typ := range tc.typesByID {
		st, ok := typ.(*ir.StructureType)
		if !ok {
			continue
		}
		// dd-trace-go span types are special: they implement context.Context
		// but the chain walk needs DDTrace-specific layout instead of the
		// generic context impl wrapping. Match them first by their canonical
		// names.
		switch st.Name {
		case "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.span":
			if attrs, ok := ddTraceSpanLayout(tc, st, ir.DDTraceSpanV1); ok {
				tc.typesByID[id] = &ir.DDTraceSpanType{
					StructureType:     st,
					DDTraceAttributes: attrs,
				}
			}
			continue
		case "github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span":
			if attrs, ok := ddTraceSpanLayout(tc, st, ir.DDTraceSpanV2); ok {
				tc.typesByID[id] = &ir.DDTraceSpanType{
					StructureType:     st,
					DDTraceAttributes: attrs,
				}
			}
			continue
		}
		if _, isCtxImpl := contextImplIRTypeIDs[id]; !isCtxImpl {
			continue
		}
		attrs := goContextAttributes(st, contextImplIRTypeIDs)
		// context.valueCtx is the one concrete context impl whose extra
		// payload (key/val) the chain walk needs to read out. Match it by
		// canonical name; no other Go context implementation exposes a
		// key/value pair in this shape.
		if st.Name == "context.valueCtx" {
			if key, ok := st.FieldByName("key"); ok {
				attrs.KeyOffset = int32(key.Offset)
			}
			if val, ok := st.FieldByName("val"); ok {
				attrs.ValueOffset = int32(val.Offset)
			}
		}
		tc.typesByID[id] = &ir.GoContextImplementationType{
			StructureType:       st,
			GoContextAttributes: attrs,
		}
	}
}

func goContextAttributes(
	st *ir.StructureType, contextImplIRTypeIDs map[ir.TypeID]struct{},
) ir.GoContextAttributes {
	attrs := ir.GoContextAttributes{
		ContextOffset: ir.GoContextNoOffset,
		KeyOffset:     ir.GoContextNoOffset,
		ValueOffset:   ir.GoContextNoOffset,
	}
	visited := make(map[ir.TypeID]struct{})
	if off, ok := embeddedContextOffset(st, contextImplIRTypeIDs, visited); ok {
		attrs.ContextOffset = int32(off)
	}
	return attrs
}

// embeddedContextOffset finds the offset of the first field of type
// context.Context (the interface) reachable from st, descending through
// nested struct fields that are themselves concrete context implementations
// (per contextImplIRTypeIDs). This generalizes the rule "the parent context
// is the first field of type context.Context", which the BPF chain walk
// relies on to step from one context to the next. The visited set guards
// against cycles where impls embed each other.
func embeddedContextOffset(
	st *ir.StructureType,
	contextImplIRTypeIDs map[ir.TypeID]struct{},
	visited map[ir.TypeID]struct{},
) (uint32, bool) {
	if _, seen := visited[st.ID]; seen {
		return 0, false
	}
	visited[st.ID] = struct{}{}
	for _, f := range st.RawFields {
		if isPlaceholderIRType(f.Type) {
			continue
		}
		if _, ok := f.Type.(*ir.GoInterfaceType); ok &&
			f.Type.GetName() == "context.Context" {
			return f.Offset, true
		}
	}
	for _, f := range st.RawFields {
		if isPlaceholderIRType(f.Type) {
			continue
		}
		nested, ok := f.Type.(*ir.StructureType)
		if !ok {
			continue
		}
		if _, ok := contextImplIRTypeIDs[nested.ID]; !ok {
			continue
		}
		if off, ok := embeddedContextOffset(nested, contextImplIRTypeIDs, visited); ok {
			return f.Offset + off, true
		}
	}
	return 0, false
}

func ddTraceSpanLayout(
	tc *typeCatalog, st *ir.StructureType, kind ir.DDTraceSpanKind,
) (ir.DDTraceAttributes, bool) {
	traceIDField := "traceID"
	spanIDField := "spanID"
	parentIDField := "parentID"
	if kind == ir.DDTraceSpanV1 {
		traceIDField = "TraceID"
		spanIDField = "SpanID"
		parentIDField = "ParentID"
	}
	traceID, ok := st.FieldByName(traceIDField)
	if !ok {
		return ir.DDTraceAttributes{}, false
	}
	spanID, ok := st.FieldByName(spanIDField)
	if !ok {
		return ir.DDTraceAttributes{}, false
	}
	parentID, ok := st.FieldByName(parentIDField)
	if !ok {
		return ir.DDTraceAttributes{}, false
	}
	attrs := ir.DDTraceAttributes{
		SpanKind:       kind,
		TraceIDOffset:  int32(traceID.Offset),
		SpanIDOffset:   int32(spanID.Offset),
		ParentIDOffset: int32(parentID.Offset),
	}
	attrs.SpanContextOffset = ir.GoContextNoOffset
	attrs.SpanContextTraceIDOffset = ir.GoContextNoOffset
	contextField, ok := st.FieldByName("context")
	if !ok {
		return attrs, true
	}
	if isPlaceholderIRType(contextField.Type) {
		return attrs, true
	}
	attrs.SpanContextOffset = int32(contextField.Offset)
	ctxType, ok := tc.typesByID[contextField.Type.GetID()].(*ir.PointerType)
	if !ok {
		return attrs, true
	}
	if isPlaceholderIRType(ctxType.Pointee) {
		return attrs, true
	}
	ctxSt, ok := tc.typesByID[ctxType.Pointee.GetID()].(*ir.StructureType)
	if !ok {
		return attrs, true
	}
	ctxTraceID, ok := ctxSt.FieldByName("traceID")
	if ok {
		attrs.SpanContextTraceIDOffset = int32(ctxTraceID.Offset)
	}
	return attrs, true
}

func typeContainsGoContext(t ir.Type) bool {
	seen := make(map[ir.TypeID]struct{})
	var walk func(ir.Type) bool
	walk = func(t ir.Type) bool {
		if _, ok := seen[t.GetID()]; ok {
			return false
		}
		seen[t.GetID()] = struct{}{}
		if isPlaceholderIRType(t) {
			return false
		}
		if t.GetName() == "context.Context" {
			return true
		}
		switch t := t.(type) {
		case *ir.PointerType:
			return walk(t.Pointee)
		case *ir.StructureType:
			for _, f := range t.RawFields {
				if walk(f.Type) {
					return true
				}
			}
		case *ir.ArrayType:
			return walk(t.Element)
		case *ir.GoSliceHeaderType:
			return walk(t.Data)
		case *ir.GoSliceDataType:
			return walk(t.Element)
		case *ir.GoMapType:
			return walk(t.HeaderType)
		case *ir.GoHMapHeaderType:
			return walk(t.BucketType)
		case *ir.GoHMapBucketType:
			return walk(t.KeyType) || walk(t.ValueType)
		case *ir.GoSwissMapHeaderType:
			return walk(t.GroupType) || walk(t.TablePtrSliceType)
		case *ir.GoSwissMapGroupsType:
			return walk(t.GroupType) || walk(t.GroupSliceType)
		}
		return false
	}
	return walk(t)
}

func isPlaceholderIRType(t ir.Type) bool {
	switch t.(type) {
	case *placeHolderType, *pointeePlaceholderType:
		return true
	default:
		return false
	}
}
