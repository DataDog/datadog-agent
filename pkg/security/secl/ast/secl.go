package ast

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/ebnf"
)

var (
	seclLexer = lexer.Must(ebnf.New(`
Ident = (alpha | "_") { "_" | alpha | digit | "." } .
String = "\"" { "\u0000"…"\uffff"-"\""-"\\" | "\\" any } "\"" .
Int = [ "-" | "+" ] digit { digit } .
Punct = "!"…"/" | ":"…"@" | "["…` + "\"`\"" + ` | "{"…"~" .
Whitespace = ( " " | "\t" ) { " " | "\t" } .
alpha = "a"…"z" | "A"…"Z" .
digit = "0"…"9" .
any = "\u0000"…"\uffff" .
`))
)

// Parse a SECL rule.
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

type Rule struct {
	Pos  lexer.Position
	Expr string

	BooleanExpression *BooleanExpression `@@`
}

func (r *Rule) ExprAt(pos lexer.Position) string {
	str := fmt.Sprintf("\n%s\n", r.Expr)
	str += strings.Repeat(" ", pos.Column-1)
	str += "^"
	return str
}

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

type Macro struct {
	Pos lexer.Position

	Expression *Expression `@@`
	Array      *Array      `| @@`
	Primary    *Primary    `| @@`
}

type BooleanExpression struct {
	Pos lexer.Position

	Expression *Expression `@@`
}

type Expression struct {
	Pos lexer.Position

	Comparison *Comparison        `@@`
	Op         *string            `[ @( "|" "|" | "&" "&" )`
	Next       *BooleanExpression `  @@ ]`
}

type Comparison struct {
	Pos lexer.Position

	BitOperation     *BitOperation     `@@`
	ScalarComparison *ScalarComparison `[ @@`
	ArrayComparison  *ArrayComparison  `| @@ ]`
}

type ScalarComparison struct {
	Pos lexer.Position

	Op   *string     `@( ">" | ">" "=" | "<" | "<" "=" | "!" "=" | "=" "=" | "=" "~" | "!" "~" )`
	Next *Comparison `  @@`
}

type ArrayComparison struct {
	Pos lexer.Position

	Op    *string ` ( @( "in" | "not" "in" )`
	Array *Array  `@@ )`
}

type BitOperation struct {
	Pos lexer.Position

	Unary *Unary        `@@`
	Op    *string       `[ @( "&" | "|" | "^" )`
	Next  *BitOperation `  @@ ]`
}

type Unary struct {
	Pos lexer.Position

	Op      *string  `  ( @( "!" | "-" | "^" )`
	Unary   *Unary   `    @@ )`
	Primary *Primary `| @@`
}

type Primary struct {
	Pos lexer.Position

	Ident         *string     `@Ident`
	Number        *int        `| @Int`
	String        *string     `| @String`
	SubExpression *Expression `| "(" @@ ")"`
}

type Array struct {
	Pos lexer.Position

	Strings []string `"[" @String { "," @String } "]"`
	Numbers []int    `| "[" @Int { "," @Int } "]"`
	Ident   *string  `| @Ident`
}
