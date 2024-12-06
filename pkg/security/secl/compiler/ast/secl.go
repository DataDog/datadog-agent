// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds ast related files
package ast

import (
	"bytes"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/ebnf"
)

// ParsingContext defines a parsing context
type ParsingContext struct {
	ruleParser  *participle.Parser
	macroParser *participle.Parser

	ruleCache map[string]*Rule
}

// NewParsingContext returns a new parsing context
func NewParsingContext(withRuleCache bool) *ParsingContext {
	seclLexer := lexer.Must(ebnf.New(`
Comment = ("#" | "//") { "\u0000"…"\uffff"-"\n" } .
CIDR = IP "/" digit { digit } .
IP = (ipv4 | ipv6) .
Variable = "${" (alpha | "_") { "_" | alpha | digit | "." } "}" .
Duration = digit { digit } ("m" | "s" | "m" | "h") { "s" } .
Regexp = "r\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Ident = (alpha | "_") { "_" | alpha | digit | "." | "[" | "]" } .
String = "\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Pattern = "~\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Int = [ "-" | "+" ] digit { digit } .
Punct = "!"…"/" | ":"…"@" | "["…` + "\"`\"" + ` | "{"…"~" .
Whitespace = ( " " | "\t" | "\n" ) { " " | "\t" | "\n" } .
ipv4 = (digit { digit } "." digit { digit } "." digit { digit } "." digit { digit }) .
ipv6 = ( [hex { hex }] ":" [hex { hex }] ":" [hex { hex }] [":" | "."] [hex { hex }] [":" | "."] [hex { hex }] [":" | "."] [hex { hex }] [":" | "."] [hex { hex }] [":" | "."] [hex { hex }]) .
hex = "a"…"f" | "A"…"F" | "0"…"9" .
alpha = "a"…"z" | "A"…"Z" .
digit = "0"…"9" .
any = "\u0000"…"\uffff" .
`))

	var ruleCache map[string]*Rule
	if withRuleCache {
		ruleCache = make(map[string]*Rule)
	}

	return &ParsingContext{
		ruleParser:  buildParser(&Rule{}, seclLexer),
		macroParser: buildParser(&Macro{}, seclLexer),
		ruleCache:   ruleCache,
	}
}

func buildParser(obj interface{}, lexer lexer.Definition) *participle.Parser {
	parser, err := participle.Build(obj,
		participle.Lexer(lexer),
		participle.Elide("Whitespace", "Comment"),
		participle.Map(unquoteLiteral, "String"),
		participle.Map(parseDuration, "Duration"),
		participle.Map(unquotePattern, "Pattern", "Regexp"),
	)
	if err != nil {
		panic(err)
	}
	return parser
}

func unquoteLiteral(t lexer.Token) (lexer.Token, error) {
	unquoted := strings.TrimSpace(t.Value)
	unquoted = unquoted[1 : len(unquoted)-1]
	t.Value = unquoted
	return t, nil
}

func unquotePattern(t lexer.Token) (lexer.Token, error) {
	unquoted := strings.TrimSpace(t.Value[1:])
	unquoted = unquoted[1 : len(unquoted)-1]
	t.Value = unquoted
	return t, nil
}

func parseDuration(t lexer.Token) (lexer.Token, error) {
	duration, err := time.ParseDuration(t.Value)
	if err != nil {
		return t, participle.Errorf(t.Pos, "invalid duration string %q: %s", t.Value, err)
	}

	t.Value = strconv.Itoa(int(duration.Nanoseconds()))

	return t, nil
}

// ParseRule parses a SECL rule.
func (pc *ParsingContext) ParseRule(expr string) (*Rule, error) {
	if pc.ruleCache != nil {
		if ast, ok := pc.ruleCache[expr]; ok {
			return ast, nil
		}
	}

	rule := &Rule{}
	err := pc.ruleParser.Parse(bytes.NewBufferString(expr), rule)
	if err != nil {
		return nil, err
	}
	rule.Expr = expr

	if pc.ruleCache != nil {
		pc.ruleCache[expr] = rule
	}

	return rule, nil
}

// Rule describes a SECL rule
type Rule struct {
	Pos  lexer.Position
	Expr string

	BooleanExpression *BooleanExpression `parser:"@@"`
}

// ParseMacro parses a SECL macro
func (pc *ParsingContext) ParseMacro(expr string) (*Macro, error) {
	macro := &Macro{}
	err := pc.macroParser.Parse(bytes.NewBufferString(expr), macro)
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
	Op         *string            `parser:"[ @( \"|\" \"|\" | \"or\" | \"&\" \"&\" | \"and\" )"`
	Next       *BooleanExpression `parser:"@@ ]"`
}

// Comparison describes a comparison
type Comparison struct {
	Pos lexer.Position

	ArithmeticOperation *ArithmeticOperation `parser:"@@"`
	ScalarComparison    *ScalarComparison    `parser:"[ @@"`
	ArrayComparison     *ArrayComparison     `parser:"| @@ ]"`
}

// ScalarComparison describes a scalar comparison : the operator with the right operand
type ScalarComparison struct {
	Pos lexer.Position

	Op   *string     `parser:"@( \">\" \"=\" | \">\" | \"<\" \"=\" | \"<\" | \"!\" \"=\" | \"=\" \"=\" | \"=\" \"~\" | \"!\" \"~\" )"`
	Next *Comparison `parser:"@@"`
}

// ArrayComparison describes an operation that tests membership in an array
type ArrayComparison struct {
	Pos lexer.Position

	Op    *string `parser:"( @( \"in\" | \"not\" \"in\" | \"allin\" )"`
	Array *Array  `parser:"@@ )"`
}

// BitOperation describes an operation on bits
type BitOperation struct {
	Pos lexer.Position

	Unary *Unary        `parser:"@@"`
	Op    *string       `parser:"[ @( \"&\" | \"|\" | \"^\" )"`
	Next  *BitOperation `parser:"@@ ]"`
}

// ArithmeticOperation describes an arithmetic operation
type ArithmeticOperation struct {
	Pos lexer.Position

	First *BitOperation        `parser:"@@"`
	Rest  []*ArithmeticElement `parser:"[ @@ { @@ } ]"`
}

// ArithmeticElement defines an arithmetic element
type ArithmeticElement struct {
	Op      string        `parser:"@( \"+\" | \"-\" )"`
	Operand *BitOperation `parser:"@@"`
}

// Unary describes an unary operation like logical not, binary not, minus
type Unary struct {
	Pos lexer.Position

	Op      *string  `parser:"( @( \"!\" | \"not\" | \"-\" | \"^\" )"`
	Unary   *Unary   `parser:"@@ )"`
	Primary *Primary `parser:"| @@"`
}

// Primary describes a single operand. It can be a simple identifier, a number,
// a string or a full expression in parenthesis
type Primary struct {
	Pos lexer.Position

	Ident         *string     `parser:"@Ident"`
	CIDR          *string     `parser:"| @CIDR"`
	IP            *string     `parser:"| @IP"`
	Number        *int        `parser:"| @Int"`
	Variable      *string     `parser:"| @Variable"`
	String        *string     `parser:"| @String"`
	Pattern       *string     `parser:"| @Pattern"`
	Regexp        *string     `parser:"| @Regexp"`
	Duration      *int        `parser:"| @Duration"`
	SubExpression *Expression `parser:"| \"(\" @@ \")\""`
}

// StringMember describes a String based array member
type StringMember struct {
	Pos lexer.Position

	String  *string `parser:"@String"`
	Pattern *string `parser:"| @Pattern"`
	Regexp  *string `parser:"| @Regexp"`
}

// CIDRMember describes a CIDR based array member
type CIDRMember struct {
	Pos lexer.Position

	IP   *string `parser:"@IP"`
	CIDR *string `parser:"| @CIDR"`
}

// Array describes an array of values
type Array struct {
	Pos lexer.Position

	CIDR          *string        `parser:"@CIDR"`
	Variable      *string        `parser:"| @Variable"`
	Ident         *string        `parser:"| @Ident"`
	StringMembers []StringMember `parser:"| \"[\" @@ { \",\" @@ } \"]\""`
	CIDRMembers   []CIDRMember   `parser:"| \"[\" @@ { \",\" @@ } \"]\""`
	Numbers       []int          `parser:"| \"[\" @Int { \",\" @Int } \"]\""`
	Idents        []string       `parser:"| \"[\" @Ident { \",\" @Ident } \"]\""`
}
