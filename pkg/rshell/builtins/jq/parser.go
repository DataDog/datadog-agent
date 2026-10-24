// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"fmt"
)

type nodeKind uint8

const (
	nodeIdentity nodeKind = iota
	nodeLiteral
	nodeVariable
	nodeField
	nodeIndex
	nodeIterator
	nodeGroup
	nodeOptional
	nodePipe
	nodeComma
	nodeAlternative
	nodeBinary
	nodeNegate
	nodeArray
	nodeObject
	nodeCall
)

type objectNodeMember struct {
	literalKey string
	dynamicKey *node
	value      *node
}

type node struct {
	kind    nodeKind
	literal value
	name    string
	op      tokenKind
	left    *node
	right   *node
	child   *node
	members []objectNodeMember
}

type filterParser struct {
	tokens []token
	pos    int
	nodes  int
	depth  int
}

func (n *node) walk(visit func(*node) error) error {
	if n == nil {
		return nil
	}
	if err := visit(n); err != nil {
		return err
	}
	for _, child := range []*node{n.left, n.right, n.child} {
		if err := child.walk(visit); err != nil {
			return err
		}
	}
	for _, member := range n.members {
		if err := member.dynamicKey.walk(visit); err != nil {
			return err
		}
		if err := member.value.walk(visit); err != nil {
			return err
		}
	}
	return nil
}

func parseFilter(input string) (*node, error) {
	tokens, err := lexFilter(input)
	if err != nil {
		return nil, err
	}
	p := &filterParser{tokens: tokens}
	root, err := p.parsePipe()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokenEOF {
		return nil, p.unexpected("end of filter")
	}
	return root, nil
}

func (p *filterParser) peek() token {
	return p.tokens[p.pos]
}

func (p *filterParser) advance() token {
	tok := p.tokens[p.pos]
	if tok.kind != tokenEOF {
		p.pos++
	}
	return tok
}

func (p *filterParser) accept(kind tokenKind) bool {
	if p.peek().kind != kind {
		return false
	}
	p.advance()
	return true
}

func (p *filterParser) expect(kind tokenKind, expected string) error {
	if !p.accept(kind) {
		return p.unexpected(expected)
	}
	return nil
}

func (p *filterParser) unexpected(expected string) error {
	tok := p.peek()
	if tok.kind == tokenEOF {
		return fmt.Errorf("expected %s at end of filter", expected)
	}
	return fmt.Errorf("expected %s, got %q at byte %d", expected, tok.text, tok.pos)
}

func (p *filterParser) makeNode(n node) (*node, error) {
	p.nodes++
	if p.nodes > MaxFilterNodes {
		return nil, fmt.Errorf("filter exceeds the %d-node limit", MaxFilterNodes)
	}
	return &n, nil
}

func (p *filterParser) enter() error {
	p.depth++
	if p.depth > MaxNestingDepth {
		p.depth--
		return fmt.Errorf("filter exceeds the nesting limit of %d", MaxNestingDepth)
	}
	return nil
}

func (p *filterParser) leave() { p.depth-- }

// Precedence rises from pipe through comma, alternative, boolean, comparison, and arithmetic.
func (p *filterParser) parsePipe() (*node, error) {
	operands := make([]*node, 0, 1)
	first, err := p.parseComma()
	if err != nil {
		return nil, err
	}
	operands = append(operands, first)
	for p.accept(tokenPipe) {
		operand, err := p.parseComma()
		if err != nil {
			return nil, err
		}
		operands = append(operands, operand)
	}
	result := operands[len(operands)-1]
	for i := len(operands) - 2; i >= 0; i-- {
		result, err = p.makeNode(node{kind: nodePipe, left: operands[i], right: result})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *filterParser) parseComma() (*node, error) {
	left, err := p.parseAlternative()
	if err != nil {
		return nil, err
	}
	for p.accept(tokenComma) {
		right, err := p.parseAlternative()
		if err != nil {
			return nil, err
		}
		left, err = p.makeNode(node{kind: nodeComma, left: left, right: right})
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *filterParser) parseAlternative() (*node, error) {
	operands := make([]*node, 0, 1)
	first, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	operands = append(operands, first)
	for p.accept(tokenAlternative) {
		operand, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		operands = append(operands, operand)
	}
	result := operands[len(operands)-1]
	for i := len(operands) - 2; i >= 0; i-- {
		result, err = p.makeNode(node{kind: nodeAlternative, left: operands[i], right: result})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *filterParser) parseOr() (*node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenIdentifier && p.peek().text == "or" {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left, err = p.makeNode(node{kind: nodeBinary, name: "or", left: left, right: right})
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *filterParser) parseAnd() (*node, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenIdentifier && p.peek().text == "and" {
		p.advance()
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left, err = p.makeNode(node{kind: nodeBinary, name: "and", left: left, right: right})
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *filterParser) parseComparison() (*node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	if !isComparison(p.peek().kind) {
		return left, nil
	}
	op := p.advance().kind
	right, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	if isComparison(p.peek().kind) {
		return nil, fmt.Errorf("comparison operators cannot be chained at byte %d", p.peek().pos)
	}
	return p.makeNode(node{kind: nodeBinary, op: op, left: left, right: right})
}

func (p *filterParser) parseAdditive() (*node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenPlus || p.peek().kind == tokenMinus {
		op := p.advance().kind
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left, err = p.makeNode(node{kind: nodeBinary, op: op, left: left, right: right})
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *filterParser) parseMultiplicative() (*node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenMultiply || p.peek().kind == tokenDivide || p.peek().kind == tokenModulo {
		op := p.advance().kind
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left, err = p.makeNode(node{kind: nodeBinary, op: op, left: left, right: right})
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *filterParser) parseUnary() (*node, error) {
	if !p.accept(tokenMinus) {
		return p.parsePostfix()
	}
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()
	child, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	return p.makeNode(node{kind: nodeNegate, child: child})
}

func (p *filterParser) parsePostfix() (*node, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tokenDot:
			dot := p.advance()
			if p.peek().kind == tokenLeftBracket {
				continue
			}
			field := p.peek()
			if !isFieldAfterDot(dot, field) {
				return nil, p.unexpected("field name")
			}
			p.advance()
			base, err = p.makeNode(node{kind: nodeField, left: base, name: field.text})
		case tokenLeftBracket:
			p.advance()
			if p.accept(tokenRightBracket) {
				base, err = p.makeNode(node{kind: nodeIterator, left: base})
				break
			}
			if err = p.enter(); err != nil {
				return nil, err
			}
			var index *node
			index, err = p.parsePipe()
			p.leave()
			if err == nil {
				err = p.expect(tokenRightBracket, "']'")
			}
			if err == nil {
				base, err = p.makeNode(node{kind: nodeIndex, left: base, right: index})
			}
		case tokenOptional:
			p.advance()
			base, err = p.makeNode(node{kind: nodeOptional, child: base})
		default:
			return base, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (p *filterParser) parsePrimary() (*node, error) {
	tok := p.advance()
	switch tok.kind {
	case tokenDot:
		base, err := p.makeNode(node{kind: nodeIdentity})
		if err != nil {
			return nil, err
		}
		if isFieldAfterDot(tok, p.peek()) {
			field := p.advance()
			return p.makeNode(node{kind: nodeField, left: base, name: field.text})
		}
		return base, nil
	case tokenVariable:
		return p.makeNode(node{kind: nodeVariable, name: tok.text})
	case tokenNumber:
		literal, err := parseNumber(tok.text)
		if err != nil {
			return nil, err
		}
		return p.makeNode(node{kind: nodeLiteral, literal: literal})
	case tokenString:
		literal, err := stringValue(tok.text)
		if err != nil {
			return nil, err
		}
		return p.makeNode(node{kind: nodeLiteral, literal: literal})
	case tokenIdentifier:
		return p.parseIdentifier(tok)
	case tokenLeftParen:
		if err := p.enter(); err != nil {
			return nil, err
		}
		child, err := p.parsePipe()
		p.leave()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokenRightParen, "')'"); err != nil {
			return nil, err
		}
		return p.makeNode(node{kind: nodeGroup, child: child})
	case tokenLeftBracket:
		return p.parseArray()
	case tokenLeftBrace:
		return p.parseObject()
	default:
		if tok.kind != tokenEOF {
			p.pos--
		}
		return nil, p.unexpected("a filter expression")
	}
}

func (p *filterParser) parseIdentifier(tok token) (*node, error) {
	switch tok.text {
	case "null":
		return p.makeNode(node{kind: nodeLiteral, literal: null()})
	case "true":
		return p.makeNode(node{kind: nodeLiteral, literal: boolean(true)})
	case "false":
		return p.makeNode(node{kind: nodeLiteral, literal: boolean(false)})
	case "length", "keys", "type", "empty", "not":
		return p.makeNode(node{kind: nodeCall, name: tok.text})
	case "select", "map", "has":
		if err := p.expect(tokenLeftParen, "'('"); err != nil {
			return nil, err
		}
		if err := p.enter(); err != nil {
			return nil, err
		}
		arg, err := p.parsePipe()
		p.leave()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokenRightParen, "')'"); err != nil {
			return nil, err
		}
		return p.makeNode(node{kind: nodeCall, name: tok.text, child: arg})
	default:
		return nil, fmt.Errorf("function %s is not supported at byte %d", tok.text, tok.pos)
	}
}

func (p *filterParser) parseArray() (*node, error) {
	if p.accept(tokenRightBracket) {
		return p.makeNode(node{kind: nodeArray})
	}
	if err := p.enter(); err != nil {
		return nil, err
	}
	child, err := p.parsePipe()
	p.leave()
	if err != nil {
		return nil, err
	}
	if err := p.expect(tokenRightBracket, "']'"); err != nil {
		return nil, err
	}
	return p.makeNode(node{kind: nodeArray, child: child})
}

func (p *filterParser) parseObject() (*node, error) {
	members := make([]objectNodeMember, 0)
	if p.accept(tokenRightBrace) {
		return p.makeNode(node{kind: nodeObject, members: members})
	}
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	for {
		member, err := p.parseObjectMember()
		if err != nil {
			return nil, err
		}
		members = append(members, member)
		if p.accept(tokenRightBrace) {
			break
		}
		if err := p.expect(tokenComma, "',' or '}'"); err != nil {
			return nil, err
		}
		if p.accept(tokenRightBrace) {
			break
		}
	}
	return p.makeNode(node{kind: nodeObject, members: members})
}

func isFieldAfterDot(dot, field token) bool {
	return field.kind == tokenString ||
		field.kind == tokenIdentifier && field.pos == dot.pos+1
}

func (p *filterParser) parseObjectMember() (objectNodeMember, error) {
	tok := p.advance()
	switch tok.kind {
	case tokenString, tokenIdentifier:
		if p.accept(tokenColon) {
			valueNode, err := p.parseObjectValue()
			if err != nil {
				return objectNodeMember{}, err
			}
			return objectNodeMember{literalKey: tok.text, value: valueNode}, nil
		}
		identity, err := p.makeNode(node{kind: nodeIdentity})
		if err != nil {
			return objectNodeMember{}, err
		}
		field, err := p.makeNode(node{kind: nodeField, left: identity, name: tok.text})
		if err != nil {
			return objectNodeMember{}, err
		}
		return objectNodeMember{literalKey: tok.text, value: field}, nil
	case tokenVariable:
		variable, err := p.makeNode(node{kind: nodeVariable, name: tok.text})
		if err != nil {
			return objectNodeMember{}, err
		}
		if p.accept(tokenColon) {
			valueNode, err := p.parseObjectValue()
			if err != nil {
				return objectNodeMember{}, err
			}
			return objectNodeMember{dynamicKey: variable, value: valueNode}, nil
		}
		return objectNodeMember{literalKey: tok.text, value: variable}, nil
	case tokenLeftParen:
		key, err := p.parsePipe()
		if err != nil {
			return objectNodeMember{}, err
		}
		if err := p.expect(tokenRightParen, "')'"); err != nil {
			return objectNodeMember{}, err
		}
		if err := p.expect(tokenColon, "':'"); err != nil {
			return objectNodeMember{}, err
		}
		valueNode, err := p.parseObjectValue()
		if err != nil {
			return objectNodeMember{}, err
		}
		return objectNodeMember{dynamicKey: key, value: valueNode}, nil
	default:
		if tok.kind != tokenEOF {
			p.pos--
		}
		return objectNodeMember{}, p.unexpected("an object key")
	}
}

// Object values admit an unparenthesized pipe; a top-level comma separates members.
func (p *filterParser) parseObjectValue() (*node, error) {
	left, err := p.parseAlternative()
	if err != nil {
		return nil, err
	}
	if !p.accept(tokenPipe) {
		return left, nil
	}
	right, err := p.parseObjectValue()
	if err != nil {
		return nil, err
	}
	return p.makeNode(node{kind: nodePipe, left: left, right: right})
}

func isComparison(kind tokenKind) bool {
	switch kind {
	case tokenEqual, tokenNotEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		return true
	default:
		return false
	}
}
