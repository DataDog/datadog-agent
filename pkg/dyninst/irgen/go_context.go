// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

var ddTraceGoContextTypes = []string{
	"context.Context",
	"*context.afterFuncCtx",
	"*context.cancelCtx",
	"*context.stopCtx",
	"*context.timerCtx",
	"*context.valueCtx",
	"*context.withoutCancelCtx",
	"*os/signal.signalCtx",
	"context.afterFuncCtx",
	"context.backgroundCtx",
	"context.cancelCtx",
	"context.emptyCtx",
	"context.stopCtx",
	"context.timerCtx",
	"context.todoCtx",
	"context.valueCtx",
	"context.withoutCancelCtx",
	"os/signal.signalCtx",
	"*gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.span",
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.span",
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.spanContext",
	"*github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span",
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span",
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.SpanContext",
}

func addDDTraceGoContextTypes(typeNames map[string]struct{}) {
	for _, name := range ddTraceGoContextTypes {
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
//   - context.{cancelCtx, valueCtx, …} → GoContextImplementationType
//   - dd-trace-go tracer.span (v1) / tracer.Span (v2) → DDTraceSpanType
//
// Wrapping (rather than mutating GoTypeAttributes on every IR type) keeps
// the metadata off types that don't need it.
func annotateSpecialGoTypes(tc *typeCatalog, enabled bool) {
	if !enabled {
		return
	}
	for id, typ := range tc.typesByID {
		st, ok := typ.(*ir.StructureType)
		if !ok {
			continue
		}
		switch st.Name {
		case "context.valueCtx":
			attrs := goContextAttributes(st)
			if key, ok := st.FieldByName("key"); ok {
				attrs.KeyOffset = int32(key.Offset)
			}
			if val, ok := st.FieldByName("val"); ok {
				attrs.ValueOffset = int32(val.Offset)
			}
			tc.typesByID[id] = &ir.GoContextImplementationType{
				StructureType:       st,
				GoContextAttributes: attrs,
			}
		case "context.afterFuncCtx",
			"context.backgroundCtx",
			"context.cancelCtx",
			"context.emptyCtx",
			"context.stopCtx",
			"context.timerCtx",
			"context.todoCtx",
			"context.withoutCancelCtx",
			"os/signal.signalCtx":
			tc.typesByID[id] = &ir.GoContextImplementationType{
				StructureType:       st,
				GoContextAttributes: goContextAttributes(st),
			}
		case "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer.span":
			if attrs, ok := ddTraceSpanLayout(tc, st, ir.DDTraceSpanV1); ok {
				tc.typesByID[id] = &ir.DDTraceSpanType{
					StructureType:     st,
					DDTraceAttributes: attrs,
				}
			}
		case "github.com/DataDog/dd-trace-go/v2/ddtrace/tracer.Span":
			if attrs, ok := ddTraceSpanLayout(tc, st, ir.DDTraceSpanV2); ok {
				tc.typesByID[id] = &ir.DDTraceSpanType{
					StructureType:     st,
					DDTraceAttributes: attrs,
				}
			}
		}
	}
}

func goContextAttributes(st *ir.StructureType) ir.GoContextAttributes {
	attrs := ir.GoContextAttributes{
		ContextOffset: ir.GoContextNoOffset,
		KeyOffset:     ir.GoContextNoOffset,
		ValueOffset:   ir.GoContextNoOffset,
	}
	if off, ok := embeddedContextOffset(st); ok {
		attrs.ContextOffset = int32(off)
	}
	return attrs
}

func embeddedContextOffset(st *ir.StructureType) (uint32, bool) {
	for _, name := range []string{"Context", "c"} {
		if f, ok := st.FieldByName(name); ok &&
			!isPlaceholderIRType(f.Type) &&
			f.Type.GetName() == "context.Context" {
			return f.Offset, true
		}
	}
	for _, f := range st.RawFields {
		if isPlaceholderIRType(f.Type) {
			continue
		}
		nested, ok := f.Type.(*ir.StructureType)
		if !ok || !isGoContextImplName(nested.Name) {
			continue
		}
		if off, ok := embeddedContextOffset(nested); ok {
			return f.Offset + off, true
		}
	}
	return 0, false
}

func isGoContextImplName(name string) bool {
	return slices.Contains([]string{
		"context.afterFuncCtx",
		"context.cancelCtx",
		"context.stopCtx",
		"context.timerCtx",
		"context.valueCtx",
		"context.withoutCancelCtx",
		"os/signal.signalCtx",
	}, name)
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
