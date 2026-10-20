// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"context"
	"fmt"
)

const (
	maxParserDepth        = 512
	maxFunctionArguments  = 256
	maxFunctionParameters = 256

	precAssign  = 10
	precTernary = 15
	precOr      = 20
	precAnd     = 30
	precIn      = 35
	precMatch   = 40
	precCompare = 45
	precConcat  = 50
	precAdd     = 60
	precMul     = 70
	precPrefix  = 80
	precPower   = 90
	precPostfix = 100
)

var unsupportedBuiltinFunctions = map[string]struct{}{
	"and":            {},
	"asort":          {},
	"asorti":         {},
	"atan2":          {},
	"bindtextdomain": {},
	"compl":          {},
	"close":          {},
	"cos":            {},
	"dcgettext":      {},
	"dcngettext":     {},
	"exp":            {},
	"fflush":         {},
	"gensub":         {},
	"isarray":        {},
	"log":            {},
	"lshift":         {},
	"mkbool":         {},
	"mktime":         {},
	"or":             {},
	"patsplit":       {},
	"rand":           {},
	"rshift":         {},
	"sin":            {},
	"sqrt":           {},
	"srand":          {},
	"strftime":       {},
	"strtonum":       {},
	"system":         {},
	"systime":        {},
	"typeof":         {},
	"xor":            {},
}

var supportedBuiltinFunctions = map[string]struct{}{
	"gsub":    {},
	"index":   {},
	"int":     {},
	"length":  {},
	"match":   {},
	"split":   {},
	"sprintf": {},
	"sub":     {},
	"substr":  {},
	"tolower": {},
	"toupper": {},
}

type parser struct {
	ctx               context.Context
	toks              []token
	pos               int
	stopPrintRedirect bool
	nestingDepth      int
	inFunction        bool
}

func (p *parser) enterNesting() error {
	if p.ctx != nil {
		if err := p.ctx.Err(); err != nil {
			return err
		}
	}
	if p.nestingDepth >= maxParserDepth {
		return parserNestingError()
	}
	p.nestingDepth++
	return nil
}

func (p *parser) leaveNesting() {
	p.nestingDepth--
}

func parserNestingError() error {
	return fmt.Errorf("parser nesting depth limit exceeded (maximum %d)", maxParserDepth)
}

type syntaxNodeDepth struct {
	node  any
	depth int
}

func validateProgramNestingContext(ctx context.Context, prog *program) error {
	stack := make([]syntaxNodeDepth, 0)
	pushExpr := func(x expr, depth int) {
		if x != nil {
			stack = append(stack, syntaxNodeDepth{node: x, depth: depth})
		}
	}
	pushStmt := func(s stmt, depth int) {
		if s != nil {
			stack = append(stack, syntaxNodeDepth{node: s, depth: depth})
		}
	}
	for _, r := range prog.rules {
		pushExpr(r.pattern, 1)
		for _, s := range r.action {
			pushStmt(s, 1)
		}
	}
	for _, fn := range prog.functions {
		for _, s := range fn.body {
			pushStmt(s, 1)
		}
	}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if item.depth > maxParserDepth {
			return parserNestingError()
		}
		childDepth := item.depth + 1
		switch n := item.node.(type) {
		case *printStmt:
			for _, x := range n.args {
				pushExpr(x, childDepth)
			}
		case *printfStmt:
			for _, x := range n.args {
				pushExpr(x, childDepth)
			}
		case *ifStmt:
			pushExpr(n.cond, childDepth)
			for _, s := range n.thenStmts {
				pushStmt(s, childDepth)
			}
			for _, s := range n.elseStmts {
				pushStmt(s, childDepth)
			}
		case *forInStmt:
			for _, s := range n.body {
				pushStmt(s, childDepth)
			}
		case *forStmt:
			pushExpr(n.init, childDepth)
			pushExpr(n.cond, childDepth)
			pushExpr(n.post, childDepth)
			for _, s := range n.body {
				pushStmt(s, childDepth)
			}
		case *whileStmt:
			pushExpr(n.cond, childDepth)
			for _, s := range n.body {
				pushStmt(s, childDepth)
			}
		case *exitStmt:
			pushExpr(n.status, childDepth)
		case *returnStmt:
			pushExpr(n.value, childDepth)
		case *deleteStmt:
			for _, x := range n.indices {
				pushExpr(x, childDepth)
			}
		case *exprStmt:
			pushExpr(n.x, childDepth)
		case *arrayRefExpr:
			for _, x := range n.indices {
				pushExpr(x, childDepth)
			}
		case *compositeExpr:
			for _, x := range n.parts {
				pushExpr(x, childDepth)
			}
		case *fieldExpr:
			pushExpr(n.index, childDepth)
		case *groupedExpr:
			pushExpr(n.x, childDepth)
		case *unaryExpr:
			pushExpr(n.x, childDepth)
		case *binaryExpr:
			pushExpr(n.left, childDepth)
			pushExpr(n.right, childDepth)
		case *ternaryExpr:
			pushExpr(n.cond, childDepth)
			pushExpr(n.then, childDepth)
			pushExpr(n.els, childDepth)
		case *rangeExpr:
			pushExpr(n.start, childDepth)
			pushExpr(n.end, childDepth)
		case *assignExpr:
			pushExpr(n.left, childDepth)
			pushExpr(n.right, childDepth)
		case *incDecExpr:
			pushExpr(n.x, childDepth)
		case *callExpr:
			for _, x := range n.args {
				pushExpr(x, childDepth)
			}
		case *nextStmt, *breakStmt, *continueStmt, *numberExpr, *stringExpr, *regexExpr, *varExpr:
		default:
			return fmt.Errorf("unknown syntax node")
		}
	}
	return nil
}

func parseProgram(src string) (*program, error) {
	return parseProgramContext(context.Background(), src)
}

func parseProgramContext(ctx context.Context, src string) (*program, error) {
	toks, err := lexContext(ctx, src)
	if err != nil {
		return nil, err
	}
	p := &parser{ctx: ctx, toks: toks}
	prog := &program{functions: make(map[string]*functionDef)}
	p.skipNewlines()
	for !p.at(tokEOF) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		selfTerminating := false
		if p.atIdent("function") {
			fn, err := p.parseFunctionDefinition()
			if err != nil {
				return nil, err
			}
			if _, exists := prog.functions[fn.name]; exists {
				return nil, fmt.Errorf("function %q is already defined", fn.name)
			}
			prog.functions[fn.name] = fn
			selfTerminating = true
		} else {
			r, err := p.parseRule()
			if err != nil {
				return nil, err
			}
			prog.rules = append(prog.rules, r)
			selfTerminating = r.action != nil
		}
		if selfTerminating {
			p.skipNewlines()
		}
		if p.match(tokSemicolon) {
			p.skipNewlines()
		} else if !selfTerminating {
			p.skipNewlines()
		}
	}
	if err := validateProgramNestingContext(ctx, prog); err != nil {
		return nil, err
	}
	if err := validateLoopControlStatementsContext(ctx, prog); err != nil {
		return nil, err
	}
	if err := validateUserFunctionNameReferencesContext(ctx, prog); err != nil {
		return nil, err
	}
	return prog, nil
}

func (p *parser) parseFunctionDefinition() (*functionDef, error) {
	p.advance()
	if p.cur().kind != tokIdent {
		return nil, fmt.Errorf("expected function name")
	}
	name := p.cur().lit
	if err := validateFunctionName(name); err != nil {
		return nil, err
	}
	p.advance()
	if !p.match(tokLParen) {
		return nil, fmt.Errorf("expected ( after function name")
	}
	params := []string{}
	seen := make(map[string]int)
	if !p.match(tokRParen) {
		for {
			if p.cur().kind != tokIdent {
				return nil, fmt.Errorf("expected function parameter")
			}
			if len(params) >= maxFunctionParameters {
				return nil, fmt.Errorf("function %q has too many parameters (maximum %d)", name, maxFunctionParameters)
			}
			param := p.cur().lit
			if err := validateFunctionParameterName(name, param); err != nil {
				return nil, err
			}
			if first, ok := seen[param]; ok {
				return nil, fmt.Errorf("function %q parameter #%d, %q, duplicates parameter #%d", name, len(params)+1, param, first)
			}
			seen[param] = len(params) + 1
			params = append(params, param)
			p.advance()
			if p.match(tokRParen) {
				break
			}
			if !p.match(tokComma) {
				return nil, fmt.Errorf("expected , or ) in function parameter list")
			}
			p.skipNewlines()
		}
	}
	p.skipNewlines()
	p.inFunction = true
	body, err := p.parseAction()
	p.inFunction = false
	if err != nil {
		return nil, err
	}
	return &functionDef{name: name, params: params, body: body}, nil
}

func (p *parser) parseRule() (rule, error) {
	if p.atIdent("BEGIN") {
		p.advance()
		action, err := p.parseAction()
		return rule{kind: ruleBegin, action: action}, err
	}
	if p.atIdent("END") {
		p.advance()
		action, err := p.parseAction()
		return rule{kind: ruleEnd, action: action}, err
	}
	if p.at(tokLBrace) {
		action, err := p.parseAction()
		return rule{kind: ruleNormal, action: action}, err
	}
	pattern, err := p.parseExpression(0)
	if err != nil {
		return rule{}, err
	}
	if p.at(tokComma) {
		p.advance()
		p.skipNewlines()
		end, err := p.parseExpression(0)
		if err != nil {
			return rule{}, err
		}
		pattern = &rangeExpr{start: pattern, end: end}
	}
	if p.at(tokLBrace) {
		action, err := p.parseAction()
		return rule{kind: ruleNormal, pattern: pattern, action: action}, err
	}
	return rule{kind: ruleNormal, pattern: pattern}, nil
}

func (p *parser) parseAction() ([]stmt, error) {
	if !p.match(tokLBrace) {
		return nil, fmt.Errorf("expected action")
	}
	return p.parseStatementList()
}

func (p *parser) parseStatementList() ([]stmt, error) {
	stmts := []stmt{}
	p.skipSeparators()
	for !p.at(tokRBrace) {
		if p.at(tokEOF) {
			return nil, fmt.Errorf("unterminated action")
		}
		st, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, st)
		if !p.at(tokRBrace) && !p.at(tokEOF) && !isSeparator(p.cur().kind) && !statementEndsBlock(st) {
			return nil, fmt.Errorf("expected statement separator")
		}
		p.skipSeparators()
	}
	p.advance()
	return stmts, nil
}

func statementEndsBlock(st stmt) bool {
	switch s := st.(type) {
	case *ifStmt:
		return s.endsBlock
	case *forStmt:
		return s.endsBlock
	case *forInStmt:
		return s.endsBlock
	case *whileStmt:
		return s.endsBlock
	default:
		return false
	}
}

func (p *parser) parseStatement() (stmt, error) {
	if err := p.enterNesting(); err != nil {
		return nil, err
	}
	defer p.leaveNesting()

	if p.atIdent("if") {
		return p.parseIf()
	}
	if p.atIdent("for") {
		return p.parseFor()
	}
	if p.atIdent("while") {
		return p.parseWhile()
	}
	if p.atIdent("next") {
		p.advance()
		return &nextStmt{}, nil
	}
	if p.atIdent("exit") {
		return p.parseExit()
	}
	if p.atIdent("return") {
		return p.parseReturn()
	}
	if p.atIdent("break") {
		p.advance()
		return &breakStmt{}, nil
	}
	if p.atIdent("continue") {
		p.advance()
		return &continueStmt{}, nil
	}
	if p.atIdent("print") {
		return p.parsePrint()
	}
	if p.atIdent("printf") {
		return p.parsePrintf()
	}
	if p.atIdent("nextfile") {
		return nil, fmt.Errorf("control flow statements are not supported")
	}
	if p.atIdent("delete") {
		return p.parseDelete()
	}
	x, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	return &exprStmt{x: x}, nil
}

func (p *parser) parseExit() (stmt, error) {
	p.advance()
	if p.at(tokRBrace) || p.at(tokEOF) || isSeparator(p.cur().kind) {
		return &exitStmt{}, nil
	}
	status, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	return &exitStmt{status: status}, nil
}

func (p *parser) parseReturn() (stmt, error) {
	if !p.inFunction {
		return nil, fmt.Errorf("return is not allowed outside a function")
	}
	p.advance()
	if p.at(tokRBrace) || p.at(tokEOF) || isSeparator(p.cur().kind) {
		return &returnStmt{}, nil
	}
	x, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	return &returnStmt{value: x}, nil
}

func (p *parser) parseFor() (stmt, error) {
	p.advance()
	if !p.match(tokLParen) {
		return nil, fmt.Errorf("expected ( after for")
	}
	if p.cur().kind == tokIdent && p.peek(1).kind == tokIdent && p.peek(1).lit == "in" {
		varName := p.cur().lit
		if err := validateIdentifierReference(varName); err != nil {
			return nil, err
		}
		p.advance()
		p.advance()
		if p.cur().kind != tokIdent {
			return nil, fmt.Errorf("expected array name in for loop")
		}
		arrayName := p.cur().lit
		if err := validateIdentifierReference(arrayName); err != nil {
			return nil, err
		}
		p.advance()
		if !p.match(tokRParen) {
			return nil, fmt.Errorf("expected ) after for loop")
		}
		body, braced, err := p.parseStatementGroup()
		if err != nil {
			return nil, err
		}
		return &forInStmt{varName: varName, arrayName: arrayName, body: body, endsBlock: braced}, nil
	}
	init, err := p.parseOptionalForExpr(tokSemicolon)
	if err != nil {
		return nil, err
	}
	if !p.match(tokSemicolon) {
		return nil, fmt.Errorf("expected ; in for loop")
	}
	p.skipNewlines()
	cond, err := p.parseOptionalForExpr(tokSemicolon)
	if err != nil {
		return nil, err
	}
	if !p.match(tokSemicolon) {
		return nil, fmt.Errorf("expected ; in for loop")
	}
	p.skipNewlines()
	post, err := p.parseOptionalForExpr(tokRParen)
	if err != nil {
		return nil, err
	}
	if !p.match(tokRParen) {
		return nil, fmt.Errorf("expected ) after for loop")
	}
	body, braced, err := p.parseStatementGroup()
	if err != nil {
		return nil, err
	}
	return &forStmt{init: init, cond: cond, post: post, body: body, endsBlock: braced}, nil
}

func (p *parser) parseOptionalForExpr(end tokenKind) (expr, error) {
	if p.at(end) {
		return nil, nil
	}
	x, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	return x, nil
}

func (p *parser) parseWhile() (stmt, error) {
	p.advance()
	if !p.match(tokLParen) {
		return nil, fmt.Errorf("expected ( after while")
	}
	cond, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if !p.match(tokRParen) {
		return nil, fmt.Errorf("expected ) after while condition")
	}
	body, braced, err := p.parseStatementGroup()
	if err != nil {
		return nil, err
	}
	return &whileStmt{cond: cond, body: body, endsBlock: braced}, nil
}

func (p *parser) parseIf() (stmt, error) {
	p.advance()
	if !p.match(tokLParen) {
		return nil, fmt.Errorf("expected ( after if")
	}
	cond, err := p.parseExpression(0)
	if err != nil {
		return nil, err
	}
	if !p.match(tokRParen) {
		return nil, fmt.Errorf("expected ) after if condition")
	}
	thenStmts, thenBraced, err := p.parseStatementGroup()
	if err != nil {
		return nil, err
	}
	save := p.pos
	canParseElse := thenBraced
	if canParseElse {
		p.skipNewlines()
	} else if p.match(tokSemicolon) {
		canParseElse = true
		p.skipNewlines()
	} else if p.at(tokNewline) {
		canParseElse = true
		p.skipNewlines()
	}
	var elseStmts []stmt
	endsBlock := thenBraced
	if canParseElse && p.atIdent("else") {
		p.advance()
		var elseBraced bool
		elseStmts, elseBraced, err = p.parseStatementGroup()
		if err != nil {
			return nil, err
		}
		endsBlock = elseBraced
	} else {
		p.pos = save
	}
	return &ifStmt{cond: cond, thenStmts: thenStmts, elseStmts: elseStmts, endsBlock: endsBlock}, nil
}

func (p *parser) parseStatementGroup() ([]stmt, bool, error) {
	p.skipNewlines()
	if p.at(tokSemicolon) {
		return nil, false, nil
	}
	if p.match(tokLBrace) {
		stmts, err := p.parseStatementList()
		return stmts, true, err
	}
	st, err := p.parseStatement()
	if err != nil {
		return nil, false, err
	}
	return []stmt{st}, statementEndsBlock(st), nil
}

func (p *parser) parseDelete() (stmt, error) {
	p.advance()
	if p.cur().kind != tokIdent {
		return nil, fmt.Errorf("delete requires an array name")
	}
	name := p.cur().lit
	if err := validateIdentifierReference(name); err != nil {
		return nil, err
	}
	p.advance()
	if !p.match(tokLBracket) {
		return &deleteStmt{name: name, all: true}, nil
	}
	indices, err := p.parseArrayIndices()
	if err != nil {
		return nil, err
	}
	return &deleteStmt{name: name, indices: indices}, nil
}

func (p *parser) parsePrint() (stmt, error) {
	p.advance()
	ps := &printStmt{}
	parenthesizedArgs := p.at(tokLParen)
	if p.at(tokRBrace) || p.at(tokEOF) || isSeparator(p.cur().kind) {
		return ps, nil
	}
	old := p.stopPrintRedirect
	p.stopPrintRedirect = true
	defer func() { p.stopPrintRedirect = old }()
	for {
		x, err := p.parseExpressionWithBareComposite(0, parenthesizedArgs)
		if err != nil {
			return nil, err
		}
		if args, ok := x.(*compositeExpr); parenthesizedArgs && ok {
			ps.args = append(ps.args, args.parts...)
			if p.at(tokComma) {
				return nil, fmt.Errorf("unexpected comma after parenthesized print arguments")
			}
		} else {
			ps.args = append(ps.args, x)
		}
		parenthesizedArgs = false
		if p.at(tokGT) || p.at(tokAppend) {
			return nil, fmt.Errorf("print redirection is not supported")
		}
		if !p.match(tokComma) {
			break
		}
		p.skipNewlines()
	}
	return ps, nil
}

func (p *parser) parsePrintf() (stmt, error) {
	p.advance()
	ps := &printfStmt{}
	parenthesized := p.match(tokLParen)
	if p.at(tokRBrace) || p.at(tokEOF) || isSeparator(p.cur().kind) || p.at(tokRParen) {
		return nil, fmt.Errorf("printf requires a format expression")
	}
	old := p.stopPrintRedirect
	p.stopPrintRedirect = true
	defer func() { p.stopPrintRedirect = old }()
	for {
		x, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		ps.args = append(ps.args, x)
		if p.at(tokGT) || p.at(tokAppend) {
			return nil, fmt.Errorf("print redirection is not supported")
		}
		if parenthesized {
			if p.match(tokRParen) {
				if len(ps.args) == 1 && p.match(tokComma) {
					parenthesized = false
					p.skipNewlines()
					continue
				}
				break
			}
			if !p.match(tokComma) {
				return nil, fmt.Errorf("expected , or ) in printf")
			}
			p.skipNewlines()
			continue
		}
		if !p.match(tokComma) {
			break
		}
		p.skipNewlines()
	}
	if p.at(tokGT) || p.at(tokAppend) {
		return nil, fmt.Errorf("print redirection is not supported")
	}
	return ps, nil
}

func (p *parser) skipNewlines() {
	for p.at(tokNewline) {
		p.advance()
	}
}

func (p *parser) parseExpression(minPrec int) (expr, error) {
	return p.parseExpressionWithBareComposite(minPrec, false)
}

func (p *parser) parseExpressionWithBareComposite(minPrec int, allowBareComposite bool) (expr, error) {
	if err := p.enterNesting(); err != nil {
		return nil, err
	}
	defer p.leaveNesting()

	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}
	for {
		if _, composite := left.(*compositeExpr); composite && (!p.atIdent("in") || precIn < minPrec) {
			if allowBareComposite {
				break
			}
			return nil, fmt.Errorf("composite expression is not valid in this context")
		}
		if p.at(tokQuestion) {
			if precTernary < minPrec {
				break
			}
			p.advance()
			p.skipNewlines()
			thenExpr, err := p.parseExpression(0)
			if err != nil {
				return nil, err
			}
			if !p.match(tokColon) {
				return nil, fmt.Errorf("expected : in conditional expression")
			}
			p.skipNewlines()
			elseExpr, err := p.parseExpression(precAssign)
			if err != nil {
				return nil, err
			}
			left = &ternaryExpr{cond: left, then: thenExpr, els: elseExpr}
			continue
		}
		if p.at(tokInc) || p.at(tokDec) {
			if precPostfix < minPrec {
				break
			}
			if isAssignableExpr(left) {
				op := p.cur().lit
				p.advance()
				left = &incDecExpr{op: op, x: left}
				continue
			}
		}
		if p.stopPrintRedirect && (p.at(tokGT) || p.at(tokAppend)) {
			break
		}
		if p.atIdent("in") {
			if precIn < minPrec {
				break
			}
			p.advance()
			if p.cur().kind != tokIdent {
				return nil, fmt.Errorf("right side of in requires an array variable")
			}
			array := p.cur()
			p.advance()
			if err := validateIdentifierReference(array.lit); err != nil {
				return nil, err
			}
			left = &binaryExpr{op: "in", left: left, right: &varExpr{name: array.lit}}
			continue
		}
		if op, prec, assoc, ok := p.binaryOp(); ok {
			if prec < minPrec {
				break
			}
			p.advance()
			if op == "||" || op == "&&" {
				p.skipNewlines()
			}
			nextMin := prec + 1
			if assoc == "right" {
				nextMin = prec
			}
			right, err := p.parseExpression(nextMin)
			if err != nil {
				return nil, err
			}
			if (op == "/" || op == "%") && isLiteralZeroDivisor(right) {
				if op == "%" {
					return nil, fmt.Errorf("division by zero attempted in `%%'")
				}
				return nil, fmt.Errorf("division by zero attempted")
			}
			if isComparisonOp(op) {
				if b, ok := left.(*binaryExpr); ok && isComparisonOp(b.op) {
					return nil, fmt.Errorf("chained comparisons are not supported")
				}
			}
			if isAssignOp(op) {
				if !isAssignableExpr(left) {
					return nil, fmt.Errorf("assignment requires a variable")
				}
				left = &assignExpr{op: op, left: left, right: right}
			} else {
				left = &binaryExpr{op: op, left: left, right: right}
			}
			continue
		}
		if minPrec <= precConcat && p.canStartConcatenation() {
			right, err := p.parseExpression(precConcat + 1)
			if err != nil {
				return nil, err
			}
			left = &binaryExpr{op: "concat", left: left, right: right}
			continue
		}
		break
	}
	return left, nil
}

func isLiteralZeroDivisor(x expr) bool {
	n, ok := literalNumericExpr(x)
	return ok && n == 0
}

func literalNumericExpr(x expr) (float64, bool) {
	switch e := x.(type) {
	case *numberExpr:
		return e.num, true
	case *unaryExpr:
		n, ok := literalNumericExpr(e.x)
		if !ok {
			return 0, false
		}
		switch e.op {
		case "-":
			return -n, true
		case "!":
			if n != 0 {
				return 0, true
			}
			return 1, true
		}
	}
	return 0, false
}

func (p *parser) parsePrefix() (expr, error) {
	tok := p.cur()
	switch tok.kind {
	case tokNumber:
		p.advance()
		n := parseAwkNumberLiteral(tok.lit)
		return &numberExpr{text: tok.lit, num: n}, nil
	case tokString:
		p.advance()
		return &stringExpr{value: tok.lit}, nil
	case tokRegex:
		p.advance()
		if _, err := compileRegexContext(p.ctx, tok.lit); err != nil {
			return nil, err
		}
		return &regexExpr{pattern: tok.lit}, nil
	case tokIdent:
		p.advance()
		if p.at(tokLParen) && (tokensAdjacent(tok, p.cur()) || isKnownBuiltinFunction(tok.lit)) {
			return p.parseFunctionCall(tok.lit)
		}
		if tok.lit == "length" {
			return &callExpr{name: tok.lit}, nil
		}
		if err := validateIdentifierReference(tok.lit); err != nil {
			return nil, err
		}
		if p.at(tokLBracket) {
			return p.parseArrayRef(tok.lit)
		}
		return &varExpr{name: tok.lit}, nil
	case tokDollar:
		return p.parseFieldRef()
	case tokLParen:
		p.advance()
		old := p.stopPrintRedirect
		p.stopPrintRedirect = false
		x, err := p.parseExpression(0)
		p.stopPrintRedirect = old
		if err != nil {
			return nil, err
		}
		if p.match(tokComma) {
			parts := []expr{x}
			for {
				p.skipNewlines()
				part, err := p.parseExpression(0)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
				if p.match(tokRParen) {
					return &compositeExpr{parts: parts}, nil
				}
				if !p.match(tokComma) {
					return nil, fmt.Errorf("expected , or ) in expression list")
				}
			}
		}
		if !p.match(tokRParen) {
			return nil, fmt.Errorf("expected )")
		}
		return &groupedExpr{x: x}, nil
	case tokPlus, tokMinus, tokBang:
		p.advance()
		x, err := p.parseExpression(precPrefix)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: tok.lit, x: x}, nil
	case tokInc, tokDec:
		p.advance()
		x, err := p.parseExpression(precPostfix + 1)
		if err != nil {
			return nil, err
		}
		if !isAssignableExpr(x) {
			return nil, fmt.Errorf("increment and decrement require variables")
		}
		return &incDecExpr{op: tok.lit, x: x, prefix: true}, nil
	default:
		return nil, fmt.Errorf("expected expression")
	}
}

func tokensAdjacent(left, right token) bool {
	return left.pos+len(left.lit) == right.pos
}

func isKnownBuiltinFunction(name string) bool {
	if name == "system" {
		return true
	}
	if _, ok := supportedBuiltinFunctions[name]; ok {
		return true
	}
	_, ok := unsupportedBuiltinFunctions[name]
	return ok
}

func (p *parser) parseArrayRef(name string) (expr, error) {
	p.advance()
	indices, err := p.parseArrayIndices()
	if err != nil {
		return nil, err
	}
	return &arrayRefExpr{name: name, indices: indices}, nil
}

func (p *parser) parseArrayIndices() ([]expr, error) {
	indices := []expr{}
	for {
		index, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		indices = append(indices, index)
		if p.match(tokRBracket) {
			return indices, nil
		}
		if !p.match(tokComma) {
			return nil, fmt.Errorf("expected , or ] after array index")
		}
		p.skipNewlines()
	}
}

func (p *parser) parseFunctionCall(name string) (expr, error) {
	if msg, ok := unsupportedExpressionKeyword(name); ok {
		return nil, fmt.Errorf("%s", msg)
	}
	if name == "system" {
		return nil, fmt.Errorf("system() is not supported")
	}
	_, supportedBuiltin := supportedBuiltinFunctions[name]
	if _, ok := unsupportedBuiltinFunctions[name]; ok {
		return nil, fmt.Errorf("%s() is not supported", name)
	}
	p.advance()
	args := []expr{}
	if p.match(tokRParen) {
		if supportedBuiltin {
			if err := validateBuiltinCallArity(name, len(args)); err != nil {
				return nil, err
			}
		} else if !validVarName(name) {
			return nil, fmt.Errorf("invalid function name %q", name)
		}
		return &callExpr{name: name}, nil
	}
	for {
		if len(args) >= maxFunctionArguments {
			return nil, fmt.Errorf("function %q has too many arguments (maximum %d)", name, maxFunctionArguments)
		}
		arg, err := p.parseExpression(0)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.match(tokRParen) {
			break
		}
		if !p.match(tokComma) {
			return nil, fmt.Errorf("expected , or ) in function call")
		}
		p.skipNewlines()
	}
	if supportedBuiltin {
		if err := validateBuiltinCallArity(name, len(args)); err != nil {
			return nil, err
		}
		if (name == "sub" || name == "gsub") && len(args) == 3 && !isAssignableExpr(args[2]) {
			return nil, fmt.Errorf("%s third argument must be assignable", name)
		}
	} else if !validVarName(name) {
		return nil, fmt.Errorf("invalid function name %q", name)
	}
	return &callExpr{name: name, args: args}, nil
}

func validateFunctionName(name string) error {
	if !validVarName(name) {
		return fmt.Errorf("invalid function name %q", name)
	}
	if msg, ok := unsupportedExpressionKeyword(name); ok {
		return fmt.Errorf("%s", msg)
	}
	if isContextualAwkKeyword(name) {
		return fmt.Errorf("%s is a reserved awk keyword", name)
	}
	if _, ok := supportedBuiltinFunctions[name]; ok {
		return fmt.Errorf("%q is a built-in function, it cannot be redefined", name)
	}
	if _, ok := unsupportedBuiltinFunctions[name]; ok {
		return fmt.Errorf("%q is a built-in function, it cannot be redefined", name)
	}
	if isReservedAwkVariableName(name) {
		return fmt.Errorf("function name %q uses a reserved awk variable name", name)
	}
	return nil
}

func validateFunctionParameterName(functionName, param string) error {
	if !validVarName(param) {
		return fmt.Errorf("invalid function parameter %q", param)
	}
	if functionName == param {
		return fmt.Errorf("function %q cannot use function name as parameter name", functionName)
	}
	if isReservedAwkVariableName(param) {
		return fmt.Errorf("parameter %q uses a reserved awk variable name", param)
	}
	if _, ok := supportedBuiltinFunctions[param]; ok {
		return fmt.Errorf("parameter %q uses a built-in function name", param)
	}
	if _, ok := unsupportedBuiltinFunctions[param]; ok {
		return fmt.Errorf("parameter %q uses a built-in function name", param)
	}
	if msg, ok := unsupportedExpressionKeyword(param); ok {
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func validateLoopControlStatementsContext(ctx context.Context, prog *program) error {
	for _, r := range prog.rules {
		if err := validateStmtListLoopControl(ctx, r.action, 0, r.kind == ruleNormal); err != nil {
			return err
		}
	}
	for _, fn := range prog.functions {
		if err := validateStmtListLoopControl(ctx, fn.body, 0, true); err != nil {
			return err
		}
	}
	return nil
}

func validateStmtListLoopControl(ctx context.Context, stmts []stmt, loopDepth int, allowNext bool) error {
	for _, st := range stmts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateStmtLoopControl(ctx, st, loopDepth, allowNext); err != nil {
			return err
		}
	}
	return nil
}

func validateStmtLoopControl(ctx context.Context, st stmt, loopDepth int, allowNext bool) error {
	switch s := st.(type) {
	case *ifStmt:
		if err := validateStmtListLoopControl(ctx, s.thenStmts, loopDepth, allowNext); err != nil {
			return err
		}
		return validateStmtListLoopControl(ctx, s.elseStmts, loopDepth, allowNext)
	case *forInStmt:
		return validateStmtListLoopControl(ctx, s.body, loopDepth+1, allowNext)
	case *forStmt:
		return validateStmtListLoopControl(ctx, s.body, loopDepth+1, allowNext)
	case *whileStmt:
		return validateStmtListLoopControl(ctx, s.body, loopDepth+1, allowNext)
	case *breakStmt:
		if loopDepth == 0 {
			return fmt.Errorf("break is not allowed outside a loop")
		}
	case *continueStmt:
		if loopDepth == 0 {
			return fmt.Errorf("continue is not allowed outside a loop")
		}
	case *nextStmt:
		if !allowNext {
			return fmt.Errorf("next is not allowed in BEGIN or END")
		}
	}
	return nil
}

func validateUserFunctionNameReferencesContext(ctx context.Context, prog *program) error {
	for _, r := range prog.rules {
		if err := validateExprUserFunctionNameReferences(ctx, r.pattern, prog.functions, nil); err != nil {
			return err
		}
		if err := validateStmtListUserFunctionNameReferences(ctx, r.action, prog.functions, nil); err != nil {
			return err
		}
	}
	for _, fn := range prog.functions {
		locals := make(map[string]struct{}, len(fn.params))
		for _, param := range fn.params {
			locals[param] = struct{}{}
		}
		if err := validateStmtListUserFunctionNameReferences(ctx, fn.body, prog.functions, locals); err != nil {
			return err
		}
	}
	return nil
}

func validateStmtListUserFunctionNameReferences(ctx context.Context, stmts []stmt, functions map[string]*functionDef, locals map[string]struct{}) error {
	for _, st := range stmts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateStmtUserFunctionNameReferences(ctx, st, functions, locals); err != nil {
			return err
		}
	}
	return nil
}

func validateStmtUserFunctionNameReferences(ctx context.Context, st stmt, functions map[string]*functionDef, locals map[string]struct{}) error {
	switch s := st.(type) {
	case *printStmt:
		return validateExprListUserFunctionNameReferences(ctx, s.args, functions, locals)
	case *printfStmt:
		return validateExprListUserFunctionNameReferences(ctx, s.args, functions, locals)
	case *ifStmt:
		if err := validateExprUserFunctionNameReferences(ctx, s.cond, functions, locals); err != nil {
			return err
		}
		if err := validateStmtListUserFunctionNameReferences(ctx, s.thenStmts, functions, locals); err != nil {
			return err
		}
		return validateStmtListUserFunctionNameReferences(ctx, s.elseStmts, functions, locals)
	case *forInStmt:
		if err := validateNameNotUserFunction(s.varName, functions, locals); err != nil {
			return err
		}
		if err := validateNameNotUserFunction(s.arrayName, functions, locals); err != nil {
			return err
		}
		return validateStmtListUserFunctionNameReferences(ctx, s.body, functions, locals)
	case *forStmt:
		if err := validateExprUserFunctionNameReferences(ctx, s.init, functions, locals); err != nil {
			return err
		}
		if err := validateExprUserFunctionNameReferences(ctx, s.cond, functions, locals); err != nil {
			return err
		}
		if err := validateExprUserFunctionNameReferences(ctx, s.post, functions, locals); err != nil {
			return err
		}
		return validateStmtListUserFunctionNameReferences(ctx, s.body, functions, locals)
	case *whileStmt:
		if err := validateExprUserFunctionNameReferences(ctx, s.cond, functions, locals); err != nil {
			return err
		}
		return validateStmtListUserFunctionNameReferences(ctx, s.body, functions, locals)
	case *exitStmt:
		return validateExprUserFunctionNameReferences(ctx, s.status, functions, locals)
	case *returnStmt:
		return validateExprUserFunctionNameReferences(ctx, s.value, functions, locals)
	case *deleteStmt:
		if err := validateNameNotUserFunction(s.name, functions, locals); err != nil {
			return err
		}
		return validateExprListUserFunctionNameReferences(ctx, s.indices, functions, locals)
	case *exprStmt:
		return validateExprUserFunctionNameReferences(ctx, s.x, functions, locals)
	default:
		return nil
	}
}

func validateExprListUserFunctionNameReferences(ctx context.Context, exprs []expr, functions map[string]*functionDef, locals map[string]struct{}) error {
	for _, x := range exprs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validateExprUserFunctionNameReferences(ctx, x, functions, locals); err != nil {
			return err
		}
	}
	return nil
}

func validateExprUserFunctionNameReferences(ctx context.Context, x expr, functions map[string]*functionDef, locals map[string]struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch e := x.(type) {
	case nil, *numberExpr, *stringExpr, *regexExpr:
		return nil
	case *varExpr:
		return validateNameNotUserFunction(e.name, functions, locals)
	case *arrayRefExpr:
		if err := validateNameNotUserFunction(e.name, functions, locals); err != nil {
			return err
		}
		return validateExprListUserFunctionNameReferences(ctx, e.indices, functions, locals)
	case *compositeExpr:
		return validateExprListUserFunctionNameReferences(ctx, e.parts, functions, locals)
	case *fieldExpr:
		return validateExprUserFunctionNameReferences(ctx, e.index, functions, locals)
	case *groupedExpr:
		return validateExprUserFunctionNameReferences(ctx, e.x, functions, locals)
	case *unaryExpr:
		return validateExprUserFunctionNameReferences(ctx, e.x, functions, locals)
	case *binaryExpr:
		if err := validateExprUserFunctionNameReferences(ctx, e.left, functions, locals); err != nil {
			return err
		}
		return validateExprUserFunctionNameReferences(ctx, e.right, functions, locals)
	case *ternaryExpr:
		if err := validateExprUserFunctionNameReferences(ctx, e.cond, functions, locals); err != nil {
			return err
		}
		if err := validateExprUserFunctionNameReferences(ctx, e.then, functions, locals); err != nil {
			return err
		}
		return validateExprUserFunctionNameReferences(ctx, e.els, functions, locals)
	case *rangeExpr:
		if err := validateExprUserFunctionNameReferences(ctx, e.start, functions, locals); err != nil {
			return err
		}
		return validateExprUserFunctionNameReferences(ctx, e.end, functions, locals)
	case *assignExpr:
		if err := validateExprUserFunctionNameReferences(ctx, e.left, functions, locals); err != nil {
			return err
		}
		return validateExprUserFunctionNameReferences(ctx, e.right, functions, locals)
	case *incDecExpr:
		return validateExprUserFunctionNameReferences(ctx, e.x, functions, locals)
	case *callExpr:
		if _, ok := locals[e.name]; ok {
			return fmt.Errorf("parameter %q cannot be called as a function", e.name)
		}
		if _, builtin := supportedBuiltinFunctions[e.name]; !builtin {
			if _, ok := functions[e.name]; !ok {
				return fmt.Errorf("function %q not defined", e.name)
			}
		}
		return validateExprListUserFunctionNameReferences(ctx, e.args, functions, locals)
	default:
		return nil
	}
}

func validateNameNotUserFunction(name string, functions map[string]*functionDef, locals map[string]struct{}) error {
	if _, ok := locals[name]; ok {
		return nil
	}
	if isContextualAwkKeyword(name) {
		return fmt.Errorf("%s is a reserved awk keyword", name)
	}
	if _, ok := functions[name]; ok {
		return fmt.Errorf("function %q cannot be used as a variable or array", name)
	}
	return nil
}

func validateBuiltinCallArity(name string, argc int) error {
	switch name {
	case "length":
		if argc > 1 {
			return fmt.Errorf("length expects at most 1 argument")
		}
	case "substr":
		if argc != 2 && argc != 3 {
			return fmt.Errorf("substr expects 2 or 3 arguments")
		}
	case "index":
		if argc != 2 {
			return fmt.Errorf("index expects 2 arguments")
		}
	case "split":
		if argc != 2 && argc != 3 {
			return fmt.Errorf("split expects 2 or 3 arguments")
		}
	case "sub", "gsub":
		if argc != 2 && argc != 3 {
			return fmt.Errorf("%s expects 2 or 3 arguments", name)
		}
	case "match":
		if argc != 2 {
			return fmt.Errorf("match expects 2 arguments")
		}
	case "sprintf":
		if argc < 1 {
			return fmt.Errorf("sprintf expects at least 1 argument")
		}
	case "tolower", "toupper", "int":
		if argc != 1 {
			return fmt.Errorf("%s expects 1 argument", name)
		}
	}
	return nil
}

func isAssignableExpr(x expr) bool {
	switch x.(type) {
	case *varExpr, *arrayRefExpr, *fieldExpr:
		return true
	default:
		return false
	}
}

func validateIdentifierReference(name string) error {
	if msg, ok := unsupportedExpressionKeyword(name); ok {
		return fmt.Errorf("%s", msg)
	}
	if name == "system" {
		return fmt.Errorf("system() is not supported")
	}
	if _, ok := supportedBuiltinFunctions[name]; ok {
		return fmt.Errorf("function calls are not supported")
	}
	if _, ok := unsupportedBuiltinFunctions[name]; ok {
		return fmt.Errorf("%s() is not supported", name)
	}
	return nil
}

func unsupportedExpressionKeyword(name string) (string, bool) {
	switch name {
	case "BEGIN", "END":
		return "BEGIN and END are reserved patterns", true
	case "IGNORECASE":
		return "IGNORECASE is not supported", true
	case "if", "while", "for", "next", "nextfile", "exit", "break", "continue", "return", "function":
		return "control flow statements are not supported", true
	case "do", "else", "func", "getline", "in":
		return fmt.Sprintf("%s is a reserved awk keyword", name), true
	case "delete":
		return "arrays are not supported", true
	case "printf":
		return "printf is not supported", true
	case "print":
		return "print is not supported in expressions", true
	default:
		return "", false
	}
}

func isContextualAwkKeyword(name string) bool {
	switch name {
	case "BEGINFILE", "ENDFILE", "switch", "case", "default":
		return true
	default:
		return false
	}
}

func validCommandLineAssignmentName(name string, prog *program) bool {
	if !validIdentifierName(name) {
		return false
	}
	if _, reserved := unsupportedExpressionKeyword(name); reserved {
		return false
	}
	if isContextualAwkKeyword(name) || isCommandOnlyAwkKeyword(name) {
		return false
	}
	if _, builtin := supportedBuiltinFunctions[name]; builtin {
		return false
	}
	if _, builtin := unsupportedBuiltinFunctions[name]; builtin {
		return false
	}
	if prog != nil {
		_, function := prog.functions[name]
		return !function
	}
	return true
}

func isCommandOnlyAwkKeyword(name string) bool {
	switch name {
	case "eval", "include", "load", "namespace":
		return true
	default:
		return false
	}
}

func isComparisonOp(op string) bool {
	switch op {
	case "==", "!=", "<", ">", "<=", ">=":
		return true
	default:
		return false
	}
}

func (p *parser) parseFieldRef() (expr, error) {
	p.advance()
	if p.at(tokIdent) && p.peek(1).kind != tokLParen {
		tok := p.cur()
		p.advance()
		if err := validateIdentifierReference(tok.lit); err != nil {
			return nil, err
		}
		if p.at(tokLBracket) {
			index, err := p.parseArrayRef(tok.lit)
			if err != nil {
				return nil, err
			}
			return &fieldExpr{index: index}, nil
		}
		return &fieldExpr{index: &varExpr{name: tok.lit}}, nil
	}
	switch p.cur().kind {
	case tokNumber, tokString, tokRegex, tokIdent, tokDollar, tokLParen,
		tokPlus, tokMinus, tokBang, tokInc, tokDec:
	default:
		return nil, fmt.Errorf("expected field reference")
	}
	x, err := p.parseExpression(precPostfix + 1)
	if err != nil {
		return nil, err
	}
	return &fieldExpr{index: x}, nil
}

func (p *parser) binaryOp() (string, int, string, bool) {
	switch p.cur().kind {
	case tokAssign:
		return "=", precAssign, "right", true
	case tokPlusAssign:
		return "+=", precAssign, "right", true
	case tokMinusAssign:
		return "-=", precAssign, "right", true
	case tokStarAssign:
		return "*=", precAssign, "right", true
	case tokSlashAssign:
		return "/=", precAssign, "right", true
	case tokPercentAssign:
		return "%=", precAssign, "right", true
	case tokCaretAssign:
		return "^=", precAssign, "right", true
	case tokOr:
		return "||", precOr, "left", true
	case tokAnd:
		return "&&", precAnd, "left", true
	case tokEQ:
		return "==", precCompare, "left", true
	case tokNE:
		return "!=", precCompare, "left", true
	case tokLT:
		return "<", precCompare, "left", true
	case tokGT:
		return ">", precCompare, "left", true
	case tokLE:
		return "<=", precCompare, "left", true
	case tokGE:
		return ">=", precCompare, "left", true
	case tokMatch:
		return "~", precMatch, "left", true
	case tokNotMatch:
		return "!~", precMatch, "left", true
	case tokPlus:
		return "+", precAdd, "left", true
	case tokMinus:
		return "-", precAdd, "left", true
	case tokStar:
		return "*", precMul, "left", true
	case tokSlash:
		return "/", precMul, "left", true
	case tokPercent:
		return "%", precMul, "left", true
	case tokCaret:
		return "^", precPower, "right", true
	default:
		return "", 0, "", false
	}
}

func (p *parser) canStartConcatenation() bool {
	switch p.cur().kind {
	case tokIdent, tokNumber, tokString, tokRegex, tokDollar, tokLParen, tokBang, tokInc, tokDec:
		return true
	default:
		return false
	}
}

func isAssignOp(op string) bool {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=", "^=":
		return true
	default:
		return false
	}
}

func (p *parser) skipSeparators() {
	for isSeparator(p.cur().kind) {
		p.advance()
	}
}

func isSeparator(k tokenKind) bool {
	return k == tokNewline || k == tokSemicolon
}

func (p *parser) cur() token {
	if p.pos >= len(p.toks) {
		return token{kind: tokEOF}
	}
	return p.toks[p.pos]
}

func (p *parser) peek(n int) token {
	idx := p.pos + n
	if idx >= len(p.toks) {
		return token{kind: tokEOF}
	}
	return p.toks[idx]
}

func (p *parser) at(k tokenKind) bool {
	return p.cur().kind == k
}

func (p *parser) atIdent(s string) bool {
	return p.cur().kind == tokIdent && p.cur().lit == s
}

func (p *parser) match(k tokenKind) bool {
	if !p.at(k) {
		return false
	}
	p.advance()
	return true
}

func (p *parser) advance() {
	if p.pos < len(p.toks) {
		p.pos++
	}
}
