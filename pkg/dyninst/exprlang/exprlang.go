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
		return nil, fmt.Errorf("parse error: empty DSL expression")
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
			return nil, fmt.Errorf("parse error: ref value cannot be empty")
		}

		if err := readClosingBrace(); err != nil {
			return nil, err
		}

		return &RefExpr{Ref: refValue}, nil
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
