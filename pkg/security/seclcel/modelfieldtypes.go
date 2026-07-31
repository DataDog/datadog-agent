// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"strings"

	"github.com/google/cel-go/common/types"
)

// ModelFieldTypes answers from the generated SECL object types, so it describes
// exactly the fields the agent's model exposes.
type ModelFieldTypes struct{}

// ListPrefix implements FieldTypes.
func (ModelFieldTypes) ListPrefix(field string) string {
	_, prefix, ok := walkModel(field)
	if !ok {
		return ""
	}
	return prefix
}

// IsListLeaf implements FieldTypes.
func (ModelFieldTypes) IsListLeaf(field string) bool {
	fieldType, _, ok := walkModel(field)
	return ok && fieldType.Kind() == types.ListKind
}

// IsPseudoField implements FieldTypes.
func (ModelFieldTypes) IsPseudoField(field string) bool {
	// A pseudo field is absent from the type tree, because `x.length` would
	// require `x` to be both a value and an object, while the field it derives
	// from is present.
	if _, _, ok := walkModel(field); ok {
		return false
	}

	for _, suffix := range []string{lengthSuffix, rootDomainSuffix} {
		if base, found := strings.CutSuffix(field, suffix); found {
			if _, _, ok := walkModel(base); ok {
				return true
			}
		}
	}

	return false
}

// walkModel resolves a dotted SECL field name against the generated object
// types.
//
// It returns the type the name denotes and the longest prefix of it that is a
// list of objects, which is the range an exists() would iterate. Descending
// through such a list yields the element type, so the returned type is the one a
// single element holds — a string for process.ancestors.file.name.
func walkModel(field string) (*types.Type, string, bool) {
	segments := strings.Split(field, ".")

	fieldType, ok := modelRoots[segments[0]]
	if !ok {
		return nil, "", false
	}

	prefix, listPrefix := segments[0], ""
	if elem, isObjectList := objectListElem(fieldType); isObjectList {
		listPrefix, fieldType = prefix, elem
	}

	for _, segment := range segments[1:] {
		if fieldType.Kind() != types.StructKind {
			return nil, "", false
		}
		next, ok := modelShapes[fieldType.TypeName()][segment]
		if !ok {
			return nil, "", false
		}

		fieldType = next
		prefix += "." + segment
		if elem, isObjectList := objectListElem(fieldType); isObjectList {
			listPrefix, fieldType = prefix, elem
		}
	}

	return fieldType, listPrefix, true
}

// objectListElem reports whether a type is a list of objects, and returns the
// element type. Only such a list can be the range of a quantifier that then
// selects a member: a list of values is the leaf case IsListLeaf covers.
func objectListElem(t *types.Type) (*types.Type, bool) {
	if t.Kind() != types.ListKind {
		return nil, false
	}
	elem := t.Parameters()[0]
	if elem.Kind() != types.StructKind {
		return nil, false
	}
	return elem, true
}
