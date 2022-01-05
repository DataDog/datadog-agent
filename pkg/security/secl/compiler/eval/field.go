// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
)

// Field name
type Field = string

// FieldValueType represents the type of the value of a field
type FieldValueType int

// Field value types
const (
	ScalarValueType   FieldValueType = 1 << 0
	PatternValueType  FieldValueType = 1 << 1
	RegexpValueType   FieldValueType = 1 << 2
	BitmaskValueType  FieldValueType = 1 << 3
	VariableValueType FieldValueType = 1 << 4
)

// FieldValue describes a field value with its type
type FieldValue struct {
	Value interface{}
	Type  FieldValueType

	PatternMatcher PatternMatcher
}

// Compile the regular expression or the pattern
func (f *FieldValue) Compile() error {
	switch f.Type {
	case PatternValueType:
		value, ok := f.Value.(string)
		if !ok {
			return fmt.Errorf("invalid pattern `%v`", f.Value)
		}

		var matcher SimplePatternMatcher
		if err := matcher.Compile(value); err != nil {
			return fmt.Errorf("invalid pattern `%s`: %s", value, err)
		}
		f.PatternMatcher = &matcher
	case RegexpValueType:
		value, ok := f.Value.(string)
		if !ok {
			return fmt.Errorf("invalid regexp `%v`", f.Value)
		}

		var matcher RegexpPatternMatcher
		if err := matcher.Compile(value); err != nil {
			return fmt.Errorf("invalid pattern `%s`: %s", value, err)
		}
		f.PatternMatcher = &matcher
	}

	return nil
}
