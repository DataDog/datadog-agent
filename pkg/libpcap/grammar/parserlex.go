// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grammar

import "github.com/DataDog/datadog-agent/pkg/libpcap/codegen"

// parserLex adapts the Scanner to goyacc's yyLexer interface.
// It bridges the hand-written scanner to the generated parser.
type parserLex struct {
	scanner *Scanner
	cs      *codegen.CompilerState
	err     string
}

// Lex is called by the goyacc-generated parser to get the next token.
func (pl *parserLex) Lex(lval *yySymType) int {
	return pl.scanner.Lex(lval)
}

// Error is called by the parser on syntax errors.
func (pl *parserLex) Error(msg string) {
	pl.err = msg
	pl.cs.SetError(&parseError{msg: msg})
}

// parseError wraps a parser error message.
type parseError struct {
	msg string
}

func (e *parseError) Error() string {
	return "can't parse filter expression: " + e.msg
}

// Parse parses a BPF filter expression and populates the compiler state's CFG.
// Returns an error if parsing fails.
func Parse(cs *codegen.CompilerState, input string) error {
	scanner := NewScanner(input)
	pl := &parserLex{
		scanner: scanner,
		cs:      cs,
	}

	yyParse(pl)

	if pl.err != "" {
		return &parseError{msg: pl.err}
	}
	return cs.Err
}
