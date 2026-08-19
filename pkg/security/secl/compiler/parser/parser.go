// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language.
//
// It is a recursive descent parser, one function per precedence level, from
// the loosest to the tightest binding:
//
//	or              a || b, a or b
//	and             a && b, a and b
//	comparison      a == b, a != b, a < b, a <= b, a > b, a >= b, a =~ b, a !~ b,
//	                a in b, a not in b, a allin b
//	additive        a + b, a - b, a | b, a ^ b
//	multiplicative  a & b
//	unary           !a, not a, -a, ^a
//	primary         literals, identifiers, arrays, (a)
//
// Binary operators are left associative, `a - b - c` is `(a - b) - c`, but for
// comparisons that can't be chained: `a == b == c` is an error.
//
// The parser looks one token ahead, and never backtracks.
package parser

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// ParseExpression parses a SECL expression.
//
// On failure, the error is an ErrorList holding every syntax error found.
func ParseExpression(expr string) (ast.Node, error) {
	_, root, err := parse(expr)
	return root, err
}

// ParseRule parses the expression of a SECL rule.
//
// On failure, the error is an ErrorList holding every syntax error found.
func ParseRule(expr string) (*ast.Rule, error) {
	start, root, err := parse(expr)
	if err != nil {
		return nil, err
	}
	return &ast.Rule{Meta: ast.At(start), Expr: expr, Root: root}, nil
}

// ParseMacro parses the expression of a SECL macro.
//
// On failure, the error is an ErrorList holding every syntax error found.
func ParseMacro(expr string) (*ast.Macro, error) {
	start, root, err := parse(expr)
	if err != nil {
		return nil, err
	}
	return &ast.Macro{Meta: ast.At(start), Root: root}, nil
}

// parse parses an expression, and returns the position of its first token and
// its tree, or its errors.
func parse(expr string) (start ast.Position, root ast.Node, err error) {
	p := &parser{}
	p.scanner = newScanner(expr, &p.errs)

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

// parser holds the state of the parsing of an expression.
//
// A parsing function reports the errors it finds and keeps going, returning a
// nil node where it couldn't build one, without consuming the faulty token. The
// `and` level recovers from the errors of its operands by skipping to the next
// logical operator, so that the errors of each operand get reported.
type parser struct {
	scanner *scanner
	errs    ErrorList

	// tok is the current token, the one the parser looks ahead at.
	tok Token
	// depth is the number of opened parentheses.
	depth int
}

// next moves to the next token.
func (p *parser) next() {
	p.tok = p.scanner.next()
}

// parseRoot parses a whole expression.
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

	for p.tok.Kind == OrOr {
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

	for p.tok.Kind == AndAnd {
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
	if p.tok.Kind == AndAnd || p.tok.Kind == OrOr || p.atEndOfExpr() {
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
		case AndAnd, OrOr:
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

// scalarOp returns the operator of a scalar comparison.
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

// startsComparison returns whether the current token is a comparison
// operator.
func (p *parser) startsComparison() bool {
	_, ok := scalarOp(p.tok.Kind)
	return ok || p.tok.Kind == In || p.tok.Kind == AllIn || p.tok.Kind == Not
}

// parseComparison parses a comparison.
//
//	Comparison → Additive [ ScalarOp Additive ]
//	Comparison → Additive [ ArrayOp Additive ]
//
//	ArrayOp → "in" | "not" "in" | "allin"
//
// The right operand of an array comparison is any operand: whether it is an
// array is checked by the compilation.
func (p *parser) parseComparison() ast.Node {
	start := p.tok.Pos

	left := p.parseAdditive()
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
		right := p.parseAdditive()

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
		right := p.parseAdditive()

		node = &ast.ArrayCompareExpr{Meta: ast.At(start), Op: op, Left: left, Right: right}
	}

	if p.startsComparison() {
		p.errorAt(p.tok, "comparisons can't be chained", "add parentheses")
		return nil
	}

	return node
}

// parseAdditive parses an additive operation, arithmetic or bitwise.
//
//	Additive → Multiplicative { ( "+" | "-" | "|" | "^" ) Multiplicative }
func (p *parser) parseAdditive() ast.Node {
	start := p.tok.Pos

	left := p.parseMultiplicative()

	for {
		var build func(right ast.Node) ast.Node
		switch p.tok.Kind {
		case Plus:
			build = arith(start, ast.ArithAdd, left)
		case Minus:
			build = arith(start, ast.ArithSub, left)
		case Pipe:
			build = bit(start, ast.BitOr, left)
		case Caret:
			build = bit(start, ast.BitXor, left)
		default:
			return left
		}

		op := p.tok
		p.next()

		if !p.expectOperand(op) {
			return nil
		}
		left = build(p.parseMultiplicative())
	}
}

// parseMultiplicative parses a multiplicative operation.
//
//	Multiplicative → Unary { "&" Unary }
func (p *parser) parseMultiplicative() ast.Node {
	start := p.tok.Pos

	left := p.parseUnary()

	for p.tok.Kind == Amp {
		op := p.tok
		p.next()

		if !p.expectOperand(op) {
			return nil
		}
		left = bit(start, ast.BitAnd, left)(p.parseUnary())
	}

	return left
}

func arith(start ast.Position, op ast.ArithOp, left ast.Node) func(ast.Node) ast.Node {
	return func(right ast.Node) ast.Node {
		return &ast.ArithExpr{Meta: ast.At(start), Op: op, Left: left, Right: right}
	}
}

func bit(start ast.Position, op ast.BitOp, left ast.Node) func(ast.Node) ast.Node {
	return func(right ast.Node) ast.Node {
		return &ast.BitExpr{Meta: ast.At(start), Op: op, Left: left, Right: right}
	}
}

// parseUnary parses a unary operation.
//
//	Unary → ( "!" | "not" | "-" | "^" ) Unary
//	Unary → Primary
//
// A sign directly followed by an integer, without whitespace, is part of the
// literal: `-1` is an IntLiteral, `- 1` and `-x` are negations.
func (p *parser) parseUnary() ast.Node {
	tok := p.tok

	var op ast.UnaryOp
	switch tok.Kind {
	case Bang, Not:
		op = ast.UnaryNot
	case Caret:
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
//	Primary → Ident | Int | String | Pattern | Regexp | ... | Array | "(" Or ")"
func (p *parser) parsePrimary() ast.Node {
	tok := p.tok
	meta := ast.At(tok.Pos)

	switch tok.Kind {
	case Ident:
		p.next()
		return &ast.Ident{Meta: meta, Name: tok.Text}
	case Int:
		return p.parseInt(nil)
	case String:
		p.next()
		// escape sequences are kept as written
		return &ast.StringLiteral{Meta: meta, Value: tok.Text[1 : len(tok.Text)-1]}
	case Pattern:
		p.next()
		return &ast.PatternLiteral{Meta: meta, Value: tok.Text[2 : len(tok.Text)-1]}
	case Regexp:
		p.next()
		return &ast.RegexpLiteral{Meta: meta, Value: tok.Text[2 : len(tok.Text)-1]}
	case Duration:
		p.next()
		// the scanner checked the duration
		value, _ := time.ParseDuration(tok.Text)
		return &ast.DurationLiteral{Meta: meta, Value: int(value.Nanoseconds())}
	case IP:
		p.next()
		return &ast.IPLiteral{Meta: meta, Value: tok.Text}
	case CIDR:
		p.next()
		return &ast.CIDRLiteral{Meta: meta, Value: tok.Text}
	case Variable:
		p.next()
		return &ast.VariableRef{Meta: meta, Name: tok.Text[2 : len(tok.Text)-1]}
	case FieldRef:
		p.next()
		return &ast.FieldRef{Meta: meta, Name: tok.Text[2 : len(tok.Text)-1]}
	case LBracket:
		return p.parseArray()
	case LParen:
		return p.parseParenthesized()
	}

	p.errorExpected("operand", "")
	return nil
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
	return &ast.IntLiteral{Meta: ast.At(start), Value: int(value)}
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
		stringMembers []ast.StringArrayMember
		cidrMembers   []ast.CIDRArrayMember
		ints          []int
		idents        []string
	)

	for {
		if memberKind, ok := p.memberKind(); !ok || memberKind != kind {
			p.errorExpected("member of "+arrayKindNames[kind], "")
			p.skipArray()
			return nil
		}

		switch member := p.parseMember().(type) {
		case ast.StringArrayMember:
			stringMembers = append(stringMembers, member)
		case ast.CIDRArrayMember:
			cidrMembers = append(cidrMembers, member)
		case *ast.IntLiteral:
			ints = append(ints, member.Value)
		case *ast.Ident:
			idents = append(idents, member.Name)
		default:
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

	switch kind {
	case stringArray:
		return &ast.StringArray{Meta: meta, Members: stringMembers}
	case cidrArray:
		return &ast.CIDRArray{Meta: meta, Members: cidrMembers}
	case intArray:
		return &ast.IntArray{Meta: meta, Values: ints}
	default:
		return &ast.IdentArray{Meta: meta, Names: idents}
	}
}

// skipArray skips the rest of an array literal after an error: the tokens up
// to its closing bracket, unless the expression ends first.
func (p *parser) skipArray() {
	for !p.atEndOfExpr() && p.tok.Kind != AndAnd && p.tok.Kind != OrOr {
		if p.tok.Kind == RBracket {
			p.next()
			return
		}
		p.next()
	}
}

// parseMember parses a member of an array literal: a literal, or an
// identifier.
func (p *parser) parseMember() ast.Node {
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

// startsOperand returns whether the current token can start an operand.
func (p *parser) startsOperand() bool {
	switch p.tok.Kind {
	case Ident, Int, Duration, String, Pattern, Regexp, IP, CIDR, Variable, FieldRef,
		LParen, LBracket, Bang, Not, Minus, Plus, Caret:
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

// errorAt reports an error over a token.
func (p *parser) errorAt(tok Token, msg, hint string) {
	p.errs.add(&Error{Pos: tok.Pos, End: tok.End(), Msg: msg, Hint: hint})
}
