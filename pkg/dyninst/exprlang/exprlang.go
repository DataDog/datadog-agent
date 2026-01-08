// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package exprlang provides utilities for parsing and working with expressions
// in the expression language of the dynamic instrumentation and live debugger products.
package exprlang

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/go-json-experiment/json/jsontext"
)

// Expr represents an expression in the DSL.
type Expr interface {
	expr() // marker method
}

// RefExpr represents a reference to a variable.
type RefExpr struct {
	Ref string
}

func (re *RefExpr) expr() {}

// GetMemberExpr represents a member access expression (e.g., obj.field).
type GetMemberExpr struct {
	Base   Expr
	Member string
}

func (gme *GetMemberExpr) expr() {}

// UnsupportedExpr represents an expression type that is not yet supported.
type UnsupportedExpr struct {
	Operation string
	Argument  jsontext.Value
}

func (ue *UnsupportedExpr) expr() {}

var decoderPool = sync.Pool{
	New: func() any {
		r := &bytes.Reader{}
		d := jsontext.NewDecoder(r)
		return &pooledDecoder{decoder: d, reader: r}
	},
}

type pooledDecoder struct {
	decoder *jsontext.Decoder
	reader  *bytes.Reader
}

func getPooledDecoder(data []byte) *pooledDecoder {
	d := decoderPool.Get().(*pooledDecoder)
	d.reader.Reset(data)
	return d
}

func (d *pooledDecoder) put() {
	d.reader.Reset(nil)
	d.decoder.Reset(d.reader)
	decoderPool.Put(d)
}

// Parse parses a DSL JSON expression into a strongly-typed AST node.
func Parse(dslJSON []byte) (Expr, error) {
	if len(dslJSON) == 0 {
		return nil, errors.New("parse error: empty DSL expression")
	}
	pooled := getPooledDecoder(dslJSON)
	defer pooled.put()
	dec := pooled.decoder

	// Closure to read and validate closing brace
	readClosingBrace := func() error {
		endObj, err := dec.ReadToken()
		if err != nil {
			return fmt.Errorf("parse error: failed to read closing brace: %w", err)
		}
		if kind := endObj.Kind(); kind != '}' {
			return fmt.Errorf("parse error: malformed DSL: got token %v (%v), expected }", endObj, kind)
		}
		return nil
	}

	// Ensure we have a JSON object
	objStart, err := dec.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("parse error: failed to read token: %w", err)
	}
	if kind := objStart.Kind(); kind != '{' {
		return nil, fmt.Errorf("parse error: malformed DSL: got token %v (%v), expected {", objStart, kind)
	}

	// Read the operation key
	key, err := dec.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("parse error: failed to read operation key: %w", err)
	}
	if kind := key.Kind(); kind != '"' {
		return nil, fmt.Errorf("parse error: malformed DSL: got key token %v (%v), expected string", key, kind)
	}

	operation := key.String()
	switch operation {
	case "ref":
		// Read the ref value
		val, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read ref value: %w", err)
		}
		if kind := val.Kind(); kind != '"' {
			return nil, fmt.Errorf("parse error: malformed ref: got value token %v (%v), expected string", val, kind)
		}

		refValue := val.String()
		if refValue == "" {
			return nil, errors.New("parse error: ref value cannot be empty")
		}

		if err := readClosingBrace(); err != nil {
			return nil, err
		}

		return &RefExpr{Ref: refValue}, nil
	case "getmember":
		// Handle both lowercase and camelCase variants.
		// Read the array argument [baseExpr, "memberName"]
		arrStart, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read getmember array start: %w", err)
		}
		if kind := arrStart.Kind(); kind != '[' {
			return nil, fmt.Errorf("parse error: malformed getmember: got token %v (%v), expected [", arrStart, kind)
		}

		// Read base expression (first element) - use ReadValue to get the complete JSON value
		baseExprJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read getmember base expression: %w", err)
		}

		// Parse base expression recursively using the bytes returned by ReadValue
		baseExpr, err := Parse(baseExprJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse getmember base expression: %w", err)
		}

		// Read comma separator (should be next token after ReadValue)
		nextToken, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read getmember separator: %w", err)
		}
		var memberName string
		switch nextKind := nextToken.Kind(); nextKind {
		case ',':
			// Comma found, read member name
			memberToken, err := dec.ReadToken()
			if err != nil {
				return nil, fmt.Errorf("parse error: failed to read getmember member name: %w", err)
			}
			if kind := memberToken.Kind(); kind != '"' {
				return nil, fmt.Errorf("parse error: malformed getmember: member name must be string, got %v (%v)", memberToken, kind)
			}
			memberName = memberToken.String()
		case '"':
			// No comma, member name is next token
			memberName = nextToken.String()
		default:
			return nil, fmt.Errorf("parse error: malformed getmember: expected comma or string, got %v (%v)", nextToken, nextKind)
		}

		// Read array closing bracket
		arrEnd, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read getmember array end: %w", err)
		}
		if kind := arrEnd.Kind(); kind != ']' {
			return nil, fmt.Errorf("parse error: malformed getmember: got token %v (%v), expected ]", arrEnd, kind)
		}

		if err := readClosingBrace(); err != nil {
			return nil, err
		}

		return &GetMemberExpr{Base: baseExpr, Member: memberName}, nil
	default:
		// Read the argument for unsupported operations.
		// We track the offset before and after reading the argument value
		// so we can store the value after the decoder resets the internal slice
		// containing the value bytes.
		argument, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read argument: %w", err)
		}
		postReadOffset := dec.InputOffset()
		if err := readClosingBrace(); err != nil {
			return nil, err
		}
		unsupportedArgument := dslJSON[postReadOffset-int64(len(argument)) : postReadOffset]
		return &UnsupportedExpr{
			Operation: operation,
			Argument:  unsupportedArgument,
		}, nil
	}
}
