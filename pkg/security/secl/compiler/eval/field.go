// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import "errors"

// Field name
type Field = string

// FieldValueType represents the type of the value of a field
type FieldValueType int

// Field value types
const (
	ScalarValueType   FieldValueType = 1 << 0
	GlobValueType     FieldValueType = 1 << 1
	PatternValueType  FieldValueType = 1 << 2
	RegexpValueType   FieldValueType = 1 << 3
	BitmaskValueType  FieldValueType = 1 << 4
	VariableValueType FieldValueType = 1 << 5
	IPNetValueType    FieldValueType = 1 << 6
)

// MarshalJSON returns the JSON encoding of the FieldValueType
func (t FieldValueType) MarshalJSON() ([]byte, error) {
	s := t.String()
	if s == "" {
		return nil, errors.New("invalid field value type")
	}

	return []byte(`"` + s + `"`), nil
}

func (t FieldValueType) String() string {
	switch t {
	case ScalarValueType:
		return "scalar"
	case GlobValueType:
		return "glob"
	case PatternValueType:
		return "pattern"
	case RegexpValueType:
		return "regex"
	case BitmaskValueType:
		return "bitmask"
	case VariableValueType:
		return "variable"
	case IPNetValueType:
		return "ip"
	}

	return ""
}

// FieldValue describes a field value with its type
type FieldValue struct {
	Value interface{}
	Type  FieldValueType
}
