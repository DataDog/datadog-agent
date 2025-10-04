// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package exprlang provides utilities for parsing and working with expressions
// in the expression language of the dynamic instrumentation and live debugger products.
//
// This package focuses solely on DSL parsing and validation, without any dependencies
// on IR program structure. The parsed expressions can then be validated against specific
// IR programs in higher-level packages.
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
	Instruction string
	Argument    any
}

func (ue *UnsupportedExpr) expr() {}

// ParseError represents an error that occurred during DSL parsing.
type ParseError struct {
	Message string
	Cause   error
}

func (e *ParseError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("parse error: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("parse error: %s", e.Message)
}

func (e *ParseError) Unwrap() error {
	return e.Cause
}

// decoderPool is a sync.Pool for jsontext.Decoder instances to optimize allocations.
var decoderPool = sync.Pool{
	New: func() any {
		return &jsontext.Decoder{}
	},
}

// Parse parses a DSL JSON expression into a strongly-typed AST node.
func Parse(dslJSON []byte) (Expr, error) {
	if len(dslJSON) == 0 {
		return nil, &ParseError{Message: "empty DSL expression"}
	}

	// Get a decoder from the pool and reset it
	dec := decoderPool.Get().(*jsontext.Decoder)
	defer decoderPool.Put(dec)
	dec.Reset(bytes.NewReader(dslJSON))

	// Ensure we have a JSON object
	objStart, err := dec.ReadToken()
	if err != nil {
		return nil, &ParseError{Message: "failed to read token", Cause: err}
	}
	if kind := objStart.Kind(); kind != '{' {
		return nil, &ParseError{Message: fmt.Sprintf("malformed DSL: got token %v (%v), expected {", objStart, kind)}
	}

	// Read the instruction key
	key, err := dec.ReadToken()
	if err != nil {
		return nil, &ParseError{Message: "failed to read instruction key", Cause: err}
	}
	if kind := key.Kind(); kind != '"' {
		return nil, &ParseError{Message: fmt.Sprintf("malformed DSL: got key token %v (%v), expected string", key, kind)}
	}

	instruction := key.String()
	switch instruction {
	case "ref":
		return parseRefExpr(dec)
	default:
		// Read the argument for unsupported instructions
		arg, err := dec.ReadToken()
		if err != nil {
			return nil, &ParseError{Message: "failed to read argument", Cause: err}
		}

		// Extract the argument value before reading more tokens
		var argument any
		switch arg.Kind() {
		case '"':
			argument = arg.String()
		case 'n':
			argument = nil
		case 't', 'f':
			argument = arg.Bool()
		case '0':
			// For numbers in unsupported expressions, store as string
			argument = arg.String()
		default:
			argument = arg.String()
		}

		// Read the closing brace
		endObj, err := dec.ReadToken()
		if err != nil {
			return nil, &ParseError{Message: "failed to read closing brace", Cause: err}
		}
		if kind := endObj.Kind(); kind != '}' {
			return nil, &ParseError{Message: fmt.Sprintf("malformed DSL: got token %v (%v), expected }", endObj, kind)}
		}

		return &UnsupportedExpr{
			Instruction: instruction,
			Argument:    argument,
		}, nil
	}
}

func parseRefExpr(dec *jsontext.Decoder) (*RefExpr, error) {
	// Read the value token
	val, err := dec.ReadToken()
	if err != nil {
		return nil, &ParseError{Message: "failed to read ref value", Cause: err}
	}
	if kind := val.Kind(); kind != '"' {
		return nil, &ParseError{Message: fmt.Sprintf("malformed ref: got value token %v (%v), expected string", val, kind)}
	}

	refValue := val.String()
	if refValue == "" {
		return nil, &ParseError{Message: "ref value cannot be empty"}
	}

	// Read the closing brace
	endObj, err := dec.ReadToken()
	if err != nil {
		return nil, &ParseError{Message: "failed to read closing brace", Cause: err}
	}
	if kind := endObj.Kind(); kind != '}' {
		return nil, &ParseError{Message: fmt.Sprintf("malformed DSL: got token %v (%v), expected }", endObj, kind)}
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

// CollectVariableReferences extracts all variable names referenced in an expression.
func CollectVariableReferences(expr Expr) []string {
	switch e := expr.(type) {
	case *RefExpr:
		return []string{e.Ref}
	case *UnsupportedExpr:
		// For unsupported expressions, try to extract variable names if possible
		if e.Instruction == "ref" {
			if refStr, ok := e.Argument.(string); ok && refStr != "" {
				return []string{refStr}
			}
		}
		return nil
	default:
		return nil
	}
}
