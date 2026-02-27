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

// ParsingContext holds the parsers and optional rule cache; it has no parent (root factory for Rule, Macro, Expression).
type ParsingContext struct {
	ruleParser       *participle.Parser
	macroParser      *participle.Parser
	expressionParser *participle.Parser

	ruleCache map[string]*Rule
}

// NewParsingContext returns a new parsing context
func NewParsingContext(withRuleCache bool) *ParsingContext {
	seclLexer := lexer.Must(ebnf.New(`
Comment = ("#" | "//") { "\u0000"…"\uffff"-"\n" } .
CIDR = IP "/" digit { digit } .
IP = (ipv4 | ipv6) .
Variable = "${" (alpha | "_") { "_" | alpha | digit | "." } "}" .
FieldReference = "%{" (alpha | "_") { "_" | alpha | digit | "." | "[" | "]" } "}" .
Duration = digit { digit } ("m" | "s" | "m" | "h") { "s" } .
Regexp = "r\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Ident = (alpha | "_") { "_" | alpha | digit | "." | "[" | "]" } .
String = "\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Pattern = "~\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Int = [ "-" | "+" ] digit { digit } .
Punct = ( "!" | "=" | "<" | ">" | "+" | "-" | "[" | "]" | "(" | ")" | "," | "&" | "|" | "~" | "^" | "%" ).
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
		ruleParser:       buildParser(&Rule{}, seclLexer),
		macroParser:      buildParser(&Macro{}, seclLexer),
		expressionParser: buildParser(&Expression{}, seclLexer),
		ruleCache:        ruleCache,
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

// Rule is the root of a SECL rule AST; it has no parent and contains a BooleanExpression.
//
// Grammar:
//
//	Rule → BooleanExpression
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

// ParseExpression parses a SECL expression
func (pc *ParsingContext) ParseExpression(expr string) (*Expression, error) {
	expression := &Expression{}
	err := pc.expressionParser.Parse(bytes.NewBufferString(expr), expression)
	if err != nil {
		return nil, err
	}

	return expression, nil
}

// Macro is the root of a SECL macro body; it has no parent and contains an Expression, an Array, or a Primary.
//
// Grammar:
//
//	Macro → Expression
//	Macro → Array
//	Macro → Primary
type Macro struct {
	Pos lexer.Position

	Expression *Expression `parser:"@@"`
	Array      *Array      `parser:"| @@"`
	Primary    *Primary    `parser:"| @@"`
}

// BooleanExpression is contained only by Rule; it holds the root Expression of the rule.
//
// Grammar:
//
//	BooleanExpression → Expression
type BooleanExpression struct {
	Pos lexer.Position

	Expression *Expression `parser:"@@"`
}

// Expression is contained by BooleanExpression or by another Expression (via Next in a logical chain).
//
// Grammar:
//
//	Expression → Comparison
//	Expression → Comparison LogicalOp BooleanExpression
//
//	LogicalOp → "||"
//	LogicalOp → "or"
//	LogicalOp → "&&"
//	LogicalOp → "and"
type Expression struct {
	Pos lexer.Position

	Comparison *Comparison        `parser:"@@"`
	Op         *string            `parser:"[ @( \"|\" \"|\" | \"or\" | \"&\" \"&\" | \"and\" )"`
	Next       *BooleanExpression `parser:"@@ ]"`
}

// Comparison is contained only by Expression; it holds the left ArithmeticOperation and optional ScalarComparison or ArrayComparison.
//
// Grammar:
//
//	Comparison → ArithmeticOperation
//	Comparison → ArithmeticOperation ScalarComparison
//	Comparison → ArithmeticOperation ArrayComparison
type Comparison struct {
	Pos lexer.Position

	ArithmeticOperation *ArithmeticOperation `parser:"@@"`
	ScalarComparison    *ScalarComparison    `parser:"[ @@"`
	ArrayComparison     *ArrayComparison     `parser:"| @@ ]"`
}

// ScalarComparison is contained only by Comparison; it holds a scalar operator and the right-hand Comparison.
//
// Grammar:
//
//	ScalarComparison → ScalarOp Comparison
//
//	ScalarOp → ">="
//	ScalarOp → ">"
//	ScalarOp → "<="
//	ScalarOp → "<"
//	ScalarOp → "!="
//	ScalarOp → "=="
//	ScalarOp → "=~"
//	ScalarOp → "!~"
type ScalarComparison struct {
	Pos lexer.Position

	Op   *string     `parser:"@( \">\" \"=\" | \">\" | \"<\" \"=\" | \"<\" | \"!\" \"=\" | \"=\" \"=\" | \"=\" \"~\" | \"!\" \"~\" )"`
	Next *Comparison `parser:"@@"`
}

// ArrayComparison is contained only by Comparison; it holds the array operator and the right-hand Array.
//
// Grammar:
//
//	ArrayComparison → ArrayOp Array
//
//	ArrayOp → "in"
//	ArrayOp → "notin"
//	ArrayOp → "allin"
type ArrayComparison struct {
	Pos lexer.Position

	Op    *string `parser:"@( \"in\" | \"not\" \"in\" | \"allin\" )"`
	Array *Array  `parser:"@@"`
}

// BitOperation is contained by ArithmeticOperation (as First or as Operand inside Rest) or by another BitOperation (via Next).
//
// Grammar:
//
//	BitOperation → Unary
//	BitOperation → Unary BitOp BitOperation
//
//	BitOp → "&"
//	BitOp → "|"
//	BitOp → "^"
type BitOperation struct {
	Pos lexer.Position

	Unary *Unary        `parser:"@@"`
	Op    *string       `parser:"[ @( \"&\" | \"|\" | \"^\" )"`
	Next  *BitOperation `parser:"@@ ]"`
}

// ArithmeticOperation is contained only by Comparison; it holds the first BitOperation and an optional list of ArithmeticElements.
//
// Grammar:
//
//	ArithmeticOperation → BitOperation
//	ArithmeticOperation → BitOperation ArithmeticElementList
//
//	ArithmeticElementList → ArithmeticElement
//	ArithmeticElementList → ArithmeticElement ArithmeticElementList
type ArithmeticOperation struct {
	Pos lexer.Position

	First *BitOperation        `parser:"@@"`
	Rest  []*ArithmeticElement `parser:"[ @@ { @@ } ]"`
}

// ArithmeticElement defines one element of ArithmeticElementList.
//
// Grammar:
//
//	ArithmeticElement → ArithOp BitOperation
//
//	ArithOp → "+"
//	ArithOp → "-"
type ArithmeticElement struct {
	Op      string        `parser:"@( \"+\" | \"-\" )"`
	Operand *BitOperation `parser:"@@"`
}

// Unary is contained by BitOperation or by UnaryWithOp (or recursively by Unary); it holds a Primary or a UnaryWithOp.
//
// Grammar:
//
//	Unary → Primary
//	Unary → UnaryOp Unary
type Unary struct {
	Pos lexer.Position

	UnaryWithOp *UnaryWithOp `parser:"@@"`
	Primary     *Primary     `parser:"| @@"`
}

// UnaryWithOp is contained only by Unary; it holds a unary operator and the operand Unary.
//
// Grammar:
//
//	UnaryWithOp → UnaryOp Unary
//
//	UnaryOp → "!"
//	UnaryOp → "not"
//	UnaryOp → "-"
//	UnaryOp → "^"
type UnaryWithOp struct {
	Pos lexer.Position

	Op    *string `parser:"@( \"!\" | \"not\" | \"-\" | \"^\" )"`
	Unary *Unary  `parser:"@@"`
}

// Primary is contained by Unary or by Macro; it holds a single operand.
//
// Grammar:
//
//	Primary → Ident
//	Primary → CIDR
//	Primary → IP
//	Primary → Number
//	Primary → Variable
//	Primary → String
//	Primary → Pattern
//	Primary → Regexp
//	Primary → Duration
//	Primary → FieldReference
//	Primary → "(" Expression ")"
type Primary struct {
	Pos lexer.Position

	Ident          *string     `parser:"@Ident"`
	CIDR           *string     `parser:"| @CIDR"`
	IP             *string     `parser:"| @IP"`
	Number         *int        `parser:"| @Int"`
	Variable       *string     `parser:"| @Variable"`
	FieldReference *string     `parser:"| @FieldReference"`
	String         *string     `parser:"| @String"`
	Pattern        *string     `parser:"| @Pattern"`
	Regexp         *string     `parser:"| @Regexp"`
	Duration       *int        `parser:"| @Duration"`
	SubExpression  *Expression `parser:"| \"(\" @@ \")\""`
}

// StringMember defines one element of StringMemberList.
//
// Grammar:
//
//	StringMember → String
//	StringMember → Pattern
//	StringMember → Regexp
type StringMember struct {
	Pos lexer.Position

	String  *string `parser:"@String"`
	Pattern *string `parser:"| @Pattern"`
	Regexp  *string `parser:"| @Regexp"`
}

// CIDRMember defines one element of CIDRMemberList.
//
// Grammar:
//
//	CIDRMember → IP
//	CIDRMember → CIDR
type CIDRMember struct {
	Pos lexer.Position

	IP   *string `parser:"@IP"`
	CIDR *string `parser:"| @CIDR"`
}

// Array is contained only by ArrayComparison or by Macro; it holds the right-hand side of an array membership test.
//
// Grammar:
//
//	Array → CIDR
//	Array → Variable
//	Array → Ident
//	Array → FieldReference
//	Array → "[" StringMemberList "]"
//	Array → "[" CIDRMemberList "]"
//	Array → "[" NumberList "]"
//	Array → "[" IdentList "]"
//
//	StringMemberList → StringMember
//	StringMemberList → StringMember "," StringMemberList
//
//	CIDRMemberList → CIDRMember
//	CIDRMemberList → CIDRMember "," CIDRMemberList
//
//	NumberList → Number
//	NumberList → Number "," NumberList
//
//	IdentList → Ident
//	IdentList → Ident "," IdentList
type Array struct {
	Pos lexer.Position

	CIDR           *string        `parser:"@CIDR"`
	Variable       *string        `parser:"| @Variable"`
	FieldReference *string        `parser:"| @FieldReference"`
	Ident          *string        `parser:"| @Ident"`
	StringMembers  []StringMember `parser:"| \"[\" @@ { \",\" @@ } \"]\""`
	CIDRMembers    []CIDRMember   `parser:"| \"[\" @@ { \",\" @@ } \"]\""`
	Numbers        []int          `parser:"| \"[\" @Int { \",\" @Int } \"]\""`
	Idents         []string       `parser:"| \"[\" @Ident { \",\" @Ident } \"]\""`
}
