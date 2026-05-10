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
	"iter"
	"strconv"
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

// EqExpr represents an equality comparison expression.
type EqExpr struct {
	Left, Right Expr
}

func (ee *EqExpr) expr() {}

// NeExpr represents an inequality comparison expression (left != right).
type NeExpr struct {
	Left, Right Expr
}

func (ne *NeExpr) expr() {}

// LtExpr represents a less-than comparison expression (left < right).
type LtExpr struct {
	Left, Right Expr
}

func (lt *LtExpr) expr() {}

// LeExpr represents a less-than-or-equal comparison expression (left <= right).
type LeExpr struct {
	Left, Right Expr
}

func (le *LeExpr) expr() {}

// GtExpr represents a greater-than comparison expression (left > right).
type GtExpr struct {
	Left, Right Expr
}

func (gt *GtExpr) expr() {}

// GeExpr represents a greater-than-or-equal comparison expression (left >= right).
type GeExpr struct {
	Left, Right Expr
}

func (ge *GeExpr) expr() {}

// LiteralExpr represents a literal value (int64, float64, bool, or string).
type LiteralExpr struct {
	Value any // int64 | float64 | bool | string
}

func (le *LiteralExpr) expr() {}

// LenExpr represents a len() call on a collection (string, slice, map).
type LenExpr struct {
	Operand Expr
}

func (le *LenExpr) expr() {}

// IsEmptyExpr represents an isEmpty() call on a collection (string, slice, map).
type IsEmptyExpr struct {
	Operand Expr
}

func (ie *IsEmptyExpr) expr() {}

// ContainsExpr represents a contains(map, key) call. It evaluates to true
// when the map contains the given key and to false otherwise (including
// when the map is nil).
type ContainsExpr struct {
	Base Expr // must resolve to a map
	Key  Expr // literal key (base type or string)
}

func (ce *ContainsExpr) expr() {}

// IndexExpr represents an index access expression (e.g., arr[0]).
type IndexExpr struct {
	Base  Expr
	Index Expr
}

func (ie *IndexExpr) expr() {}

// AndExpr represents a logical AND of exactly two sub-expressions.
// Deeper conjunctions are expressed as right-associated nesting
// (e.g., and(A, and(B, C))).
type AndExpr struct {
	Left, Right Expr
}

func (ae *AndExpr) expr() {}

// OrExpr represents a logical OR of exactly two sub-expressions.
// Deeper disjunctions are expressed as right-associated nesting.
type OrExpr struct {
	Left, Right Expr
}

func (oe *OrExpr) expr() {}

// NotExpr represents a logical NOT of a single sub-expression.
type NotExpr struct {
	Operand Expr
}

func (ne *NotExpr) expr() {}

// UnsupportedExpr represents an expression type that is not yet supported.
type UnsupportedExpr struct {
	Operation string
	Argument  jsontext.Value
}

func (ue *UnsupportedExpr) expr() {}

// Rewrite performs a bottom-up rewrite of an expression tree. For each node,
// children are rewritten first, then f is called. If f returns a non-nil
// replacement, that replacement is used; otherwise the (possibly rebuilt) node
// is kept. Nodes whose children are unchanged are not reallocated.
func Rewrite(root Expr, f func(Expr) Expr) Expr {
	var result Expr
	switch e := root.(type) {
	case *GetMemberExpr:
		newBase := Rewrite(e.Base, f)
		if newBase != e.Base {
			result = &GetMemberExpr{Base: newBase, Member: e.Member}
		} else {
			result = root
		}
	case *EqExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &EqExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *NeExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &NeExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *LtExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &LtExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *LeExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &LeExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *GtExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &GtExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *GeExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &GeExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *IndexExpr:
		newBase := Rewrite(e.Base, f)
		newIndex := Rewrite(e.Index, f)
		if newBase != e.Base || newIndex != e.Index {
			result = &IndexExpr{Base: newBase, Index: newIndex}
		} else {
			result = root
		}
	case *LenExpr:
		newOp := Rewrite(e.Operand, f)
		if newOp != e.Operand {
			result = &LenExpr{Operand: newOp}
		} else {
			result = root
		}
	case *IsEmptyExpr:
		newOp := Rewrite(e.Operand, f)
		if newOp != e.Operand {
			result = &IsEmptyExpr{Operand: newOp}
		} else {
			result = root
		}
	case *ContainsExpr:
		newBase := Rewrite(e.Base, f)
		newKey := Rewrite(e.Key, f)
		if newBase != e.Base || newKey != e.Key {
			result = &ContainsExpr{Base: newBase, Key: newKey}
		} else {
			result = root
		}
	case *AndExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &AndExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *OrExpr:
		newLeft := Rewrite(e.Left, f)
		newRight := Rewrite(e.Right, f)
		if newLeft != e.Left || newRight != e.Right {
			result = &OrExpr{Left: newLeft, Right: newRight}
		} else {
			result = root
		}
	case *NotExpr:
		newOp := Rewrite(e.Operand, f)
		if newOp != e.Operand {
			result = &NotExpr{Operand: newOp}
		} else {
			result = root
		}
	case *RefExpr, *LiteralExpr, *UnsupportedExpr:
		// Leaf types: no children to rewrite.
		result = root
	default:
		panic(fmt.Sprintf("exprlang.Rewrite: unhandled expression type %T", root))
	}
	if r := f(result); r != nil {
		return r
	}
	return result
}

// Children yields every expression in the tree rooted at root, in bottom-up
// order (children before their parent, including the root). Break in the
// consumer terminates the walk.
func Children(root Expr) iter.Seq[Expr] {
	return func(yield func(Expr) bool) {
		stopped := false
		Rewrite(root, func(e Expr) Expr {
			if !stopped && !yield(e) {
				stopped = true
			}
			return nil
		})
	}
}

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

// parseBinaryOperands reads a strictly two-element JSON array of operand
// expressions for a binary operator ("eq" / "ne" / "lt" / "le" / "gt" /
// "ge" / "and" / "or"). It returns the parsed LHS and RHS but does NOT
// consume the trailing closing brace of the enclosing object.
func parseBinaryOperands(operation string, dec *jsontext.Decoder) (Expr, Expr, error) {
	arrStart, err := dec.ReadToken()
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: failed to read %s array start: %w", operation, err)
	}
	if kind := arrStart.Kind(); kind != '[' {
		return nil, nil, fmt.Errorf("parse error: malformed %s: got token %v (%v), expected [", operation, arrStart, kind)
	}

	if dec.PeekKind() == ']' {
		return nil, nil, fmt.Errorf("parse error: malformed %s: expected exactly two operands, got 0", operation)
	}
	lhsJSON, err := dec.ReadValue()
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: failed to read %s LHS expression: %w", operation, err)
	}
	lhs, err := Parse(lhsJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: failed to parse %s LHS expression: %w", operation, err)
	}

	if dec.PeekKind() == ']' {
		return nil, nil, fmt.Errorf("parse error: malformed %s: expected exactly two operands, got 1", operation)
	}
	rhsJSON, err := dec.ReadValue()
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: failed to read %s RHS expression: %w", operation, err)
	}
	rhs, err := Parse(rhsJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: failed to parse %s RHS expression: %w", operation, err)
	}

	if dec.PeekKind() != ']' {
		return nil, nil, fmt.Errorf("parse error: malformed %s: expected exactly two operands, got 3 or more", operation)
	}
	arrEnd, err := dec.ReadToken()
	if err != nil {
		return nil, nil, fmt.Errorf("parse error: failed to read %s array end: %w", operation, err)
	}
	if kind := arrEnd.Kind(); kind != ']' {
		return nil, nil, fmt.Errorf("parse error: malformed %s: expected exactly two operands", operation)
	}
	return lhs, rhs, nil
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

	// Peek at the first token to handle literal values (non-object).
	objStart, err := dec.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("parse error: failed to read token: %w", err)
	}
	switch kind := objStart.Kind(); kind {
	case '"':
		return &LiteralExpr{Value: objStart.String()}, nil
	case '0':
		// Number: try int64 first, fallback to float64.
		numStr := objStart.String()
		if i, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			return &LiteralExpr{Value: i}, nil
		}
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse number %q: %w", numStr, err)
		}
		return &LiteralExpr{Value: f}, nil
	case 't':
		return &LiteralExpr{Value: true}, nil
	case 'f':
		return &LiteralExpr{Value: false}, nil
	case 'n':
		return &LiteralExpr{Value: nil}, nil
	case '{':
		// Continue with object parsing below.
	default:
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
	case "eq", "ne", "lt", "le", "gt", "ge":
		lhs, rhs, err := parseBinaryOperands(operation, dec)
		if err != nil {
			return nil, err
		}
		if err := readClosingBrace(); err != nil {
			return nil, err
		}
		switch operation {
		case "eq":
			return &EqExpr{Left: lhs, Right: rhs}, nil
		case "ne":
			return &NeExpr{Left: lhs, Right: rhs}, nil
		case "lt":
			return &LtExpr{Left: lhs, Right: rhs}, nil
		case "le":
			return &LeExpr{Left: lhs, Right: rhs}, nil
		case "gt":
			return &GtExpr{Left: lhs, Right: rhs}, nil
		default:
			return &GeExpr{Left: lhs, Right: rhs}, nil
		}

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
	case "index":
		// Index access: {"index": [<base_expr>, <index_expr>]}
		arrStart, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read index array start: %w", err)
		}
		if kind := arrStart.Kind(); kind != '[' {
			return nil, fmt.Errorf("parse error: malformed index: got token %v (%v), expected [", arrStart, kind)
		}

		// Read base expression.
		baseJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read index base expression: %w", err)
		}
		base, err := Parse(baseJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse index base expression: %w", err)
		}

		// Read index expression.
		indexJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read index expression: %w", err)
		}
		idx, err := Parse(indexJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse index expression: %w", err)
		}

		// Read array closing bracket.
		arrEnd, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read index array end: %w", err)
		}
		if kind := arrEnd.Kind(); kind != ']' {
			return nil, fmt.Errorf("parse error: malformed index: got token %v (%v), expected ]", arrEnd, kind)
		}

		if err := readClosingBrace(); err != nil {
			return nil, err
		}

		return &IndexExpr{Base: base, Index: idx}, nil

	case "contains":
		// Map key-presence check: {"contains": [<map_expr>, <key_expr>]}
		arrStart, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read contains array start: %w", err)
		}
		if kind := arrStart.Kind(); kind != '[' {
			return nil, fmt.Errorf("parse error: malformed contains: got token %v (%v), expected [", arrStart, kind)
		}

		if dec.PeekKind() == ']' {
			return nil, errors.New("parse error: malformed contains: expected exactly two operands, got 0")
		}
		baseJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read contains base expression: %w", err)
		}
		base, err := Parse(baseJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse contains base expression: %w", err)
		}

		if dec.PeekKind() == ']' {
			return nil, errors.New("parse error: malformed contains: expected exactly two operands, got 1")
		}
		keyJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read contains key expression: %w", err)
		}
		key, err := Parse(keyJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse contains key expression: %w", err)
		}

		arrEnd, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read contains array end: %w", err)
		}
		if kind := arrEnd.Kind(); kind != ']' {
			return nil, errors.New("parse error: malformed contains: expected exactly two operands")
		}

		if err := readClosingBrace(); err != nil {
			return nil, err
		}

		return &ContainsExpr{Base: base, Key: key}, nil

	case "len", "isEmpty":
		// Read the argument value and parse it recursively.
		argJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read %s argument: %w", operation, err)
		}
		arg, err := Parse(argJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse %s argument: %w", operation, err)
		}
		if err := readClosingBrace(); err != nil {
			return nil, err
		}
		if operation == "len" {
			return &LenExpr{Operand: arg}, nil
		}
		return &IsEmptyExpr{Operand: arg}, nil

	case "and", "or":
		// Binary boolean: {"and": [<lhs>, <rhs>]} or {"or": [<lhs>, <rhs>]}.
		lhs, rhs, err := parseBinaryOperands(operation, dec)
		if err != nil {
			return nil, err
		}
		if err := readClosingBrace(); err != nil {
			return nil, err
		}
		if operation == "and" {
			return &AndExpr{Left: lhs, Right: rhs}, nil
		}
		return &OrExpr{Left: lhs, Right: rhs}, nil

	case "not":
		argJSON, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to read not argument: %w", err)
		}
		arg, err := Parse(argJSON)
		if err != nil {
			return nil, fmt.Errorf("parse error: failed to parse not argument: %w", err)
		}
		if err := readClosingBrace(); err != nil {
			return nil, err
		}
		return &NotExpr{Operand: arg}, nil

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
