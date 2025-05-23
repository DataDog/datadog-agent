// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// When cycles occur during the process of building the type graph, a
// placeholder type will be used to represent the type. This function
// rewrites the references to the types to the actual types after
func rewriteTypeReferences(tc *typeCatalog) {
	maybeRewrite := func(t *ir.Type) {
		if placeholder, ok := (*t).(*placeHolderType); ok {
			(*t) = tc.typesByID[placeholder.id]
		}
	}
	for _, t := range tc.typesByID {
		switch t := t.(type) {
		case *ir.ArrayType:
			maybeRewrite(&t.Element)
		case *ir.BaseType:
		case *ir.EventRootType:
			for i := range t.Expressions {
				maybeRewrite(&t.Expressions[i].Expression.Type)
			}
		case *ir.GoChannelType:
		case *ir.GoEmptyInterfaceType:
		case *ir.GoHMapBucketType:
			maybeRewrite(&t.KeyType)
			maybeRewrite(&t.ValueType)
		case *ir.GoHMapHeaderType:
		case *ir.GoInterfaceType:
		case *ir.GoMapType:
			maybeRewrite(&t.HeaderType)
		case *ir.GoSliceDataType:
		case *ir.GoSliceHeaderType:
		case *ir.GoStringDataType:
		case *ir.GoStringHeaderType:
		case *ir.GoSubroutineType:
		case *ir.GoSwissMapGroupsType:
			maybeRewrite(&t.GroupType)
			maybeRewrite(&t.GroupSliceType)
		case *ir.GoSwissMapHeaderType:
			maybeRewrite(&t.TablePtrSliceType)
			maybeRewrite(&t.GroupType)
		case *ir.PointerType:
			maybeRewrite(&t.Pointee)
		case *ir.StructureType:
			for i := range t.Fields {
				maybeRewrite(&t.Fields[i].Type)
			}
		default:
			panic(fmt.Sprintf("unexpected ir.Type: %#v", t))
		}
	}
}
