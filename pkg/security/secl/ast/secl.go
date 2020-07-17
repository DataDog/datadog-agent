package ast

import (
	"bytes"

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

	BooleanExpression *BooleanExpression `parser:"@@"`
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

	Expression *Expression `parser:"@@"`
	Array      *Array      `parser:"| @@"`
	Primary    *Primary    `parser:"| @@"`
}

type BooleanExpression struct {
	Pos lexer.Position

	Expression *Expression `parser:"@@"`
}

type Expression struct {
	Pos lexer.Position

	Comparison *Comparison        `parser:"@@"`
	Op         *string            `parser:"[ @( \"|\" \"|\" | \"&\" \"&\" )"`
	Next       *BooleanExpression `parser:"@@ ]"`
}

type Comparison struct {
	Pos lexer.Position

	BitOperation     *BitOperation     `parser:"@@"`
	ScalarComparison *ScalarComparison `parser:"[ @@"`
	ArrayComparison  *ArrayComparison  `parser:"| @@ ]"`
}

type ScalarComparison struct {
	Pos lexer.Position

	Op   *string     `parser:"@( \">\" | \">\" \"=\" | \"<\" | \"<\" \"=\" | \"!\" \"=\" | \"=\" \"=\" | \"=\" \"~\" | \"!\" \"~\" )"`
	Next *Comparison `parser:"@@"`
}

type ArrayComparison struct {
	Pos lexer.Position

	Op    *string `parser:"( @( \"in\" | \"not\" \"in\" )"`
	Array *Array  `parser:"@@ )"`
}

type BitOperation struct {
	Pos lexer.Position

	Unary *Unary        `parser:"@@"`
	Op    *string       `parser:"[ @( \"&\" | \"|\" | \"^\" )"`
	Next  *BitOperation `parser:"@@ ]"`
}

type Unary struct {
	Pos lexer.Position

	Op      *string  `parser:"( @( \"!\" | \"-\" | \"^\" )"`
	Unary   *Unary   `parser:"@@ )"`
	Primary *Primary `parser:"| @@"`
}

type Primary struct {
	Pos lexer.Position

	Ident         *string     `parser:"@Ident"`
	Number        *int        `parser:"| @Int"`
	String        *string     `parser:"| @String"`
	SubExpression *Expression `parser:"| \"(\" @@ \")\""`
}

type Array struct {
	Pos lexer.Position

	Strings []string `parser:"\"[\" @String { \",\" @String } \"]\""`
	Numbers []int    `parser:"| \"[\" @Int { \",\" @Int } \"]\""`
	Ident   *string  `parser:"| @Ident"`
}
