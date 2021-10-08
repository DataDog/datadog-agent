// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"errors"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/ebnf"
)

var (
	expressionLexer = lexer.Must(ebnf.New(`
		Hex = ("0" "x") hexdigit { hexdigit } .
		Ident = (alpha | "_") { "_" | "." | alpha | digit } .
		String = "\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
		UnixSystemPath = "/" alpha { alpha | digit | "-" | "." | "_" | "/" } ["*" [ "." { alpha | digit } ] ].
		Octal = "0" octaldigit { octaldigit } .
		Decimal = [ "-" | "+" ] digit { digit } .
		Punct = "!"…"/" | ":"…"@" | "["…` + "\"`\"" + ` | "{"…"~" .
		Whitespace = ( " " | "\t" ) { " " | "\t" } .
		alpha = "a"…"z" | "A"…"Z" .
		octaldigit = "0"…"7" .
		hexdigit = "A"…"F" | "a"…"f" | digit .
		digit = "0"…"9" .
		any = "\u0000"…"\uffff" .
	`))

	expressionOptions = []participle.Option{
		participle.Lexer(expressionLexer),
		participle.Unquote("String"),
		participle.UseLookahead(2),
		participle.Elide("Whitespace"),
	}

	expressionParser = participle.MustBuild(&Expression{}, expressionOptions...)

	iterableParser = participle.MustBuild(&IterableExpression{}, expressionOptions...)

	pathParser = participle.MustBuild(&PathExpression{}, expressionOptions...)

	// ErrEmptyExpression is returned for empty string used as expression input
	ErrEmptyExpression = errors.New("invalid empty expression")
)

// ParseExpression parses Expression from a string
func ParseExpression(s string) (*Expression, error) {
	if len(s) == 0 {
		return nil, ErrEmptyExpression
	}
	expr := &Expression{}
	err := expressionParser.ParseString(s, expr)
	if err != nil {
		return nil, err
	}
	return expr, nil
}

// ParseIterable parses IterableExpression from a string
func ParseIterable(s string) (*IterableExpression, error) {
	if len(s) == 0 {
		return nil, ErrEmptyExpression
	}
	expr := &IterableExpression{}
	err := iterableParser.ParseString(s, expr)
	if err != nil {
		return nil, err
	}
	return expr, nil
}

// ParsePath parses PathExpression from a string
func ParsePath(s string) (*PathExpression, error) {
	if len(s) == 0 {
		return nil, ErrEmptyExpression
	}
	expr := &PathExpression{}
	err := pathParser.ParseString(s, expr)
	if err != nil {
		return nil, err
	}
	return expr, nil
}
