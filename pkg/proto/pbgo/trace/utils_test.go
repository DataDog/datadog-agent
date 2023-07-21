// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	fmt "fmt"
	reflect "reflect"
	"testing"
)

func TestShallowCopy(t *testing.T) {
	// These tests ensure that the ShallowCopy functions for Span and TraceChunk
	// copy all of the available fields.
	t.Run("span", func(t *testing.T) {
		typ := reflect.TypeOf(&Span{})
		for i := 0; i < typ.Elem().NumField(); i++ {
			field := typ.Elem().Field(i)
			if field.PkgPath != `` {
				continue
			}
			method, ok := typ.MethodByName(`Get` + field.Name)
			if !ok ||
				method.Type.NumIn() != 1 ||
				method.Type.NumOut() != 1 ||
				method.Type.Out(0) != field.Type {
				continue
			}
			if _, ok := spanCopiedFields[field.Name]; !ok {
				panic(fmt.Sprintf("pkg/trace/pb/span_utils.go: ShallowCopy needs to be updated for new Span fields. Missing: %s", field.Name))
			}
		}
	})

	t.Run("trace-chunk", func(t *testing.T) {
		typ := reflect.TypeOf(&TraceChunk{})
		for i := 0; i < typ.Elem().NumField(); i++ {
			field := typ.Elem().Field(i)
			if field.PkgPath != `` {
				continue
			}
			method, ok := typ.MethodByName(`Get` + field.Name)
			if !ok ||
				method.Type.NumIn() != 1 ||
				method.Type.NumOut() != 1 ||
				method.Type.Out(0) != field.Type {
				continue
			}
			if _, ok := traceChunkCopiedFields[field.Name]; !ok {
				panic(fmt.Sprintf("pkg/trace/pb/tracer_payload_utils.go: ShallowCopy needs to be updated for new TraceChunk fields. Missing: %s", field.Name))
			}
		}
	})
}
