// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package ast

import (
	"bytes"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/ebnf"
)

var (
	seclLexer = lexer.Must(ebnf.New(`
Ident = (alpha | "_") { "_" | alpha | digit | "." | "[" | "]" } .
String = "\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Int = [ "-" | "+" ] digit { digit } .
Punct = "!"…"/" | ":"…"@" | "["…` + "\"`\"" + ` | "{"…"~" .
Whitespace = ( " " | "\t" | "\n" ) { " " | "\t" | "\n" } .
alpha = "a"…"z" | "A"…"Z" .
digit = "0"…"9" .
any = "\u0000"…"\uffff" .
`))
)

// ParseRule parses a SECL rule.
func ParseRule(expr string) (*Rule, error) {
	parser, err := participle.Build(&Rule{},
		participle.Lexer(seclLexer),
		participle.Elide("Whitespace"),
		participle.Unquote("String"))
	if err != nil {
		return nil, err
	}

	rule := &Rule{}

	err = parser.Parse(bytes.NewBufferString(expr), rule)
	if err != nil {
		return nil, err
	}
	rule.Expr = expr

	return rule, nil
}

// Rule describes a SECL rule
type Rule struct {
	Pos  lexer.Position
	Expr string

	BooleanExpression *BooleanExpression `parser:"@@"`
}

// ParseMacro parses a SECL macro
func ParseMacro(expr string) (*Macro, error) {
	parser, err := participle.Build(&Macro{},
		participle.Lexer(seclLexer),
		participle.Elide("Whitespace"),
		participle.Unquote("String"))
	if err != nil {
		return nil, err
	}

	macro := &Macro{}

	err = parser.Parse(bytes.NewBufferString(expr), macro)
	if err != nil {
		return nil, err
	}

	return macro, nil
}

// Macro describes a SECL macro
type Macro struct {
	Pos lexer.Position

	Expression *Expression `parser:"@@"`
	Array      *Array      `parser:"| @@"`
	Primary    *Primary    `parser:"| @@"`
}

// BooleanExpression describes a boolean expression
type BooleanExpression struct {
	Pos lexer.Position

	Expression *Expression `parser:"@@"`
}

// Expression describes an expression
type Expression struct {
	Pos lexer.Position

	Comparison *Comparison        `parser:"@@"`
	Op         *string            `parser:"[ @( \"|\" \"|\" | \"&\" \"&\" )"`
	Next       *BooleanExpression `parser:"@@ ]"`
}

// Comparison describes a comparison
type Comparison struct {
	Pos lexer.Position

	BitOperation     *BitOperation     `parser:"@@"`
	ScalarComparison *ScalarComparison `parser:"[ @@"`
	ArrayComparison  *ArrayComparison  `parser:"| @@ ]"`
}

// ScalarComparison describes a scalar comparison : the operator with the right operand
type ScalarComparison struct {
	Pos lexer.Position

	Op   *string     `parser:"@( \">\" | \">\" \"=\" | \"<\" | \"<\" \"=\" | \"!\" \"=\" | \"=\" \"=\" | \"=\" \"~\" | \"!\" \"~\" )"`
	Next *Comparison `parser:"@@"`
}

// ArrayComparison describes an operation that tests membership in an array
type ArrayComparison struct {
	Pos lexer.Position

	Op    *string `parser:"( @( \"in\" | \"not\" \"in\" )"`
	Array *Array  `parser:"@@ )"`
}

// BitOperation describes an operation on bits
type BitOperation struct {
	Pos lexer.Position

	Unary *Unary        `parser:"@@"`
	Op    *string       `parser:"[ @( \"&\" | \"|\" | \"^\" )"`
	Next  *BitOperation `parser:"@@ ]"`
}

// Unary describes an unary operation like logical not, binary not, minus
type Unary struct {
	Pos lexer.Position

	Op      *string  `parser:"( @( \"!\" | \"-\" | \"^\" )"`
	Unary   *Unary   `parser:"@@ )"`
	Primary *Primary `parser:"| @@"`
}

// Primary describes a single operand. It can be a simple identifier, a number,
// a string or a full expression in parenthesis
type Primary struct {
	Pos lexer.Position

	Ident         *string     `parser:"@Ident"`
	Number        *int        `parser:"| @Int"`
	String        *string     `parser:"| @String"`
	SubExpression *Expression `parser:"| \"(\" @@ \")\""`
}

// Array describes an array of values
type Array struct {
	Pos lexer.Position

	Strings []string `parser:"\"[\" @String { \",\" @String } \"]\""`
	Numbers []int    `parser:"| \"[\" @Int { \",\" @Int } \"]\""`
	Ident   *string  `parser:"| @Ident"`
}
