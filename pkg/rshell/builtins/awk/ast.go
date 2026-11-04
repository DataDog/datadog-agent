// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

type program struct {
	rules     []rule
	functions map[string]*functionDef
}

type functionDef struct {
	name   string
	params []string
	body   []stmt
}

type ruleKind int

const (
	ruleNormal ruleKind = iota
	ruleBegin
	ruleEnd
)

type rule struct {
	kind    ruleKind
	pattern expr
	action  []stmt
}

type stmt interface {
	stmtNode()
}

type printStmt struct {
	args []expr
}

func (*printStmt) stmtNode() {}

type printfStmt struct {
	args []expr
}

func (*printfStmt) stmtNode() {}

type ifStmt struct {
	cond      expr
	thenStmts []stmt
	elseStmts []stmt
	endsBlock bool
}

func (*ifStmt) stmtNode() {}

type forInStmt struct {
	varName   string
	arrayName string
	body      []stmt
	endsBlock bool
}

func (*forInStmt) stmtNode() {}

type forStmt struct {
	init      expr
	cond      expr
	post      expr
	body      []stmt
	endsBlock bool
}

func (*forStmt) stmtNode() {}

type whileStmt struct {
	cond      expr
	body      []stmt
	endsBlock bool
}

func (*whileStmt) stmtNode() {}

type nextStmt struct{}

func (*nextStmt) stmtNode() {}

type exitStmt struct {
	status expr
}

func (*exitStmt) stmtNode() {}

type returnStmt struct {
	value expr
}

func (*returnStmt) stmtNode() {}

type breakStmt struct{}

func (*breakStmt) stmtNode() {}

type continueStmt struct{}

func (*continueStmt) stmtNode() {}

type deleteStmt struct {
	name    string
	indices []expr
	all     bool
}

func (*deleteStmt) stmtNode() {}

type exprStmt struct {
	x expr
}

func (*exprStmt) stmtNode() {}

type expr interface {
	exprNode()
}

type numberExpr struct {
	text string
	num  float64
}

func (*numberExpr) exprNode() {}

type stringExpr struct {
	value string
}

func (*stringExpr) exprNode() {}

type regexExpr struct {
	pattern string
}

func (*regexExpr) exprNode() {}

type varExpr struct {
	name string
}

func (*varExpr) exprNode() {}

type arrayRefExpr struct {
	name    string
	indices []expr
}

func (*arrayRefExpr) exprNode() {}

type compositeExpr struct {
	parts []expr
}

func (*compositeExpr) exprNode() {}

type fieldExpr struct {
	index expr
}

func (*fieldExpr) exprNode() {}

type groupedExpr struct {
	x expr
}

func (*groupedExpr) exprNode() {}

type unaryExpr struct {
	op string
	x  expr
}

func (*unaryExpr) exprNode() {}

type binaryExpr struct {
	op    string
	left  expr
	right expr
}

func (*binaryExpr) exprNode() {}

type ternaryExpr struct {
	cond expr
	then expr
	els  expr
}

func (*ternaryExpr) exprNode() {}

type rangeExpr struct {
	start expr
	end   expr
}

func (*rangeExpr) exprNode() {}

type assignExpr struct {
	op    string
	left  expr
	right expr
}

func (*assignExpr) exprNode() {}

type incDecExpr struct {
	op     string
	x      expr
	prefix bool
}

func (*incDecExpr) exprNode() {}

type callExpr struct {
	name string
	args []expr
}

func (*callExpr) exprNode() {}
