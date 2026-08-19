// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language, building
// its abstract syntax tree straight from the source expression.
package parser

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// Kind is the kind of a token.
type Kind int

// Token kinds.
const (
	// EOF is the end of the expression. The scanner returns it forever once
	// the source is consumed.
	EOF Kind = iota
	// Illegal is a token the scanner couldn't make sense of. The scanner
	// reports an error for it and moves on, so that the parser sees the rest
	// of the expression.
	Illegal

	// literals, Text holds them as written, quotes and delimiters included

	Ident    // process.file.name, process.ancestors[A].file.name
	Int      // 123
	Duration // 10s
	String   // "/etc/passwd"
	Pattern  // ~"/etc/*"
	Regexp   // r"/etc/.*"
	IP       // 127.0.0.1, ::1
	CIDR     // 192.168.0.0/24
	Variable // ${my_var}
	FieldRef // %{process.pid}

	// keywords, spelled as identifiers

	Not   // not, both the negation and the start of `not in`
	In    // in
	AllIn // allin

	// operators and punctuation

	AndAnd   // &&, also spelled `and`
	OrOr     // ||, also spelled `or`
	Bang     // !
	Eq       // ==
	NotEq    // !=
	Lt       // <
	Le       // <=
	Gt       // >
	Ge       // >=
	Match    // =~
	NotMatch // !~
	Amp      // &
	Pipe     // |
	Caret    // ^
	Plus     // +
	Minus    // -
	LParen   // (
	RParen   // )
	LBracket // [
	RBracket // ]
	Comma    // ,
)

var kindNames = [...]string{
	EOF:      "end of expression",
	Illegal:  "illegal token",
	Ident:    "identifier",
	Int:      "integer",
	Duration: "duration",
	String:   "string",
	Pattern:  "pattern",
	Regexp:   "regular expression",
	IP:       "IP",
	CIDR:     "CIDR",
	Variable: "variable",
	FieldRef: "field reference",
	Not:      "`not`",
	In:       "`in`",
	AllIn:    "`allin`",
	AndAnd:   "`&&`",
	OrOr:     "`||`",
	Bang:     "`!`",
	Eq:       "`==`",
	NotEq:    "`!=`",
	Lt:       "`<`",
	Le:       "`<=`",
	Gt:       "`>`",
	Ge:       "`>=`",
	Match:    "`=~`",
	NotMatch: "`!~`",
	Amp:      "`&`",
	Pipe:     "`|`",
	Caret:    "`^`",
	Plus:     "`+`",
	Minus:    "`-`",
	LParen:   "`(`",
	RParen:   "`)`",
	LBracket: "`[`",
	RBracket: "`]`",
	Comma:    "`,`",
}

func (k Kind) String() string {
	if k >= 0 && int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// keywords maps the keywords to their kind. An operator has a single kind
// whatever its spelling, the text of its token telling which one was written.
var keywords = map[string]Kind{
	"and":   AndAnd,
	"or":    OrOr,
	"not":   Not,
	"in":    In,
	"allin": AllIn,
}

// Token is a token of a SECL expression.
type Token struct {
	Kind Kind
	// Text is the token as written in the source.
	Text string
	// Pos is the position of the first character of the token.
	Pos ast.Position
}

// End returns the offset right after the last character of the token.
func (t Token) End() int {
	return t.Pos.Offset + len(t.Text)
}

// describe renders the token for an error message, as written: "string
// `"/etc/passwd"`", "`and`".
func (t Token) describe() string {
	switch t.Kind {
	case EOF:
		return t.Kind.String()
	case Ident, Int, Duration, String, Pattern, Regexp, IP, CIDR, Variable, FieldRef, Illegal:
		return fmt.Sprintf("%s `%s`", t.Kind, t.Text)
	}
	return "`" + t.Text + "`"
}
