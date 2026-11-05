// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"fmt"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// The payload is an opentelemetry.proto.processcontext.v1development.ProcessContext.
// It is decoded with protowire rather than the generated types: proto.Unmarshal drags
// the reflective protobuf runtime into the binary, which costs cws-instrumentation
// more than 1 MiB for the few attributes read here. Field numbers come from
// go.opentelemetry.io/proto/slim/otlp.
const (
	// ProcessContext.resource (Resource) and attributes (repeated KeyValue).
	processContextResource   protowire.Number = 1
	processContextAttributes protowire.Number = 2

	// Resource.attributes (repeated KeyValue).
	resourceAttributes protowire.Number = 1

	// KeyValue.
	keyValueKey   protowire.Number = 1
	keyValueValue protowire.Number = 2

	// AnyValue, all members of the value oneof.
	anyValueString protowire.Number = 1
	anyValueBool   protowire.Number = 2
	anyValueInt    protowire.Number = 3
	anyValueDouble protowire.Number = 4
	anyValueArray  protowire.Number = 5
	anyValueKVList protowire.Number = 6
	anyValueBytes  protowire.Number = 7

	// ArrayValue.values (repeated AnyValue).
	arrayValueValues protowire.Number = 1
)

// anyValueTypes is the wire type of each AnyValue oneof member. A member arriving with
// another wire type is an unknown field, as proto.Unmarshal treats it.
var anyValueTypes = map[protowire.Number]protowire.Type{
	anyValueString: protowire.BytesType,
	anyValueBool:   protowire.VarintType,
	anyValueInt:    protowire.VarintType,
	anyValueDouble: protowire.Fixed64Type,
	anyValueArray:  protowire.BytesType,
	anyValueKVList: protowire.BytesType,
	anyValueBytes:  protowire.BytesType,
}

// Context is the decoded process context.
type Context struct {
	resource   []keyValue
	attributes []keyValue
}

// ProcessContext is what Read returns; nil means no process context.
type ProcessContext = *Context

type keyValue struct {
	key string
	// value is nil when the KeyValue carries no value field.
	value *anyValue
}

// anyValue keeps only what the accessors read: the string member, and the string
// members of an array member's elements.
type anyValue struct {
	// kind is the oneof member that is set, 0 for none.
	kind  protowire.Number
	str   string
	array []string
}

// decodeProcessContext decodes payload the way proto.Unmarshal would, except that
// the contents of fields the accessors never read (resource fields other than
// attributes, kvlist_value, the nested values of array elements) are only checked
// to be well-framed, not decoded.
func decodeProcessContext(payload []byte) (ProcessContext, error) {
	ctx := &Context{}
	// A message field repeated on the wire is merged: see decodeKeyValue.
	var resource []byte
	err := walkFields(payload, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		switch num {
		case processContextResource:
			resource = append(resource, v...)
		case processContextAttributes:
			kv, err := decodeKeyValue(v)
			if err != nil {
				return err
			}
			ctx.attributes = append(ctx.attributes, kv)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = walkFields(resource, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if num != resourceAttributes || typ != protowire.BytesType {
			return nil
		}
		kv, err := decodeKeyValue(v)
		if err != nil {
			return err
		}
		ctx.resource = append(ctx.resource, kv)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func decodeKeyValue(b []byte) (keyValue, error) {
	var kv keyValue
	// A message field repeated on the wire is merged, and merging two encoded messages
	// is decoding their concatenation.
	var value []byte
	hasValue := false
	err := walkFields(b, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if typ != protowire.BytesType {
			return nil
		}
		switch num {
		case keyValueKey:
			key, err := decodeString(v)
			if err != nil {
				return err
			}
			kv.key = key
		case keyValueValue:
			value = append(value, v...)
			hasValue = true
		}
		return nil
	})
	if err != nil {
		return kv, err
	}
	if !hasValue {
		return kv, nil
	}

	kind, str, array, err := decodeAnyValue(value)
	if err != nil {
		return kv, err
	}
	kv.value = &anyValue{kind: kind, str: str}
	if kind != anyValueArray {
		return kv, nil
	}
	err = walkFields(array, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if num != arrayValueValues || typ != protowire.BytesType {
			return nil
		}
		elemKind, elemStr, _, err := decodeAnyValue(v)
		if err != nil {
			return err
		}
		if elemKind != anyValueString {
			elemStr = ""
		}
		kv.value.array = append(kv.value.array, elemStr)
		return nil
	})
	return kv, err
}

// decodeAnyValue returns the oneof member that is set, the string member if it is the
// one, and the encoded ArrayValue if the array member is.
func decodeAnyValue(b []byte) (kind protowire.Number, str string, array []byte, err error) {
	err = walkFields(b, func(num protowire.Number, typ protowire.Type, v []byte) error {
		if want, ok := anyValueTypes[num]; !ok || typ != want {
			return nil
		}
		switch num {
		case anyValueString:
			s, err := decodeString(v)
			if err != nil {
				return err
			}
			str = s
		case anyValueArray:
			// Setting the same message member again merges into it; setting any other
			// member first clears it.
			if kind != anyValueArray {
				array = nil
			}
			array = append(array, v...)
		}
		kind = num
		return nil
	})
	return kind, str, array, err
}

// decodeString applies the UTF-8 check proto.Unmarshal makes on proto3 strings.
func decodeString(v []byte) (string, error) {
	if !utf8.Valid(v) {
		return "", fmt.Errorf("%w: invalid UTF-8 in string field", ErrMalformed)
	}
	return string(v), nil
}

// walkFields calls fn with every field of the message encoded in b, in wire order. v is
// the contents of a length-delimited field, and the raw encoding of any other.
func walkFields(b []byte, fn func(num protowire.Number, typ protowire.Type, v []byte) error) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return fmt.Errorf("%w: %w", ErrMalformed, protowire.ParseError(n))
		}
		b = b[n:]

		var v []byte
		if typ == protowire.BytesType {
			v, n = protowire.ConsumeBytes(b)
		} else {
			n = protowire.ConsumeFieldValue(num, typ, b)
			if n >= 0 {
				v = b[:n]
			}
		}
		if n < 0 {
			return fmt.Errorf("%w: %w", ErrMalformed, protowire.ParseError(n))
		}
		b = b[n:]

		if err := fn(num, typ, v); err != nil {
			return err
		}
	}
	return nil
}
