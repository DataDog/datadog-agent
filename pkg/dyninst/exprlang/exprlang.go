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
	operation string
	argument  jsontext.Value
}

func (ue *UnsupportedExpr) expr() {}

// Parse parses a DSL JSON expression into a strongly-typed AST node.
func Parse(dslJSON []byte) (Expr, error) {
	if len(dslJSON) == 0 {
		return nil, fmt.Errorf("parse error: empty DSL expression")
	}
	dec := jsontext.NewDecoder(bytes.NewReader(dslJSON))
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
		return parseRefExpr(dec)
	default:
		// Read the argument for unsupported operations
		arg, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read argument: %w", err)
		}

		// Extract the argument value before reading more tokens
		var argument jsontext.Value
		switch arg.Kind() {
		case '"':
			argument = jsontext.Value(arg.String())
		case 'n':
			argument = jsontext.Value(nil)
		case 't', 'f':
			argument = jsontext.Value(arg.String())
		case '0':
			// For numbers in unsupported expressions, store as string
			argument = jsontext.Value(arg.String())
		default:
			argument = jsontext.Value(arg.String())
		}

		// Read the closing brace
		endObj, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read closing brace: %w", err)
		}
		if kind := endObj.Kind(); kind != '}' {
			return nil, fmt.Errorf("parse error: malformed DSL: got token %v (%v), expected }", endObj, kind)
		}

		return &UnsupportedExpr{
			operation: operation,
			argument:  argument,
		}, nil
	}
}

func parseRefExpr(dec *jsontext.Decoder) (*RefExpr, error) {
	// Read the value token
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

	// Read the closing brace
	endObj, err := dec.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("parse error: failed to read closing brace: %w", err)
	}
	if kind := endObj.Kind(); kind != '}' {
		return nil, fmt.Errorf("parse error: malformed DSL: got token %v (%v), expected }", endObj, kind)
	}

	return &RefExpr{Ref: refValue}, nil
}

// IsSupported returns true if the expression uses only supported DSL features.
func IsSupported(expr Expr) bool {
	switch expr.(type) {
	case *RefExpr:
		return true
	case *UnsupportedExpr:
		return false
	default:
		return false
	}
}
