// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language.
//
// It is a recursive descent parser, one function per precedence level, from
// the loosest to the tightest binding. The bitwise and arithmetic levels share
// parseOperation:
//
//	or              a || b, a or b
//	and             a && b, a and b
//	comparison      a == b, a != b, a < b, a <= b, a > b, a >= b, a =~ b, a !~ b,
//	                a in b, a not in b, a allin b
//	bitwise or      a | b
//	bitwise xor     a ^ b
//	bitwise and     a & b
//	additive        a + b, a - b
//	unary           !a, not a, -a, ^a
//	primary         literals, names, arrays, (a)
//
// Binary operators are left associative, `a - b - c` is `(a - b) - c`, but for
// comparisons that can't be chained: `a == b == c` is an error.
//
// The parser looks one token ahead, and never backtracks. A string is one
// token, the parser moving the scanner back into it for the parts of a string
// with interpolations. Syntax errors are returned as an ErrorList holding every
// error found.
package parser

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// ParseExpression parses a SECL expression.
func ParseExpression(expr string) (ast.Node, error) {
	_, root, err := parse("", expr)
	return root, err
}

// ParseRule parses the expression of a SECL rule.
func ParseRule(expr string) (*ast.Rule, error) {
	start, root, err := parse("", expr)
	if err != nil {
		return nil, err
	}
	return &ast.Rule{Meta: ast.At(start), Expr: expr, Root: root}, nil
}

// ParseMacro parses the expression of a SECL macro, tagging the positions of
// the tree with the ID of the macro.
func ParseMacro(id, expr string) (*ast.Macro, error) {
	start, root, err := parse(id, expr)
	if err != nil {
		return nil, err
	}
	return &ast.Macro{Meta: ast.At(start), Root: root}, nil
}

func parse(source, expr string) (start ast.Position, root ast.Node, err error) {
	p := &parser{}
	p.scanner = newScanner(source, expr, &p.errs)

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); !ok {
				panic(r)
			}
		}
		// the tree of an expression with errors is incomplete, nil where an
		// operand is missing
		if err = p.errs.err(); err != nil {
			root = nil
		}
	}()

	p.next()
	start = p.tok.Pos
	root = p.parseRoot()

	return
}

// A parsing function reports the errors it finds and keeps going, returning a
// nil node where it couldn't build one, without consuming the faulty token.
// parseAnd is where it recovers.
type parser struct {
	scanner *scanner
	errs    ErrorList

	tok   Token // lookahead
	depth int   // open parentheses
}

func (p *parser) next() {
	p.tok = p.scanner.next()
}

func (p *parser) parseRoot() ast.Node {
	if p.tok.Kind == EOF {
		p.errorAt(p.tok, "empty expression", "")
		return nil
	}

	// the recovery of the `and` level skips anything that isn't a logical
	// operator or the end of the expression, an unbalanced `)` included
	return p.parseOr()
}

// parseOr parses a disjunction.
//
//	Or → And { "||" And }
func (p *parser) parseOr() ast.Node {
	start := p.tok.Pos

	left := p.parseAnd()

	for p.tok.Kind == LOr {
		op := p.tok
		p.next()

		p.expectOperand(op)
		right := p.parseAnd()

		left = &ast.LogicalExpr{Meta: ast.At(start), Op: ast.LogicalOr, Left: left, Right: right}
	}

	return left
}

// parseAnd parses a conjunction.
//
//	And → Comparison { "&&" Comparison }
//
// It is where the parser recovers from errors: an operand followed by anything
// but a logical operator or the end of the expression is reported, and the
// tokens up to the next logical operator skipped.
func (p *parser) parseAnd() ast.Node {
	start := p.tok.Pos

	left := p.parseComparison()
	p.expectLogicalOpOrEnd(left)

	for p.tok.Kind == LAnd {
		op := p.tok
		p.next()

		p.expectOperand(op)
		right := p.parseComparison()
		p.expectLogicalOpOrEnd(right)

		left = &ast.LogicalExpr{Meta: ast.At(start), Op: ast.LogicalAnd, Left: left, Right: right}
	}

	return left
}

// expectLogicalOpOrEnd reports the token following an operand of a logical
// operation when it is neither a logical operator nor the end of the
// expression, and skips to the next logical operator.
func (p *parser) expectLogicalOpOrEnd(operand ast.Node) {
	if p.tok.Kind == LAnd || p.tok.Kind == LOr || p.atEndOfExpr() {
		return
	}

	hint := ""
	if operand != nil && p.startsOperand() {
		hint = "missing `&&` or `||`?"
	}
	p.errorExpected("`&&`, `||` or end of expression", hint)
	p.sync()
}

// atEndOfExpr returns whether the current token ends the expression being
// parsed: the end of the source, or the closing parenthesis of a
// sub-expression.
func (p *parser) atEndOfExpr() bool {
	return p.tok.Kind == EOF || (p.tok.Kind == RParen && p.depth > 0)
}

// sync skips tokens up to the next logical operator, or the end of the
// expression being parsed, after an error. The logical operators of a skipped
// sub-expression don't stop it.
func (p *parser) sync() {
	nested := 0
	for {
		switch p.tok.Kind {
		case EOF:
			return
		case LAnd, LOr:
			if nested == 0 {
				return
			}
		case LParen:
			nested++
		case RParen:
			if nested == 0 && p.depth > 0 {
				return
			}
			nested = max(nested-1, 0)
		}
		p.next()
	}
}

func scalarOp(kind Kind) (ast.ScalarOp, bool) {
	switch kind {
	case Eq:
		return ast.ScalarEqual, true
	case NotEq:
		return ast.ScalarNotEqual, true
	case Lt:
		return ast.ScalarLesser, true
	case Le:
		return ast.ScalarLesserOrEqual, true
	case Gt:
		return ast.ScalarGreater, true
	case Ge:
		return ast.ScalarGreaterOrEqual, true
	case Match:
		return ast.ScalarMatch, true
	case NotMatch:
		return ast.ScalarNotMatch, true
	}
	return 0, false
}

func (p *parser) startsComparison() bool {
	_, ok := scalarOp(p.tok.Kind)
	return ok || p.tok.Kind == In || p.tok.Kind == AllIn || p.tok.Kind == Not
}

// parseComparison parses a comparison.
//
//	Comparison → Operation(0) [ ScalarOp Operation(0) ]
//	Comparison → Operation(0) [ ArrayOp Operation(0) ]
//
//	ArrayOp → "in" | "not" "in" | "allin"
//
// The right operand of an array comparison is any operand: whether it is an
// array is checked at compile time.
func (p *parser) parseComparison() ast.Node {
	start := p.tok.Pos

	left := p.parseOperation(0)
	if !p.startsComparison() {
		return left
	}

	var node ast.Node
	if op, ok := scalarOp(p.tok.Kind); ok {
		opTok := p.tok
		p.next()

		if !p.expectOperand(opTok) {
			return nil
		}
		right := p.parseOperation(0)

		node = &ast.ScalarCompareExpr{Meta: ast.At(start), Op: op, Left: left, Right: right}
	} else {
		var op ast.ArrayOp
		opTok := p.tok
		switch p.tok.Kind {
		case In:
			op = ast.ArrayIn
		case AllIn:
			op = ast.ArrayAllIn
		case Not:
			// after an operand, `not` can only start a `not in`
			p.next()
			if p.tok.Kind != In {
				p.errorExpected("`in` after `not`", "")
				return nil
			}
			op, opTok = ast.ArrayNotIn, p.tok
		}
		p.next()

		if !p.expectOperand(opTok) {
			return nil
		}
		right := p.parseOperation(0)

		node = &ast.ArrayCompareExpr{Meta: ast.At(start), Op: op, Left: left, Right: right}
	}

	if p.startsComparison() {
		p.errorAt(p.tok, "comparisons can't be chained", "add parentheses")
		return nil
	}

	return node
}

// operatorLevels are the precedence levels of the bitwise and arithmetic
// operators, from the loosest to the tightest binding, as in Python and Rust.
var operatorLevels = [][]Kind{
	{BOr},
	{BXor},
	{BAnd},
	{Plus, Minus},
}

// parseOperation parses the operations of a precedence level of
// operatorLevels, the level past the last one being the unary operations.
//
//	Operation(n) → Operation(n+1) { Operator(n) Operation(n+1) }
func (p *parser) parseOperation(level int) ast.Node {
	if level == len(operatorLevels) {
		return p.parseUnary()
	}

	start := p.tok.Pos

	left := p.parseOperation(level + 1)

	for slices.Contains(operatorLevels[level], p.tok.Kind) {
		op := p.tok
		p.next()

		if !p.expectOperand(op) {
			return nil
		}
		right := p.parseOperation(level + 1)

		left = operation(ast.At(start), op.Kind, left, right)
	}

	return left
}

func operation(meta ast.Meta, kind Kind, left, right ast.Node) ast.Node {
	switch kind {
	case Plus:
		return &ast.ArithExpr{Meta: meta, Op: ast.ArithAdd, Left: left, Right: right}
	case Minus:
		return &ast.ArithExpr{Meta: meta, Op: ast.ArithSub, Left: left, Right: right}
	case BAnd:
		return &ast.BitExpr{Meta: meta, Op: ast.BitAnd, Left: left, Right: right}
	case BOr:
		return &ast.BitExpr{Meta: meta, Op: ast.BitOr, Left: left, Right: right}
	default:
		return &ast.BitExpr{Meta: meta, Op: ast.BitXor, Left: left, Right: right}
	}
}

// parseUnary parses a unary operation.
//
//	Unary → ( "!" | "not" | "-" | "^" ) Unary
//	Unary → Primary
//
// A sign directly followed by an integer, without whitespace, is part of the
// literal: `-1` is an integer literal, `- 1` and `-x` are negations.
func (p *parser) parseUnary() ast.Node {
	tok := p.tok

	var op ast.UnaryOp
	switch tok.Kind {
	case Bang, Not:
		op = ast.UnaryNot
	case BXor:
		op = ast.UnaryBitNot
	case Minus, Plus:
		p.next()
		if p.tok.Kind == Int && p.tok.Pos.Offset == tok.End() {
			return p.parseInt(&tok)
		}
		if tok.Kind == Plus {
			p.errorAt(tok, "unexpected `+`", "only an integer literal can be signed with `+`")
			return nil
		}
		return p.parseUnaryOperand(tok, ast.UnaryNeg)
	default:
		return p.parsePrimary()
	}

	p.next()
	return p.parseUnaryOperand(tok, op)
}

// parseUnaryOperand parses the operand of a unary operator, the operator being
// already consumed.
func (p *parser) parseUnaryOperand(opTok Token, op ast.UnaryOp) ast.Node {
	if !p.expectOperand(opTok) {
		return nil
	}
	operand := p.parseUnary()
	return &ast.UnaryExpr{Meta: ast.At(opTok.Pos), Op: op, Operand: operand}
}

// parsePrimary parses a single operand.
//
//	Primary → Name | Int | String | Pattern | Regexp | ... | Array | "(" Or ")"
func (p *parser) parsePrimary() ast.Node {
	tok := p.tok
	meta := ast.At(tok.Pos)

	switch tok.Kind {
	case Ident:
		p.next()
		return p.parseName(tok)
	case Int:
		return p.parseInt(nil)
	case String:
		if strings.Contains(tok.Text, "${") || strings.Contains(tok.Text, "%{") {
			return p.parseInterpolation()
		}
		p.next()
		return stringLiteral(tok)
	case Pattern:
		p.next()
		return &ast.Literal{Meta: meta, Value: ast.MakePattern(tok.Text[2 : len(tok.Text)-1])}
	case Regexp:
		p.next()
		return &ast.Literal{Meta: meta, Value: ast.MakeRegexp(tok.Text[2 : len(tok.Text)-1])}
	case Duration:
		p.next()
		// the scanner checked the duration
		value, _ := time.ParseDuration(tok.Text)
		return &ast.Literal{Meta: meta, Value: ast.MakeDuration(int(value.Nanoseconds()))}
	case IP:
		p.next()
		return &ast.Literal{Meta: meta, Value: ast.MakeIP(tok.Text)}
	case CIDR:
		p.next()
		return &ast.Literal{Meta: meta, Value: ast.MakeCIDR(tok.Text)}
	case Variable:
		p.next()
		return &ast.VariableRef{Meta: meta, Name: tok.Text[2 : len(tok.Text)-1]}
	case FieldOpen:
		return p.parseFieldRef()
	case LBracket:
		return p.parseArray()
	case LParen:
		return p.parseParenthesized()
	}

	p.errorExpected("operand", "")
	return nil
}

// parseName parses a name: an identifier, the token of which is already
// consumed, and the chain of the selectors and the subscripts over it.
//
//	Name → Ident { "." Ident | "[" Primary "]" }
func (p *parser) parseName(tok Token) ast.Node {
	meta := ast.At(tok.Pos)
	var node ast.Node = &ast.Ident{Meta: meta, Name: tok.Text}
	for {
		switch p.tok.Kind {
		case Dot:
			p.next()
			if p.tok.Kind != Ident {
				p.errorExpected("name after `.`", "")
				return nil
			}
			node = &ast.Selector{Meta: meta, Base: node, Member: p.tok.Text}
			p.next()
		case LBracket:
			open := p.tok
			p.next()
			index := p.parsePrimary()
			if index == nil {
				// already reported
				return nil
			}
			if p.tok.Kind != RBracket {
				p.errorExpected("`]`", "to close the `[` at "+open.Pos.String())
				return nil
			}
			p.next()
			node = &ast.Index{Meta: meta, Base: node, Subscript: index}
		default:
			return node
		}
	}
}

// parseFieldRef parses a field reference, `%{process.pid}`, the current token
// being its `%{`: the name inside the braces, the same as written bare,
// `process.pid`.
//
//	FieldRef → "%{" Name "}"
func (p *parser) parseFieldRef() ast.Node {
	open := p.tok
	p.next()

	if p.tok.Kind != Ident {
		p.errorExpected("field name after `%{`", "")
		return nil
	}
	ident := p.tok
	p.next()
	name := p.parseName(ident)
	if name == nil {
		// already reported
		return nil
	}

	if p.tok.Kind != RBrace {
		p.errorExpected("`}`", "to close the `%{` at "+open.Pos.String())
		return nil
	}
	p.next()
	return name
}

// parseInterpolation parses a string with values interpolated in it, the
// current token: `"pid:%{process.pid}"`, `"${my_var}/x"`. The scanner goes
// back into the string, and returns its parts. A string that turns out to have
// no interpolation, `"\${a}"`, is a literal.
//
//	Interpolation → { StringText | Variable | FieldRef } StringEnd
func (p *parser) parseInterpolation() ast.Node {
	tok := p.tok
	p.scanner.enterString(tok)
	p.next()

	var (
		parts        []ast.Node
		interpolated bool
		failed       bool
	)
	for p.tok.Kind != StringEnd && p.tok.Kind != EOF {
		part := p.tok
		switch part.Kind {
		case StringText:
			p.next()
			parts = append(parts, &ast.Literal{Meta: ast.At(part.Pos), Value: ast.MakeString(part.Text)})
		case Variable:
			p.next()
			interpolated = true
			parts = append(parts, &ast.VariableRef{Meta: ast.At(part.Pos), Name: part.Text[2 : len(part.Text)-1]})
		case FieldOpen:
			interpolated = true
			if name := p.parseFieldRef(); name != nil {
				parts = append(parts, name)
			} else {
				// already reported, the rest of the string is skipped
				failed = true
			}
		default:
			// an illegal token, already reported, or the rest of a string
			// skipped after an error
			p.next()
			failed = true
		}
	}
	p.next() // the closing quote

	switch {
	case failed:
		return nil
	case !interpolated:
		return stringLiteral(tok)
	}
	return &ast.Interpolation{Meta: ast.At(tok.Pos), Parts: parts}
}

// stringLiteral returns the literal of a string token, its escape sequences
// kept as written.
func stringLiteral(tok Token) *ast.Literal {
	return &ast.Literal{Meta: ast.At(tok.Pos), Value: ast.MakeString(tok.Text[1 : len(tok.Text)-1])}
}

// parseInt parses an integer literal, sign being the token of its sign, if
// any, already consumed.
func (p *parser) parseInt(sign *Token) ast.Node {
	tok := p.tok
	p.next()

	start, text := tok.Pos, tok.Text
	if sign != nil {
		start, text = sign.Pos, sign.Text+text
	}

	// an integer starting with a 0 is octal
	value, err := strconv.ParseInt(text, 0, strconv.IntSize)
	if err != nil {
		msg := fmt.Sprintf("invalid integer `%s`", text)
		if errors.Is(err, strconv.ErrRange) {
			msg = fmt.Sprintf("integer `%s` out of range", text)
		}
		p.errs.add(&Error{Pos: start, End: tok.End(), Msg: msg})
		return nil
	}
	return &ast.Literal{Meta: ast.At(start), Value: ast.MakeInt(int(value))}
}

// parseParenthesized parses a parenthesized expression. The parentheses are
// dropped: the node is the one of the inner expression.
//
//	Primary → "(" Or ")"
func (p *parser) parseParenthesized() ast.Node {
	open := p.tok
	p.next()
	p.depth++

	p.expectOperand(open)
	node := p.parseOr()

	p.depth--
	if p.tok.Kind != RParen {
		p.errorExpected("`)`", "to close the `(` at "+open.Pos.String())
		return nil
	}
	p.next()

	return node
}

// arrayKind is the kind of an array literal, set by its first member.
type arrayKind int

const (
	stringArray arrayKind = iota + 1 // strings, patterns and regexps
	cidrArray                        // IPs and CIDRs
	intArray                         // integers
	identArray                       // identifiers
)

var arrayKindNames = map[arrayKind]string{
	stringArray: "an array of strings",
	cidrArray:   "an array of IPs",
	intArray:    "an array of integers",
	identArray:  "an array of identifiers",
}

// memberKind returns the kind of array the current token can be a member of.
func (p *parser) memberKind() (arrayKind, bool) {
	switch p.tok.Kind {
	case String, Pattern, Regexp:
		return stringArray, true
	case IP, CIDR:
		return cidrArray, true
	case Int, Minus, Plus:
		return intArray, true
	case Ident:
		return identArray, true
	}
	return 0, false
}

// parseArray parses an array literal: `["a", ~"b"]`, `[1, 2]`. Its first
// member decides the kind of the array, and of the members that follow.
//
//	Array → "[" Member { "," Member } "]"
func (p *parser) parseArray() ast.Node {
	open := p.tok
	meta := ast.At(open.Pos)
	p.next()

	if p.tok.Kind == RBracket {
		p.errs.add(&Error{Pos: open.Pos, End: p.tok.End(), Msg: "empty array"})
		p.next()
		return nil
	}

	kind, ok := p.memberKind()
	if !ok {
		p.errorExpected("array member", "")
		p.skipArray()
		return nil
	}

	var (
		members []*ast.Literal
		idents  []string
	)

	for {
		if memberKind, ok := p.memberKind(); !ok || memberKind != kind {
			p.errorExpected("member of "+arrayKindNames[kind], "")
			p.skipArray()
			return nil
		}

		if kind == identArray {
			// a member is a constant, its name is kept as written
			tok := p.tok
			p.next()
			name := p.parseName(tok)
			if name == nil {
				// already reported
				p.skipArray()
				return nil
			}
			idents = append(idents, ast.Sprint(name))
		} else if member, ok := p.parseMember().(*ast.Literal); ok {
			members = append(members, member)
		} else {
			// already reported
			p.skipArray()
			return nil
		}

		if p.tok.Kind == RBracket {
			p.next()
			break
		}
		if p.tok.Kind != Comma {
			p.errorExpected("`,` or `]`", "to close the `[` at "+open.Pos.String())
			p.skipArray()
			return nil
		}
		p.next()
	}

	if kind == identArray {
		return &ast.IdentArray{Meta: meta, Names: idents}
	}
	return &ast.Array{Meta: meta, Members: members}
}

// skipArray skips the rest of an array literal after an error: the tokens up
// to its closing bracket, unless the expression ends first.
func (p *parser) skipArray() {
	for !p.atEndOfExpr() && p.tok.Kind != LAnd && p.tok.Kind != LOr {
		if p.tok.Kind == RBracket {
			p.next()
			return
		}
		p.next()
	}
}

// parseMember parses a member of an array literal: a literal, or an
// identifier. A string member is a literal, never interpolated: `["${IFS}"]`
// is the text `${IFS}`.
func (p *parser) parseMember() ast.Node {
	if tok := p.tok; tok.Kind == String {
		p.next()
		return stringLiteral(tok)
	}
	if p.tok.Kind != Minus && p.tok.Kind != Plus {
		return p.parsePrimary()
	}

	sign := p.tok
	p.next()
	if p.tok.Kind != Int || p.tok.Pos.Offset != sign.End() {
		p.errorAt(sign, "unexpected `"+sign.Text+"`", "a sign must directly precede its integer")
		return nil
	}
	return p.parseInt(&sign)
}

func (p *parser) startsOperand() bool {
	switch p.tok.Kind {
	case Ident, Int, Duration, String, Pattern, Regexp, IP, CIDR, Variable, FieldOpen,
		LParen, LBracket, Bang, Not, Minus, Plus, BXor:
		return true
	}
	return false
}

// expectOperand reports the current token when it can't start the operand of
// an operator, op being the token of the operator.
func (p *parser) expectOperand(op Token) bool {
	if p.startsOperand() {
		return true
	}
	p.errorExpected("operand after `"+op.Text+"`", "")
	return false
}

// errorExpected reports that the current token is not what was expected:
// `expected operand after "==", found end of expression`.
func (p *parser) errorExpected(expected, hint string) {
	p.errorAt(p.tok, "expected "+expected+", found "+p.tok.describe(), hint)
}

func (p *parser) errorAt(tok Token, msg, hint string) {
	p.errs.add(&Error{Pos: tok.Pos, End: tok.End(), Msg: msg, Hint: hint})
}
