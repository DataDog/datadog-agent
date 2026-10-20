// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

var errNextRecord = errors.New("next record")
var errBreakLoop = errors.New("break loop")
var errContinueLoop = errors.New("continue loop")

// Cap aggregate match indices and their per-match slice headers.
const maxSubstitutionMatchIndices = 2 * MaxFields

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return "exit"
}

type returnError struct {
	value value
}

func (e *returnError) Error() string {
	return "return"
}

func (rt *runtime) execStatements(ctx context.Context, stmts []stmt) error {
	prevCtx := rt.ctx
	rt.ctx = ctx
	defer func() { rt.ctx = prevCtx }()
	for _, st := range stmts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rt.chargeStatementExecution(); err != nil {
			return err
		}
		switch s := st.(type) {
		case *printStmt:
			out, err := rt.evalPrintArgs(s.args)
			if err != nil {
				return err
			}
			if err := rt.writeStdoutString(out); err != nil {
				return err
			}
		case *printfStmt:
			out, err := rt.evalPrintfArgs(s.args)
			if err != nil {
				return err
			}
			if err := rt.writeStdoutString(out); err != nil {
				return err
			}
		case *ifStmt:
			cond, err := rt.eval(s.cond)
			if err != nil {
				return err
			}
			if cond.Bool() {
				if err := rt.execStatements(ctx, s.thenStmts); err != nil {
					return err
				}
			} else if len(s.elseStmts) > 0 {
				if err := rt.execStatements(ctx, s.elseStmts); err != nil {
					return err
				}
			}
		case *forInStmt:
			keys, err := rt.arrayKeys(s.arrayName)
			if err != nil {
				return err
			}
			for _, key := range keys {
				if err := rt.chargeLoopIteration(); err != nil {
					return err
				}
				if err := rt.setVar(s.varName, stringValue(key)); err != nil {
					return err
				}
				if err := rt.execStatements(ctx, s.body); err != nil {
					if errors.Is(err, errBreakLoop) {
						break
					}
					if errors.Is(err, errContinueLoop) {
						continue
					}
					return err
				}
			}
		case *forStmt:
			if err := rt.execFor(ctx, s); err != nil {
				return err
			}
		case *whileStmt:
			if err := rt.execWhile(ctx, s); err != nil {
				return err
			}
		case *nextStmt:
			return errNextRecord
		case *exitStmt:
			code := rt.exitCode
			if s.status != nil {
				status, err := rt.eval(s.status)
				if err != nil {
					return err
				}
				code = int(status.Number())
			}
			rt.exitCode = code
			return &exitError{code: code}
		case *returnStmt:
			if s.value == nil {
				return &returnError{value: unassignedValue()}
			}
			v, err := rt.eval(s.value)
			if err != nil {
				return err
			}
			return &returnError{value: v}
		case *breakStmt:
			return errBreakLoop
		case *continueStmt:
			return errContinueLoop
		case *deleteStmt:
			if s.all {
				if err := rt.deleteArray(s.name); err != nil {
					return err
				}
				continue
			}
			key, err := rt.evalArrayKey(s.indices)
			if err != nil {
				return err
			}
			if err := rt.deleteArrayElem(s.name, key); err != nil {
				return err
			}
		case *exprStmt:
			if _, err := rt.eval(s.x); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown statement")
		}
	}
	return nil
}

func (rt *runtime) execFor(ctx context.Context, s *forStmt) error {
	if s.init != nil {
		if _, err := rt.eval(s.init); err != nil {
			return err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.cond != nil {
			cond, err := rt.eval(s.cond)
			if err != nil {
				return err
			}
			if !cond.Bool() {
				return nil
			}
		}
		if err := rt.chargeLoopIteration(); err != nil {
			return err
		}
		err := rt.execStatements(ctx, s.body)
		if errors.Is(err, errBreakLoop) {
			return nil
		}
		if err != nil && !errors.Is(err, errContinueLoop) {
			return err
		}
		if s.post != nil {
			if _, postErr := rt.eval(s.post); postErr != nil {
				return postErr
			}
		}
	}
}

func (rt *runtime) execWhile(ctx context.Context, s *whileStmt) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		cond, err := rt.eval(s.cond)
		if err != nil {
			return err
		}
		if !cond.Bool() {
			return nil
		}
		if err := rt.chargeLoopIteration(); err != nil {
			return err
		}
		err = rt.execStatements(ctx, s.body)
		if errors.Is(err, errBreakLoop) {
			return nil
		}
		if errors.Is(err, errContinueLoop) {
			continue
		}
		if err != nil {
			return err
		}
	}
}

func (rt *runtime) chargeLoopIteration() error {
	if rt.loopIterations >= maxLoopIterations {
		return fmt.Errorf("loop iteration limit exceeded (maximum %d)", maxLoopIterations)
	}
	rt.loopIterations++
	return nil
}

func (rt *runtime) chargeStatementExecution() error {
	if rt.stmtExecutions >= maxStatementExecutions {
		return fmt.Errorf("statement execution limit exceeded (maximum %d)", maxStatementExecutions)
	}
	rt.stmtExecutions++
	return nil
}

func substrStart(n float64, length int) int {
	n = math.Trunc(n)
	if n <= 1 || math.IsNaN(n) {
		return 0
	}
	if n > float64(length) {
		return length
	}
	return int(n) - 1
}

func substrEnd(start, length int, count float64) int {
	if count <= 0 || math.IsNaN(count) {
		return start
	}
	if count >= float64(length-start) {
		return length
	}
	return start + int(count)
}

type callArgumentBudget struct {
	rt    *runtime
	bytes int
}

func (b *callArgumentBudget) retain(v value) error {
	size := len(v.s)
	if size > MaxVariableBytes-b.rt.callArgBytes {
		return fmt.Errorf("function argument storage limit exceeded (maximum %d bytes)", MaxVariableBytes)
	}
	b.rt.callArgBytes += size
	b.bytes += size
	return nil
}

func (b *callArgumentBudget) convfmtString(v value) (string, error) {
	s, err := b.rt.conversionString(v, "CONVFMT")
	if err != nil {
		return "", err
	}
	if v.kind == valueNumber {
		if err := b.retain(stringValue(s)); err != nil {
			return "", err
		}
	}
	return s, nil
}

func (b *callArgumentBudget) release() {
	b.rt.callArgBytes -= b.bytes
	if b.rt.callArgBytes < 0 {
		b.rt.callArgBytes = 0
	}
	b.bytes = 0
}

type expressionTemporaryBudget struct {
	rt    *runtime
	bytes int
}

func expressionTemporaryLimitError() error {
	return fmt.Errorf("expression temporary storage exceeds %d bytes", MaxExpressionBytes)
}

func (b *expressionTemporaryBudget) retainValue(v value) error {
	if v.kind == valueNumber {
		return nil
	}
	return b.retainString(v.s)
}

func (b *expressionTemporaryBudget) retainString(s string) error {
	size := len(s)
	if size > MaxExpressionBytes-b.rt.exprTempBytes {
		return expressionTemporaryLimitError()
	}
	b.rt.exprTempBytes += size
	b.bytes += size
	return nil
}

func (b *expressionTemporaryBudget) convfmtString(v value) (string, error) {
	s, err := b.rt.conversionString(v, "CONVFMT")
	if err != nil {
		return "", err
	}
	if v.kind == valueNumber {
		if err := b.retainString(s); err != nil {
			return "", err
		}
	}
	return s, nil
}

func (b *expressionTemporaryBudget) release() {
	b.rt.exprTempBytes -= b.bytes
	if b.rt.exprTempBytes < 0 {
		b.rt.exprTempBytes = 0
	}
	b.bytes = 0
}

func (rt *runtime) evalPrintArgs(args []expr) (string, error) {
	if len(args) == 0 {
		v := rt.field(0)
		if err := rt.chargeStringValue(v); err != nil {
			return "", err
		}
		out, err := rt.formatPrintValues([]value{v})
		if err != nil {
			return "", err
		}
		if err := rt.chargeStringProcessing(len(out)); err != nil {
			return "", err
		}
		return out, nil
	}
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	vals := make([]value, 0, len(args))
	for _, arg := range args {
		v, err := rt.eval(arg)
		if err != nil {
			return "", err
		}
		if err := budget.retain(v); err != nil {
			return "", fmt.Errorf("print output exceeds %d bytes", MaxVariableBytes)
		}
		vals = append(vals, v)
	}
	out, err := rt.formatPrintValues(vals)
	if err != nil {
		return "", err
	}
	if err := rt.chargeStringProcessing(len(out)); err != nil {
		return "", err
	}
	return out, nil
}

func (rt *runtime) evalPrintfArgs(args []expr) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("printf requires a format expression")
	}
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	vals := make([]value, 0, len(args))
	for _, arg := range args {
		v, err := rt.eval(arg)
		if err != nil {
			return "", err
		}
		if err := budget.retain(v); err != nil {
			return "", err
		}
		vals = append(vals, v)
	}
	format, err := budget.convfmtString(vals[0])
	if err != nil {
		return "", err
	}
	out, err := rt.formatPrintf(format, vals[1:])
	if err != nil {
		return "", err
	}
	if err := rt.chargeStringProcessing(len(out)); err != nil {
		return "", err
	}
	return out, nil
}

func (rt *runtime) formatPrintValues(vals []value) (string, error) {
	var b strings.Builder
	ofs := rt.getVar("OFS").String()
	for i, v := range vals {
		if i > 0 {
			if err := appendPrintString(&b, ofs); err != nil {
				return "", err
			}
		}
		display, err := rt.conversionString(v, "OFMT")
		if err != nil {
			return "", err
		}
		if err := appendPrintString(&b, display); err != nil {
			return "", err
		}
	}
	if err := appendPrintString(&b, rt.getVar("ORS").String()); err != nil {
		return "", err
	}
	return b.String(), nil
}

func appendPrintString(b *strings.Builder, s string) error {
	if len(s) > MaxVariableBytes-b.Len() {
		return fmt.Errorf("print output exceeds %d bytes", MaxVariableBytes)
	}
	b.WriteString(s)
	return nil
}

func (rt *runtime) eval(x expr) (value, error) {
	if rt.ctx != nil {
		if err := rt.ctx.Err(); err != nil {
			return value{}, err
		}
	}
	if rt.exprEvaluations >= maxExpressionEvaluations {
		return value{}, fmt.Errorf("expression evaluation limit exceeded (maximum %d)", maxExpressionEvaluations)
	}
	rt.exprEvaluations++
	v, err := rt.evalNode(x)
	if err != nil {
		return value{}, err
	}
	if err := rt.chargeStringValue(v); err != nil {
		return value{}, err
	}
	return v, nil
}

func (rt *runtime) evalNode(x expr) (value, error) {
	switch e := x.(type) {
	case *numberExpr:
		return numberValue(e.num), nil
	case *stringExpr:
		return stringValue(e.value), nil
	case *regexExpr:
		if err := rt.chargeStringProcessing(len(rt.record)); err != nil {
			return value{}, err
		}
		re, err := rt.compileRegex(e.pattern)
		if err != nil {
			return value{}, err
		}
		matched, err := re.MatchString(rt.record)
		if err != nil {
			return value{}, err
		}
		return boolValue(matched), nil
	case *varExpr:
		if rt.isArray(e.name) || isBuiltinArrayName(e.name) {
			return value{}, fmt.Errorf("fatal: cannot use array %s as scalar", e.name)
		}
		return rt.getVar(e.name), nil
	case *arrayRefExpr:
		return rt.evalArrayRef(e)
	case *compositeExpr:
		key, err := rt.evalArrayKey(e.parts)
		if err != nil {
			return value{}, err
		}
		return stringValue(key), nil
	case *fieldExpr:
		v, err := rt.eval(e.index)
		if err != nil {
			return value{}, err
		}
		n := int(v.Number())
		if n < 0 {
			return value{}, fmt.Errorf("fatal: invalid field index")
		}
		return rt.field(n), nil
	case *groupedExpr:
		return rt.eval(e.x)
	case *unaryExpr:
		v, err := rt.eval(e.x)
		if err != nil {
			return value{}, err
		}
		switch e.op {
		case "+":
			return numberValue(v.Number()), nil
		case "-":
			return numberValue(-v.Number()), nil
		case "!":
			if v.Bool() {
				return numberValue(0), nil
			}
			return numberValue(1), nil
		default:
			return value{}, fmt.Errorf("unknown unary operator %s", e.op)
		}
	case *binaryExpr:
		return rt.evalBinary(e)
	case *ternaryExpr:
		cond, err := rt.eval(e.cond)
		if err != nil {
			return value{}, err
		}
		if cond.Bool() {
			return rt.eval(e.then)
		}
		return rt.eval(e.els)
	case *assignExpr:
		return rt.evalAssign(e)
	case *incDecExpr:
		return rt.evalIncDec(e)
	case *callExpr:
		return rt.evalCall(e)
	default:
		return value{}, fmt.Errorf("unknown expression")
	}
}

func (rt *runtime) chargeStringValue(v value) error {
	if v.kind == valueNumber {
		return nil
	}
	return rt.chargeStringProcessing(len(v.s))
}

func (rt *runtime) chargeStringProcessing(size int) error {
	if size > maxStringProcessingBytes-rt.stringWorkBytes {
		return fmt.Errorf("string processing limit exceeded (maximum %d bytes)", maxStringProcessingBytes)
	}
	rt.stringWorkBytes += size
	return nil
}

func (rt *runtime) evalCall(e *callExpr) (value, error) {
	if fn, ok := rt.prog.functions[e.name]; ok {
		return rt.evalUserFunction(fn, e.args)
	}
	if e.name == "split" {
		return rt.evalSplit(e)
	}
	if e.name == "sub" || e.name == "gsub" {
		return rt.evalSubstitution(e)
	}
	if e.name == "match" {
		return rt.evalMatch(e)
	}
	if e.name == "length" {
		return rt.evalLength(e)
	}
	if _, ok := supportedBuiltinFunctions[e.name]; !ok {
		if _, unsupported := unsupportedBuiltinFunctions[e.name]; unsupported {
			return value{}, fmt.Errorf("function calls are not supported")
		}
		return value{}, fmt.Errorf("function %q not defined", e.name)
	}
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	args := make([]value, 0, len(e.args))
	for _, arg := range e.args {
		v, err := rt.eval(arg)
		if err != nil {
			return value{}, err
		}
		if err := budget.retain(v); err != nil {
			return value{}, err
		}
		args = append(args, v)
	}
	switch e.name {
	case "substr":
		s, err := budget.convfmtString(args[0])
		if err != nil {
			return value{}, err
		}
		length := runeLen(s)
		start := substrStart(args[1].Number(), length)
		if start >= length {
			return stringValue(""), nil
		}
		end := length
		if len(args) == 3 {
			end = substrEnd(start, length, args[2].Number())
		}
		startByte, endByte := 0, len(s)
		runeIndex := 0
		for byteIndex := range s {
			if runeIndex == start {
				startByte = byteIndex
			}
			if runeIndex == end {
				endByte = byteIndex
				break
			}
			runeIndex++
		}
		return stringValue(cloneStoredString(s[startByte:endByte])), nil
	case "index":
		haystack, err := budget.convfmtString(args[0])
		if err != nil {
			return value{}, err
		}
		needle, err := budget.convfmtString(args[1])
		if err != nil {
			return value{}, err
		}
		pos, err := rt.indexString(haystack, needle)
		if err != nil {
			return value{}, err
		}
		return numberValue(float64(pos)), nil
	case "tolower":
		s, err := budget.convfmtString(args[0])
		if err != nil {
			return value{}, err
		}
		return stringValue(strings.ToLower(s)), nil
	case "toupper":
		s, err := budget.convfmtString(args[0])
		if err != nil {
			return value{}, err
		}
		return stringValue(strings.ToUpper(s)), nil
	case "int":
		v := args[0]
		return numberValue(math.Trunc(v.Number())), nil
	case "sprintf":
		format, err := budget.convfmtString(args[0])
		if err != nil {
			return value{}, err
		}
		out, err := rt.formatPrintf(format, args[1:])
		if err != nil {
			return value{}, err
		}
		return stringValue(out), nil
	default:
		return value{}, fmt.Errorf("function %q not defined", e.name)
	}
}

func (rt *runtime) evalLength(e *callExpr) (value, error) {
	if len(e.args) == 0 {
		s := rt.field(0).String()
		if err := rt.chargeStringProcessing(len(s)); err != nil {
			return value{}, err
		}
		return numberValue(float64(len([]rune(s)))), nil
	}
	if arg, ok := e.args[0].(*varExpr); ok {
		if err := rt.ensureBuiltinArray(arg.name); err != nil {
			return value{}, err
		}
		if rt.isArray(arg.name) {
			length, err := rt.arrayLen(arg.name)
			if err != nil {
				return value{}, err
			}
			return numberValue(float64(length)), nil
		}
	}
	v, err := rt.eval(e.args[0])
	if err != nil {
		return value{}, err
	}
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	if err := budget.retain(v); err != nil {
		return value{}, err
	}
	s, err := budget.convfmtString(v)
	if err != nil {
		return value{}, err
	}
	return numberValue(float64(len([]rune(s)))), nil
}

type functionArg struct {
	value           value
	valueSet        bool
	arrayAlias      *localVar
	globalArrayName string
}

func (rt *runtime) evalUserFunction(fn *functionDef, args []expr) (value, error) {
	if rt.functionDepth >= maxFunctionDepth {
		return value{}, fmt.Errorf("function call depth limit exceeded (maximum %d)", maxFunctionDepth)
	}
	if rt.functionCalls >= maxFunctionCalls {
		return value{}, fmt.Errorf("function call limit exceeded (maximum %d)", maxFunctionCalls)
	}
	rt.functionCalls++
	rt.functionDepth++
	defer func() {
		rt.functionDepth--
	}()
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	callArgs := make([]functionArg, len(args))
	for i, arg := range args {
		v, err := rt.evalFunctionArg(arg)
		if err != nil {
			return value{}, err
		}
		if v.valueSet {
			if err := budget.retain(v.value); err != nil {
				return value{}, err
			}
		}
		callArgs[i] = v
	}
	frame := callFrame{locals: make(map[string]*localVar, len(fn.params))}
	for _, param := range fn.params {
		frame.locals[param] = &localVar{}
	}
	globalAliases := make(map[string]*localVar)
	rt.frames = append(rt.frames, frame)
	defer rt.popFrame()
	for i, arg := range callArgs[:min(len(callArgs), len(fn.params))] {
		local := rt.lookupLocal(fn.params[i])
		local.arrayAlias = arg.arrayAlias
		if arg.globalArrayName != "" {
			alias := globalAliases[arg.globalArrayName]
			if alias == nil {
				alias = &localVar{globalArrayName: arg.globalArrayName}
				globalAliases[arg.globalArrayName] = alias
			}
			local.arrayAlias = alias
		}
		if arg.valueSet {
			if err := rt.setLocalScalar(local, arg.value); err != nil {
				return value{}, err
			}
		}
	}
	callArgs = nil
	budget.release()
	if rt.ctx == nil {
		return value{}, fmt.Errorf("missing evaluation context")
	}
	err := rt.execStatements(rt.ctx, fn.body)
	if ret, ok := err.(*returnError); ok {
		return ret.value, nil
	}
	if err != nil {
		return value{}, err
	}
	return unassignedValue(), nil
}

func (rt *runtime) evalFunctionArg(arg expr) (functionArg, error) {
	if v, ok := arg.(*varExpr); ok {
		return rt.evalVariableFunctionArg(v.name)
	}
	value, err := rt.eval(arg)
	if err != nil {
		return functionArg{}, err
	}
	return functionArg{value: value, valueSet: true}, nil
}

func (rt *runtime) evalVariableFunctionArg(name string) (functionArg, error) {
	if local := rt.lookupLocal(name); local != nil {
		arg := functionArg{}
		if local.valueSet {
			if err := rt.chargeStringValue(local.value); err != nil {
				return functionArg{}, err
			}
			arg.value = local.value
			arg.valueSet = true
		}
		root := rootLocalVar(local)
		if rt.localIsArray(root) || !local.valueSet {
			arg.arrayAlias = root
		}
		return arg, nil
	}
	if rt.isGlobalArray(name) {
		return functionArg{globalArrayName: name}, nil
	}
	if v, ok := rt.vars[name]; ok {
		if err := rt.chargeStringValue(v); err != nil {
			return functionArg{}, err
		}
		return functionArg{value: v, valueSet: true}, nil
	}
	if isBuiltinArrayName(name) {
		return functionArg{globalArrayName: name}, nil
	}
	if isBuiltinScalarName(name) {
		v := rt.getVar(name)
		if err := rt.chargeStringValue(v); err != nil {
			return functionArg{}, err
		}
		return functionArg{value: v, valueSet: true}, nil
	}
	return functionArg{globalArrayName: name}, nil
}

func (rt *runtime) popFrame() {
	frame := rt.frames[len(rt.frames)-1]
	rt.frames = rt.frames[:len(rt.frames)-1]
	for _, local := range frame.locals {
		rt.varBytes -= local.valueSize
		if local.arrayAlias != nil || local.globalArrayName != "" {
			continue
		}
		for _, size := range local.arraySizes {
			rt.varBytes -= size
		}
	}
	if rt.varBytes < 0 {
		rt.varBytes = 0
	}
}

func (rt *runtime) evalSubstitution(e *callExpr) (value, error) {
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	pattern, err := rt.evalRegexPatternArg(e.args[0], &budget)
	if err != nil {
		return value{}, err
	}
	repl, err := rt.eval(e.args[1])
	if err != nil {
		return value{}, err
	}
	if err := budget.retain(repl); err != nil {
		return value{}, err
	}
	var target assignTarget
	var current value
	if len(e.args) == 3 {
		target, current, err = rt.resolveAssignable(e.args[2])
		if err != nil {
			return value{}, err
		}
	} else {
		target = assignTarget{field: true, fieldIndex: 0}
		current = rt.field(0)
	}
	if err := rt.chargeStringValue(current); err != nil {
		return value{}, err
	}
	re, err := rt.compileRegex(pattern)
	if err != nil {
		return value{}, err
	}
	currentString, err := budget.convfmtString(current)
	if err != nil {
		return value{}, err
	}
	replacement, err := budget.convfmtString(repl)
	if err != nil {
		return value{}, err
	}
	next, count, err := substituteAwk(re, currentString, replacement, e.name == "gsub")
	if err != nil {
		return value{}, err
	}
	if count == 0 {
		return numberValue(0), nil
	}
	if err := rt.chargeStringProcessing(len(next)); err != nil {
		return value{}, err
	}
	if err := rt.setResolvedAssignable(target, stringValue(next)); err != nil {
		return value{}, err
	}
	return numberValue(float64(count)), nil
}

func (rt *runtime) evalMatch(e *callExpr) (value, error) {
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	input, err := rt.eval(e.args[0])
	if err != nil {
		return value{}, err
	}
	if err := budget.retain(input); err != nil {
		return value{}, err
	}
	pattern, err := rt.evalRegexPatternArg(e.args[1], &budget)
	if err != nil {
		return value{}, err
	}
	re, err := rt.compileRegex(pattern)
	if err != nil {
		return value{}, err
	}
	text, err := budget.convfmtString(input)
	if err != nil {
		return value{}, err
	}
	match, err := re.FindStringIndex(text)
	if err != nil {
		return value{}, err
	}
	if match == nil {
		if err := rt.setVar("RSTART", numberValue(0)); err != nil {
			return value{}, err
		}
		if err := rt.setVar("RLENGTH", numberValue(-1)); err != nil {
			return value{}, err
		}
		return numberValue(0), nil
	}
	start, length := awkMatchPosition(text, match[0], match[1])
	if err := rt.setVar("RSTART", numberValue(float64(start))); err != nil {
		return value{}, err
	}
	if err := rt.setVar("RLENGTH", numberValue(float64(length))); err != nil {
		return value{}, err
	}
	return numberValue(float64(start)), nil
}

func awkMatchPosition(text string, startByte, endByte int) (int, int) {
	start, end := runeRangeForByteRange(text, startByte, endByte)
	return start + 1, end - start
}

func (rt *runtime) evalRegexPatternArg(x expr, budget *callArgumentBudget) (string, error) {
	if rx, ok := x.(*regexExpr); ok {
		return rx.pattern, nil
	}
	v, err := rt.eval(x)
	if err != nil {
		return "", err
	}
	if err := budget.retain(v); err != nil {
		return "", err
	}
	return budget.convfmtString(v)
}

func substituteAwk(re *awkRegex, input, replacement string, all bool) (string, int, error) {
	var matches [][]int
	if all {
		matchLimit := maxSubstitutionMatchIndices / 2
		var err error
		matches, err = re.FindAllStringIndex(input, matchLimit+1)
		if err != nil {
			return "", 0, err
		}
		if len(matches) > matchLimit {
			return "", 0, fmt.Errorf("substitution match index storage exceeds %d indices", maxSubstitutionMatchIndices)
		}
	} else {
		loc, err := re.FindStringIndex(input)
		if err != nil {
			return "", 0, err
		}
		if loc != nil {
			matches = [][]int{loc}
		}
	}
	if len(matches) == 0 {
		return input, 0, nil
	}

	var b strings.Builder
	last := 0
	for _, loc := range matches {
		start := loc[0]
		end := loc[1]
		if err := appendLimitedString(&b, input[last:start]); err != nil {
			return "", 0, err
		}
		if err := appendAwkReplacement(&b, replacement, input[start:end]); err != nil {
			return "", 0, err
		}
		last = end
	}
	if err := appendLimitedString(&b, input[last:]); err != nil {
		return "", 0, err
	}
	return b.String(), len(matches), nil
}

func appendAwkReplacement(b *strings.Builder, replacement, matched string) error {
	for i := 0; i < len(replacement); i++ {
		switch replacement[i] {
		case '&':
			if err := appendLimitedString(b, matched); err != nil {
				return err
			}
		case '\\':
			start := i
			for i < len(replacement) && replacement[i] == '\\' {
				i++
			}
			backslashes := i - start
			if i >= len(replacement) || replacement[i] != '&' {
				if err := appendLimitedString(b, replacement[start:i]); err != nil {
					return err
				}
				i--
				continue
			}
			if err := appendLimitedString(b, replacement[start:start+backslashes/2]); err != nil {
				return err
			}
			if backslashes%2 == 0 {
				if err := appendLimitedString(b, matched); err != nil {
					return err
				}
			} else if err := appendLimitedString(b, "&"); err != nil {
				return err
			}
		default:
			if err := appendLimitedString(b, replacement[i:i+1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseAwkNumberLiteral(s string) float64 {
	if n, ok := parseAwkFloat(s); ok {
		return n
	}
	return 0
}

func digitValue(ch byte) (int, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0'), true
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10, true
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10, true
	default:
		return 0, false
	}
}

func appendLimitedString(b *strings.Builder, s string) error {
	if len(s) > MaxVariableBytes-b.Len() {
		return fmt.Errorf("replacement output exceeds %d bytes", MaxVariableBytes)
	}
	b.WriteString(s)
	return nil
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func (rt *runtime) evalSplit(e *callExpr) (value, error) {
	budget := callArgumentBudget{rt: rt}
	defer budget.release()
	target, ok := e.args[1].(*varExpr)
	if !ok {
		return value{}, fmt.Errorf("split destination must be an array variable")
	}
	input, err := rt.eval(e.args[0])
	if err != nil {
		return value{}, err
	}
	if err := budget.retain(input); err != nil {
		return value{}, err
	}
	sep := rt.getVar("FS").String()
	charSplit := false
	regexSplit := false
	if len(e.args) == 3 {
		if rx, ok := e.args[2].(*regexExpr); ok {
			sep = rx.pattern
			regexSplit = true
		} else {
			sepValue, err := rt.eval(e.args[2])
			if err != nil {
				return value{}, err
			}
			if err := budget.retain(sepValue); err != nil {
				return value{}, err
			}
			sep, err = budget.convfmtString(sepValue)
			if err != nil {
				return value{}, err
			}
			charSplit = sep == ""
		}
	}
	inputString, err := budget.convfmtString(input)
	if err != nil {
		return value{}, err
	}
	var parts []string
	if charSplit {
		parts, err = splitAwkChars(inputString)
	} else if regexSplit {
		parts, err = rt.splitAwkRegex(inputString, sep)
	} else {
		parts, err = rt.splitAwkFields(inputString, sep)
	}
	if err != nil {
		if errors.Is(err, errTooManyFields) {
			return value{}, fmt.Errorf("split result exceeds %d fields", MaxFields)
		}
		return value{}, err
	}
	if len(parts) > MaxFields {
		return value{}, fmt.Errorf("split result exceeds %d fields", MaxFields)
	}
	elems := make(map[string]value, len(parts))
	for i, part := range parts {
		if rt.ctx != nil && i%256 == 0 {
			if err := rt.ctx.Err(); err != nil {
				return value{}, err
			}
		}
		elems[fmt.Sprintf("%d", i+1)] = inputStringValue(part)
	}
	if err := rt.replaceArray(target.name, elems); err != nil {
		return value{}, err
	}
	return numberValue(float64(len(parts))), nil
}

func (rt *runtime) evalBinary(e *binaryExpr) (value, error) {
	if e.op == "&&" {
		left, err := rt.eval(e.left)
		if err != nil {
			return value{}, err
		}
		if !left.Bool() {
			return numberValue(0), nil
		}
		right, err := rt.eval(e.right)
		if err != nil {
			return value{}, err
		}
		if right.Bool() {
			return numberValue(1), nil
		}
		return numberValue(0), nil
	}
	if e.op == "||" {
		left, err := rt.eval(e.left)
		if err != nil {
			return value{}, err
		}
		if left.Bool() {
			return numberValue(1), nil
		}
		right, err := rt.eval(e.right)
		if err != nil {
			return value{}, err
		}
		if right.Bool() {
			return numberValue(1), nil
		}
		return numberValue(0), nil
	}
	left, err := rt.eval(e.left)
	if err != nil {
		return value{}, err
	}
	switch e.op {
	case "~", "!~":
		budget := expressionTemporaryBudget{rt: rt}
		defer budget.release()
		if _, literal := e.right.(*regexExpr); !literal {
			if err := budget.retainValue(left); err != nil {
				return value{}, err
			}
		}
		matched, err := rt.matchRegexExpr(left, e.right, &budget)
		if err != nil {
			return value{}, err
		}
		if e.op == "!~" {
			matched = !matched
		}
		return boolValue(matched), nil
	case "in":
		arrayName, ok := e.right.(*varExpr)
		if !ok {
			return value{}, fmt.Errorf("right side of in requires an array variable")
		}
		key, err := rt.conversionString(left, "CONVFMT")
		if err != nil {
			return value{}, err
		}
		ok, err = rt.hasArrayElem(arrayName.name, key)
		if err != nil {
			return value{}, err
		}
		return boolValue(ok), nil
	}
	budget := expressionTemporaryBudget{rt: rt}
	if err := budget.retainValue(left); err != nil {
		return value{}, err
	}
	defer budget.release()
	right, err := rt.eval(e.right)
	if err != nil {
		return value{}, err
	}
	var concatLeft, concatRight string
	if e.op == "concat" {
		concatLeft, err = rt.conversionString(left, "CONVFMT")
		if err != nil {
			return value{}, err
		}
		concatRight, err = rt.conversionString(right, "CONVFMT")
		if err != nil {
			return value{}, err
		}
		if len(concatRight) > MaxExpressionBytes-len(concatLeft) {
			return value{}, fmt.Errorf("string expression exceeds %d bytes", MaxExpressionBytes)
		}
	}
	if err := budget.retainValue(right); err != nil {
		return value{}, err
	}
	switch e.op {
	case "concat":
		return stringValue(concatLeft + concatRight), nil
	case "+":
		return numberValue(left.Number() + right.Number()), nil
	case "-":
		return numberValue(left.Number() - right.Number()), nil
	case "*":
		return numberValue(left.Number() * right.Number()), nil
	case "/":
		if right.Number() == 0 {
			return value{}, fmt.Errorf("fatal: division by zero attempted")
		}
		return numberValue(left.Number() / right.Number()), nil
	case "%":
		if right.Number() == 0 {
			return value{}, fmt.Errorf("fatal: division by zero attempted")
		}
		return numberValue(math.Mod(left.Number(), right.Number())), nil
	case "^":
		return numberValue(math.Pow(left.Number(), right.Number())), nil
	case "==", "!=", "<", "<=", ">", ">=":
		result, err := rt.compareValues(left, right, e.op, &budget)
		if err != nil {
			return value{}, err
		}
		return boolValue(result), nil
	default:
		return value{}, fmt.Errorf("unknown binary operator %s", e.op)
	}
}

func (rt *runtime) matchRegexExpr(left value, rightExpr expr, budget *expressionTemporaryBudget) (bool, error) {
	leftString, err := budget.convfmtString(left)
	if err != nil {
		return false, err
	}
	if rx, ok := rightExpr.(*regexExpr); ok {
		re, err := rt.compileRegex(rx.pattern)
		if err != nil {
			return false, err
		}
		return re.MatchString(leftString)
	}
	right, err := rt.eval(rightExpr)
	if err != nil {
		return false, err
	}
	if err := budget.retainValue(right); err != nil {
		return false, err
	}
	rightString, err := budget.convfmtString(right)
	if err != nil {
		return false, err
	}
	re, err := rt.compileRegex(rightString)
	if err != nil {
		return false, err
	}
	return re.MatchString(leftString)
}

func (rt *runtime) evalAssign(e *assignExpr) (value, error) {
	budget := expressionTemporaryBudget{rt: rt}
	defer budget.release()
	right, err := rt.eval(e.right)
	if err != nil {
		return value{}, err
	}
	if err := budget.retainValue(right); err != nil {
		return value{}, err
	}
	target, left, err := rt.resolveAssignable(e.left)
	if err != nil {
		return value{}, err
	}
	if err := budget.retainString(target.key); err != nil {
		return value{}, err
	}
	if e.op != "=" {
		if err := rt.chargeStringValue(left); err != nil {
			return value{}, err
		}
		switch e.op {
		case "+=":
			right = numberValue(left.Number() + right.Number())
		case "-=":
			right = numberValue(left.Number() - right.Number())
		case "*=":
			right = numberValue(left.Number() * right.Number())
		case "/=":
			if right.Number() == 0 {
				return value{}, fmt.Errorf("fatal: division by zero attempted")
			}
			right = numberValue(left.Number() / right.Number())
		case "%=":
			if right.Number() == 0 {
				return value{}, fmt.Errorf("fatal: division by zero attempted")
			}
			right = numberValue(math.Mod(left.Number(), right.Number()))
		case "^=":
			right = numberValue(math.Pow(left.Number(), right.Number()))
		default:
			return value{}, fmt.Errorf("unknown assignment operator %s", e.op)
		}
	}
	if err := rt.setResolvedAssignable(target, right); err != nil {
		return value{}, err
	}
	return right, nil
}

func (rt *runtime) evalIncDec(e *incDecExpr) (value, error) {
	target, old, err := rt.resolveAssignable(e.x)
	if err != nil {
		return value{}, err
	}
	if err := rt.chargeStringValue(old); err != nil {
		return value{}, err
	}
	oldNumber := old.Number()
	next := oldNumber
	if e.op == "++" {
		next++
	} else {
		next--
	}
	nv := numberValue(next)
	if err := rt.setResolvedAssignable(target, nv); err != nil {
		return value{}, err
	}
	if e.prefix {
		return nv, nil
	}
	return numberValue(oldNumber), nil
}

type assignTarget struct {
	name       string
	key        string
	array      bool
	field      bool
	fieldIndex int
}

func (rt *runtime) resolveAssignable(x expr) (assignTarget, value, error) {
	switch v := x.(type) {
	case *varExpr:
		if rt.isArray(v.name) {
			return assignTarget{}, value{}, fmt.Errorf("fatal: cannot use array %s as scalar", v.name)
		}
		return assignTarget{name: v.name}, rt.getVar(v.name), nil
	case *arrayRefExpr:
		keyString, err := rt.evalArrayKey(v.indices)
		if err != nil {
			return assignTarget{}, value{}, err
		}
		current, err := rt.getArrayElem(v.name, keyString)
		if err != nil {
			return assignTarget{}, value{}, err
		}
		return assignTarget{name: v.name, key: keyString, array: true}, current, nil
	case *fieldExpr:
		index, err := rt.eval(v.index)
		if err != nil {
			return assignTarget{}, value{}, err
		}
		n := int(index.Number())
		if n < 0 {
			return assignTarget{}, value{}, fmt.Errorf("fatal: invalid field index")
		}
		return assignTarget{field: true, fieldIndex: n}, rt.field(n), nil
	default:
		return assignTarget{}, value{}, fmt.Errorf("expected variable")
	}
}

func (rt *runtime) setResolvedAssignable(target assignTarget, v value) error {
	if target.array {
		return rt.setArrayElem(target.name, target.key, v)
	}
	if target.field {
		return rt.setField(target.fieldIndex, v)
	}
	return rt.setVar(target.name, v)
}

func (rt *runtime) evalArrayRef(ref *arrayRefExpr) (value, error) {
	key, err := rt.evalArrayKey(ref.indices)
	if err != nil {
		return value{}, err
	}
	return rt.getArrayElem(ref.name, key)
}

func (rt *runtime) evalArrayKey(indices []expr) (string, error) {
	if len(indices) == 0 {
		return "", fmt.Errorf("array index is required")
	}
	budget := expressionTemporaryBudget{rt: rt}
	defer budget.release()
	parts := make([]string, len(indices))
	joinedBytes := 0
	for i, index := range indices {
		v, err := rt.eval(index)
		if err != nil {
			return "", err
		}
		part, err := rt.conversionString(v, "CONVFMT")
		if err != nil {
			return "", err
		}
		if err := budget.retainString(part); err != nil {
			return "", err
		}
		if len(part) > MaxExpressionBytes-joinedBytes {
			return "", expressionTemporaryLimitError()
		}
		parts[i] = part
		joinedBytes += len(part)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	sep := rt.getVar("SUBSEP").String()
	separatorCount := len(parts) - 1
	if len(sep) > (MaxExpressionBytes-joinedBytes)/separatorCount {
		return "", expressionTemporaryLimitError()
	}
	return strings.Join(parts, sep), nil
}

func boolValue(ok bool) value {
	if ok {
		return numberValue(1)
	}
	return numberValue(0)
}

func (rt *runtime) compareValues(left, right value, op string, budget *expressionTemporaryBudget) (bool, error) {
	var cmp int
	if valuesAreNumeric(left, right) {
		ln, rn := left.Number(), right.Number()
		if math.IsNaN(ln) || math.IsNaN(rn) {
			return op == "!=", nil
		}
		switch {
		case ln < rn:
			cmp = -1
		case ln > rn:
			cmp = 1
		default:
			cmp = 0
		}
	} else {
		ls, err := budget.convfmtString(left)
		if err != nil {
			return false, err
		}
		rs, err := budget.convfmtString(right)
		if err != nil {
			return false, err
		}
		cmp = strings.Compare(ls, rs)
	}
	switch op {
	case "==":
		return cmp == 0, nil
	case "!=":
		return cmp != 0, nil
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	default:
		return false, nil
	}
}

func valuesAreNumeric(left, right value) bool {
	switch left.kind {
	case valueNumber, valueStrNum:
		switch right.kind {
		case valueNumber, valueStrNum:
			return true
		}
	}
	return false
}
