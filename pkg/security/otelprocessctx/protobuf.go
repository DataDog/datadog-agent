// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// A hand-written decoder for the sliver of
// opentelemetry.proto.processcontext.v1development.ProcessContext this reader needs.
//
// Field numbers, from the proto definitions:
//
//	ProcessContext { Resource resource = 1; repeated KeyValue extra_attributes = 2; }
//	Resource       { repeated KeyValue attributes = 1; uint32 dropped = 2; }
//	KeyValue       { string key = 1; AnyValue value = 2; uint32 key_ref = 3; }
//	AnyValue       { string string_value = 1; bool = 2; int64 int_value = 3;
//	                 double = 4; ArrayValue array_value = 5; kvlist = 6; bytes = 7; }
//	ArrayValue     { repeated AnyValue values = 1; }
const (
	fieldProcessCtxResource        = 1
	fieldProcessCtxExtraAttributes = 2

	fieldResourceAttributes = 1

	fieldKeyValueKey   = 1
	fieldKeyValueValue = 2

	fieldAnyValueString = 1
	fieldAnyValueInt    = 3
	fieldAnyValueArray  = 5

	fieldArrayValueValues = 1
)

// maxValueDepth bounds how deeply AnyValue and ArrayValue may nest into each other.
// This reader only ever uses a string, an int or an array of strings, so a payload
// nested deeper than that is either corrupt or hostile, and the payload is read out
// of a process that is not trusted to bound the recursion for us.
const maxValueDepth = 4

// Protobuf wire types.
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

var (
	errTruncated    = errors.New("truncated protobuf message")
	errInvalidField = errors.New("invalid protobuf field number 0")
	errTooDeep      = errors.New("protobuf value nested too deeply")
)

// buffer walks a protobuf message field by field.
type buffer struct {
	data []byte
}

// next returns the field number and payload of the next field. Varint fields yield
// their value in num, length-delimited ones their bytes in payload.
func (b *buffer) next() (field int, num uint64, payload []byte, err error) {
	tag, err := b.varint()
	if err != nil {
		return 0, 0, nil, err
	}
	field, wire := int(tag>>3), int(tag&7)
	if field == 0 {
		return 0, 0, nil, errInvalidField
	}

	switch wire {
	case wireVarint:
		num, err = b.varint()
	case wireFixed64:
		payload, err = b.take(8)
	case wireBytes:
		var size uint64
		if size, err = b.varint(); err == nil {
			payload, err = b.take(int(size))
		}
	case wireFixed32:
		payload, err = b.take(4)
	default:
		err = fmt.Errorf("unsupported protobuf wire type %d", wire)
	}
	return field, num, payload, err
}

func (b *buffer) varint() (uint64, error) {
	v, n := binary.Uvarint(b.data)
	if n <= 0 {
		return 0, errTruncated
	}
	b.data = b.data[n:]
	return v, nil
}

func (b *buffer) take(n int) ([]byte, error) {
	if n < 0 || n > len(b.data) {
		return nil, errTruncated
	}
	out := b.data[:n]
	b.data = b.data[n:]
	return out, nil
}

func (b *buffer) empty() bool { return len(b.data) == 0 }

// decodeProcessContext decodes a ProcessContext payload into its two halves.
func decodeProcessContext(payload []byte) (ProcessContext, error) {
	ctx := ProcessContext{Resource: Attributes{}, Attributes: Attributes{}}

	buf := buffer{data: payload}
	for !buf.empty() {
		field, _, data, err := buf.next()
		if err != nil {
			return ProcessContext{}, err
		}

		switch field {
		case fieldProcessCtxResource:
			err = decodeResource(data, ctx.Resource)
		case fieldProcessCtxExtraAttributes:
			err = addAttribute(ctx.Attributes, data)
		}
		if err != nil {
			return ProcessContext{}, err
		}
	}

	return ctx, nil
}

// decodeResource adds the attributes of a Resource message to attrs.
func decodeResource(data []byte, attrs Attributes) error {
	buf := buffer{data: data}
	for !buf.empty() {
		field, _, payload, err := buf.next()
		if err != nil {
			return err
		}
		if field != fieldResourceAttributes {
			continue
		}
		if err := addAttribute(attrs, payload); err != nil {
			return err
		}
	}

	return nil
}

// addAttribute decodes one KeyValue and adds it to attrs.
func addAttribute(attrs Attributes, data []byte) error {
	key, value, err := decodeKeyValue(data)
	if err != nil {
		return err
	}
	// A key_ref-only attribute has no name this reader can resolve, and an unset
	// value carries nothing. Both are simply not attributes we can use.
	if key == "" || value.Kind == ValueUnset {
		return nil
	}
	attrs[key] = value
	return nil
}

func decodeKeyValue(data []byte) (string, Value, error) {
	var key string
	var value Value

	buf := buffer{data: data}
	for !buf.empty() {
		field, _, payload, err := buf.next()
		if err != nil {
			return "", Value{}, err
		}
		switch field {
		case fieldKeyValueKey:
			key = string(payload)
		case fieldKeyValueValue:
			if value, err = decodeAnyValue(payload, 0); err != nil {
				return "", Value{}, err
			}
		}
	}

	return key, value, nil
}

func decodeAnyValue(data []byte, depth int) (Value, error) {
	if depth > maxValueDepth {
		return Value{}, errTooDeep
	}

	var value Value

	buf := buffer{data: data}
	for !buf.empty() {
		field, num, payload, err := buf.next()
		if err != nil {
			return Value{}, err
		}
		switch field {
		case fieldAnyValueString:
			value = Value{Kind: ValueString, Str: string(payload)}
		case fieldAnyValueInt:
			value = Value{Kind: ValueInt, Int: int64(num)}
		case fieldAnyValueArray:
			strs, err := decodeStringArray(payload, depth+1)
			if err != nil {
				return Value{}, err
			}
			value = Value{Kind: ValueStringSlice, StringSlice: strs}
		}
	}

	return value, nil
}

// decodeStringArray decodes an ArrayValue as a string slice. An entry that is not a
// string is decoded as the empty string rather than dropped, so the positions of the
// entries that are strings, which is what a key map is indexed by, stay correct.
func decodeStringArray(data []byte, depth int) ([]string, error) {
	var strs []string

	buf := buffer{data: data}
	for !buf.empty() {
		field, _, payload, err := buf.next()
		if err != nil {
			return nil, err
		}
		if field != fieldArrayValueValues {
			continue
		}
		entry, err := decodeAnyValue(payload, depth+1)
		if err != nil {
			return nil, err
		}
		strs = append(strs, entry.Str)
	}

	return strs, nil
}
