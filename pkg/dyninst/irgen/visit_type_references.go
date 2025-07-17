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
func rewritePlaceholderReferences(tc *typeCatalog) {
	visitTypeReferences(tc, func(t *ir.Type) {
		if placeholder, ok := (*t).(*placeHolderType); ok {
			(*t) = tc.typesByID[placeholder.id]
			return
		}
	})
}

func visitTypeReferences(tc *typeCatalog, f func(t *ir.Type)) {
	for _, t := range tc.typesByID {
		switch t := t.(type) {

		case *ir.ArrayType:
			f(&t.Element)

		case *ir.EventRootType:
			for i := range t.Expressions {
				f(&t.Expressions[i].Expression.Type)
			}

		case *ir.GoHMapBucketType:
			f(&t.KeyType)
			f(&t.ValueType)

		case *ir.GoMapType:
			f(&t.HeaderType)

		case *ir.PointerType:
			f(&t.Pointee)

		case *ir.StructureType:
			for i := range t.Fields {
				f(&t.Fields[i].Type)
			}

		case *ir.GoSliceDataType:
			f(&t.Element)

		case *ir.BaseType:
		case *ir.GoChannelType:
		case *ir.GoEmptyInterfaceType:
		case *ir.GoHMapHeaderType:
		case *ir.GoInterfaceType:
		case *ir.GoSliceHeaderType:
		case *ir.GoStringDataType:
		case *ir.GoStringHeaderType:
		case *ir.GoSubroutineType:
		case *ir.GoSwissMapGroupsType:
		case *ir.GoSwissMapHeaderType:
		case *ir.VoidPointerType:
		default:
			panic(fmt.Sprintf("unexpected ir.Type: %#v", t))
		}
	}
}
