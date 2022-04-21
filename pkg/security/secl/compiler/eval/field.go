// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"fmt"
	"net"
)

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
	IPValueType       FieldValueType = 1 << 6
	CIDRValueType     FieldValueType = 1 << 7
)

// FieldValue describes a field value with its type
type FieldValue struct {
	Value interface{}
	Type  FieldValueType

	IPMatcher IPMatcher
}

// Compile the regular expression or the pattern
func (f *FieldValue) Compile() error {
	switch f.Type {
	case IPValueType, CIDRValueType:
		value, ok := f.Value.(string)
		if !ok {
			return fmt.Errorf("invalid IP `%v`", f.Value)
		}

		matcher, err := NewIPMatcher(f.Type, value)
		if err != nil {
			return err
		}

		f.IPMatcher = matcher
	}

	return nil
}

// NewIPFieldValue returns a new FieldValue pointer initiailised with the provided IP
func NewIPFieldValue(ip net.IP, net *net.IPNet) *FieldValue {
	if net != nil {
		return &FieldValue{
			Type: CIDRValueType,
			IPMatcher: &CIDRMatcher{
				net: net,
			},
		}
	}
	return &FieldValue{
		Type: IPValueType,
		IPMatcher: &SingleIPMatcher{
			ip: ip,
		},
	}
}
