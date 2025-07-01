// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// nolint: unused
func (ev *Event) setStringArrayFieldValue(field string, fv *[]string, value interface{}) error {
	switch rv := value.(type) {
	case string:
		*fv = append(*fv, rv)
	case []string:
		*fv = append(*fv, rv...)
	default:
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	return nil
}

// nolint: unused
func (ev *Event) setIntArrayFieldValue(field string, fv *[]int, value interface{}) error {
	switch rv := value.(type) {
	case int:
		*fv = append(*fv, rv)
	case []int:
		*fv = append(*fv, rv...)
	default:
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	return nil
}

// nolint: unused
func (ev *Event) setBoolArrayFieldValue(field string, fv *[]bool, value interface{}) error {
	switch rv := value.(type) {
	case bool:
		*fv = append(*fv, rv)
	case []bool:
		*fv = append(*fv, rv...)
	default:
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	return nil
}

// nolint: unused
func (ev *Event) setStringFieldValue(field string, fv *string, value interface{}) error {
	rv, ok := value.(string)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	*fv = rv
	return nil
}

// nolint: unused
func (ev *Event) setBoolFieldValue(field string, fv *bool, value interface{}) error {
	rv, ok := value.(bool)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	*fv = rv
	return nil
}

// nolint: unused
func (ev *Event) setUint8FieldValue(field string, fv *uint8, value interface{}) error {
	rv, ok := value.(int)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	if rv < 0 || rv > math.MaxUint8 {
		return &eval.ErrValueOutOfRange{Field: field}
	}
	*fv = uint8(rv)
	return nil
}

// nolint: unused
func (ev *Event) setUint16FieldValue(field string, fv *uint16, value interface{}) error {
	rv, ok := value.(int)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	if rv < 0 || rv > math.MaxUint16 {
		return &eval.ErrValueOutOfRange{Field: field}
	}
	*fv = uint16(rv)
	return nil
}

// nolint: unused
func (ev *Event) setUint32FieldValue(field string, fv *uint32, value interface{}) error {
	rv, ok := value.(int)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	*fv = uint32(rv)
	return nil
}

// nolint: unused
func (ev *Event) setUint64FieldValue(field string, fv *uint64, value interface{}) error {
	rv, ok := value.(int)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	*fv = uint64(rv)
	return nil
}

// nolint: unused
func (ev *Event) setInt64FieldValue(field string, fv *int64, value interface{}) error {
	rv, ok := value.(int)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	*fv = int64(rv)
	return nil
}

// nolint: unused
func (ev *Event) setIntFieldValue(field string, fv *int, value interface{}) error {
	rv, ok := value.(int)
	if !ok {
		return &eval.ErrValueTypeMismatch{Field: field}
	}
	*fv = rv
	return nil
}
