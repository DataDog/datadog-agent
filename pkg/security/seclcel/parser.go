// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/cel-go/common"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"
)

// Standard CEL and extension library functions the translation targets.
const (
	bitAndFunc   = "math.bitAnd"
	bitOrFunc    = "math.bitOr"
	bitXorFunc   = "math.bitXor"
	bitNotFunc   = "math.bitNot"
	ipFunc       = "ip"
	cidrFunc     = "cidr"
	durationFunc = "duration"
	matchesFunc  = "matches"
	sizeFunc     = "size"
)

// SECL derives these two suffixes from another field rather than storing them,
// so they are translated to size() and a helper call.
const (
	lengthSuffix     = ".length"
	rootDomainSuffix = ".root_domain"
)

// iterVarBase names the variables bound by the comprehensions the translation
// introduces, for `allin` and for the array semantics of a comparison.
const iterVarBase = "elem"

// reSubscript matches a `[…]` field subscript, which SECL uses both for a
// numeric index and for an iterator variable.
var reSubscript = regexp.MustCompile(`\[([^\]]*)\]`)

// reInterpolation matches the `${…}` variables and `%{…}` field references that
// SECL substitutes inside string literals.
var reInterpolation = regexp.MustCompile(`\$\{[^}]*\}|%\{[^}]*\}`)

// operandKind records what a translated SECL operand was written as. The
// enclosing operator needs it because in SECL the *literal* carries the
// matching semantics: `x == ~"/a/*"` globs while `x == "/a/*"` compares.
type operandKind uint8

const (
	// kindDynamic covers fields, macros, variables, and anything computed.
	kindDynamic operandKind = iota
	kindString
	kindPattern
	kindRegexp
	kindInt
	kindBool
	kindIP
	kindCIDR
	kindDuration
)

// isMatcher reports whether the kind is a static string matcher, i.e. one that
// SECL applies to the other side of a comparison instead of comparing values.
func (k operandKind) isMatcher() bool {
	return k == kindPattern || k == kindRegexp
}

func (k operandKind) isCIDR() bool {
	return k == kindIP || k == kindCIDR
}

// operand is a translated SECL sub-expression together with the syntactic
// information the enclosing operator needs.
type operand struct {
	expr celast.Expr
	kind operandKind

	// text is the literal text of a string, pattern, regexp or duration.
	text string
	// num is the value of an integer, or the nanosecond value of a duration.
	num int64

	// fromArithmetic reports whether this operand is the result of a `+`/`-`
	// chain. SECL compares such an operand against a duration literal directly
	// instead of measuring the time elapsed since it.
	fromArithmetic bool

	// field is the dotted SECL field name when the operand is a plain field
	// reference, which is what the array semantics are keyed on. It is empty for
	// anything computed, and for a field reached through an iterator variable.
	field string

	// listExpr marks an operand whose expression is already a list needing to be
	// quantified, which happens when a list valued leaf sits inside an iterated
	// node: process.ancestors.args_flags is a list of flags per ancestor.
	listExpr bool

	// wrap is the function a pseudo field applies to the value it derives from,
	// size() or the root domain helper. It travels with the operand so that a
	// quantified field applies it per element: process.ancestors.file.name.length
	// is the length of each ancestor's name, not of the list of ancestors.
	wrap string

	start, end int
}

// arrayOperand is the right hand side of an `in`, `not in` or `allin`.
type arrayOperand struct {
	// expr is set when the array is written as a single name: a macro, a
	// variable, a field reference, or a bare CIDR.
	expr celast.Expr
	// members is set when the array is written as a bracketed list.
	members []operand

	// cidr reports whether the array holds IP or CIDR values.
	cidr bool

	start, end int
}

// register is the single SECL iterator variable an expression may bind, as in
// `process.ancestors[A].file.name`.
type register struct {
	name  string
	field string

	start, end int
}

type parser struct {
	toks []token
	i    int

	fac    celast.ExprFactory
	info   *celast.SourceInfo
	nextID int64

	// types is nil for a translation that has no model to consult, in which
	// case the array semantics of a comparison cannot be recovered and the
	// comparison is translated literally.
	types FieldTypes

	register *register
	iterVars int
}

// nextIterVar returns a fresh name for a bound variable, avoiding the SECL
// iterator variable the expression may already bind.
func (p *parser) nextIterVar() string {
	for {
		name := iterVarBase
		if p.iterVars > 0 {
			name = fmt.Sprintf("%s%d", iterVarBase, p.iterVars+1)
		}
		p.iterVars++
		if p.register == nil || p.register.name != name {
			return name
		}
	}
}

// Parse translates a SECL expression into a CEL AST.
//
// The returned AST is unchecked: it has not been resolved against any set of
// field, macro or variable declarations. Use Compile to type-check it, or
// Translate to render it back as CEL source.
func Parse(expr string) (*celast.AST, error) {
	return ParseSource(common.NewTextSource(expr))
}

// ParseWithTypes is Parse against a set of field types, which is what lets the
// array semantics SECL leaves implicit be written out: a comparison against an
// array field becomes an exists() over it.
//
// Without them the comparison is translated literally, which is valid CEL but
// means "the list equals the scalar" rather than "some element does".
func ParseWithTypes(expr string, fieldTypes FieldTypes) (*celast.AST, error) {
	return parseSource(common.NewTextSource(expr), fieldTypes)
}

// ParseSource is Parse over a named source, so that errors and position
// information refer to it.
func ParseSource(src common.Source) (*celast.AST, error) {
	return parseSource(src, nil)
}

func parseSource(src common.Source, fieldTypes FieldTypes) (*celast.AST, error) {
	toks, lexErr := lex(src.Content())
	if lexErr != nil {
		return nil, lexErr.withSource(src)
	}

	p := &parser{
		toks:   toks,
		fac:    celast.NewExprFactory(),
		info:   celast.NewSourceInfo(src),
		nextID: 1,
		types:  fieldTypes,
	}

	root, err := p.expression()
	if err != nil {
		return nil, err.withSource(src)
	}
	if tok := p.peek(); tok.kind != tokEOF {
		return nil, errorf(tok.start, "unexpected %s", tok.describe()).withSource(src)
	}

	expr := root.expr

	// A SECL expression that binds an iterator variable matches when *some*
	// element of the iterated field satisfies the whole expression, which is
	// exactly CEL's exists() macro over that field.
	if r := p.register; r != nil {
		iterRange, chainErr := p.selectChain(r.field, r.start, r.end)
		if chainErr != nil {
			return nil, chainErr.withSource(src)
		}
		expr = p.newComprehension("exists", iterRange, r.name, expr, root.start, root.end)
	}

	return celast.NewAST(expr, p.info), nil
}

//
// token access
//

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) peekAt(n int) token {
	if p.i+n >= len(p.toks) {
		return p.toks[len(p.toks)-1]
	}
	return p.toks[p.i+n]
}

func (p *parser) next() token {
	tok := p.toks[p.i]
	if tok.kind != tokEOF {
		p.i++
	}
	return tok
}

func (p *parser) peekPunct(n int, val string) bool {
	tok := p.peekAt(n)
	return tok.kind == tokPunct && tok.val == val
}

func (p *parser) matchPunct(val string) bool {
	if p.peekPunct(0, val) {
		p.next()
		return true
	}
	return false
}

// matchPunct2 consumes a two character operator. SECL lexes operators one
// character at a time and discards the whitespace between them, so `> =` is
// accepted just like `>=`.
func (p *parser) matchPunct2(val string) bool {
	if p.peekPunct(0, val[:1]) && p.peekPunct(1, val[1:]) {
		p.next()
		p.next()
		return true
	}
	return false
}

func (p *parser) peekWord(n int, val string) bool {
	tok := p.peekAt(n)
	return tok.kind == tokIdent && tok.val == val
}

func (p *parser) matchWord(val string) bool {
	if p.peekWord(0, val) {
		p.next()
		return true
	}
	return false
}

func (p *parser) matchWord2(first, second string) bool {
	if p.peekWord(0, first) && p.peekWord(1, second) {
		p.next()
		p.next()
		return true
	}
	return false
}

//
// AST construction helpers
//

// id allocates an expression id and records the source range it came from.
func (p *parser) id(start, end int) int64 {
	id := p.nextID
	p.nextID++
	p.info.SetOffsetRange(id, celast.OffsetRange{Start: int32(start), Stop: int32(end)})
	return id
}

func (p *parser) call(fn string, start, end int, args ...celast.Expr) celast.Expr {
	return p.fac.NewCall(p.id(start, end), fn, args...)
}

func (p *parser) memberCall(fn string, target celast.Expr, start, end int, args ...celast.Expr) celast.Expr {
	return p.fac.NewMemberCall(p.id(start, end), fn, target, args...)
}

func (p *parser) not(e celast.Expr, start, end int) celast.Expr {
	return p.call(operators.LogicalNot, start, end, e)
}

func (p *parser) strLit(s string, start, end int) celast.Expr {
	return p.fac.NewLiteral(p.id(start, end), types.String(s))
}

func (p *parser) intLit(n int64, start, end int) celast.Expr {
	return p.fac.NewLiteral(p.id(start, end), types.Int(n))
}

// selectChain builds the CEL identifier and field selections for a dotted SECL
// name. CEL resolves such a chain against the declared names, so a field, a
// macro and a constant all translate the same way.
func (p *parser) selectChain(name string, start, end int) (celast.Expr, *ParseError) {
	parts := strings.Split(name, ".")
	for _, part := range parts {
		if part == "" {
			return nil, errorf(start, "malformed name %q", name)
		}
	}

	e := p.fac.NewIdent(p.id(start, end), parts[0])
	for _, part := range parts[1:] {
		e = p.fac.NewSelect(p.id(start, end), e, part)
	}
	return e, nil
}

// appendSelects applies a `.a.b` suffix to an already built expression.
func (p *parser) appendSelects(e celast.Expr, suffix string, start, end int) (celast.Expr, *ParseError) {
	if suffix == "" {
		return e, nil
	}
	if suffix[0] != '.' {
		return nil, errorf(start, "expected `.` after subscript, got %q", suffix)
	}
	for _, part := range strings.Split(suffix[1:], ".") {
		if part == "" {
			return nil, errorf(start, "malformed name after subscript")
		}
		e = p.fac.NewSelect(p.id(start, end), e, part)
	}
	return e, nil
}

// newComprehension builds the comprehension that CEL's all() and exists()
// macros expand to, and records the macro call so that the result can be
// unparsed as `range.all(v, …)` rather than as the raw comprehension.
func (p *parser) newComprehension(fn string, iterRange celast.Expr, iterVar string, pred celast.Expr, start, end int) celast.Expr {
	id := p.id(start, end)

	var accuInit, cond, step celast.Expr
	switch fn {
	case "all":
		accuInit = p.fac.NewLiteral(p.id(start, end), types.True)
		cond = p.call(operators.NotStrictlyFalse, start, end, p.fac.NewAccuIdent(p.id(start, end)))
		step = p.call(operators.LogicalAnd, start, end, p.fac.NewAccuIdent(p.id(start, end)), pred)
	case "exists":
		accuInit = p.fac.NewLiteral(p.id(start, end), types.False)
		cond = p.call(operators.NotStrictlyFalse, start, end,
			p.not(p.fac.NewAccuIdent(p.id(start, end)), start, end))
		step = p.call(operators.LogicalOr, start, end, p.fac.NewAccuIdent(p.id(start, end)), pred)
	}

	comp := p.fac.NewComprehension(id, iterRange, iterVar, p.fac.AccuIdentName(),
		accuInit, cond, step, p.fac.NewAccuIdent(p.id(start, end)))

	p.info.SetMacroCall(id, p.fac.NewMemberCall(0, fn, iterRange,
		p.fac.NewIdent(p.id(start, end), iterVar), pred))

	return comp
}

//
// grammar
//

// expression parses
//
//	expression := comparison [ ("||" | "or" | "&&" | "and") expression ]
//
// SECL gives `&&` and `||` the same precedence and makes them right
// associative, so `a && b || c` groups as `a && (b || c)`. Building the CEL AST
// directly preserves that grouping; rendering it back as CEL source
// parenthesises it so that CEL's own precedence does not change the meaning.
func (p *parser) expression() (operand, *ParseError) {
	lhs, err := p.comparison()
	if err != nil {
		return operand{}, err
	}

	var fn string
	switch {
	case p.matchPunct2("||"), p.matchWord("or"):
		fn = operators.LogicalOr
	case p.matchPunct2("&&"), p.matchWord("and"):
		fn = operators.LogicalAnd
	default:
		return lhs, nil
	}

	rhs, err := p.expression()
	if err != nil {
		return operand{}, err
	}

	return operand{
		expr:  p.call(fn, lhs.start, rhs.end, lhs.expr, rhs.expr),
		start: lhs.start,
		end:   rhs.end,
	}, nil
}

// comparison parses
//
//	comparison := arithmetic [ scalarOp comparison | arrayOp array ]
func (p *parser) comparison() (operand, *ParseError) {
	lhs, err := p.arithmetic()
	if err != nil {
		return operand{}, err
	}

	if op, ok := p.scalarOp(); ok {
		rhs, err := p.comparison()
		if err != nil {
			return operand{}, err
		}
		return p.scalarComparison(op, lhs, rhs)
	}

	if op, ok := p.arrayOp(); ok {
		arr, err := p.array()
		if err != nil {
			return operand{}, err
		}
		return p.arrayComparison(op, lhs, arr)
	}

	return lhs, nil
}

// scalarOp consumes a scalar comparison operator. A lone `!` or `=` is left in
// place: it is not an operator on its own.
func (p *parser) scalarOp() (string, bool) {
	for _, op := range []string{">=", "<=", "!=", "==", "=~", "!~"} {
		if p.matchPunct2(op) {
			return op, true
		}
	}
	for _, op := range []string{">", "<"} {
		if p.matchPunct(op) {
			return op, true
		}
	}
	return "", false
}

func (p *parser) arrayOp() (string, bool) {
	switch {
	case p.matchWord2("not", "in"):
		return "notin", true
	case p.matchWord("in"):
		return "in", true
	case p.matchWord("allin"):
		return "allin", true
	}
	return "", false
}

// arithmetic parses
//
//	arithmetic := bitOperation { ("+" | "-") bitOperation }
//
// which is left associative, unlike the rest of the SECL operators.
func (p *parser) arithmetic() (operand, *ParseError) {
	acc, err := p.bitOperation()
	if err != nil {
		return operand{}, err
	}

	for {
		var fn string
		switch {
		case p.matchPunct("+"):
			fn = operators.Add
		case p.matchPunct("-"):
			fn = operators.Subtract
		default:
			return acc, nil
		}

		rhs, err := p.bitOperation()
		if err != nil {
			return operand{}, err
		}
		acc = operand{
			expr:           p.call(fn, acc.start, rhs.end, acc.expr, rhs.expr),
			fromArithmetic: true,
			start:          acc.start,
			end:            rhs.end,
		}
	}
}

// bitOperation parses
//
//	bitOperation := unary [ ("&" | "|" | "^") bitOperation ]
func (p *parser) bitOperation() (operand, *ParseError) {
	lhs, err := p.unary()
	if err != nil {
		return operand{}, err
	}

	var fn string
	switch {
	// `&&` and `||` belong to the enclosing logical expression.
	case p.peekPunct(0, "&") && !p.peekPunct(1, "&"):
		fn = bitAndFunc
	case p.peekPunct(0, "|") && !p.peekPunct(1, "|"):
		fn = bitOrFunc
	case p.peekPunct(0, "^"):
		fn = bitXorFunc
	default:
		return lhs, nil
	}
	p.next()

	rhs, err := p.bitOperation()
	if err != nil {
		return operand{}, err
	}

	return operand{
		expr:  p.call(fn, lhs.start, rhs.end, lhs.expr, rhs.expr),
		start: lhs.start,
		end:   rhs.end,
	}, nil
}

// unary parses
//
//	unary := ("!" | "not" | "-" | "^") unary | primary
func (p *parser) unary() (operand, *ParseError) {
	start := p.peek().start

	var fn string
	switch {
	case p.peekPunct(0, "!") && !p.peekPunct(1, "=") && !p.peekPunct(1, "~"):
		p.next()
		fn = operators.LogicalNot
	case p.matchWord("not"):
		fn = operators.LogicalNot
	case p.matchPunct("-"):
		fn = operators.Negate
	case p.matchPunct("^"):
		fn = bitNotFunc
	default:
		return p.primary()
	}

	inner, err := p.unary()
	if err != nil {
		return operand{}, err
	}

	// The CEL parser folds a minus applied to an integer literal into the
	// literal, so fold it here too and keep the output in CEL's own normal form.
	// SECL already lexes `-3` as a single integer, so this only ever fires for a
	// minus that was written separately.
	if fn == operators.Negate && inner.kind == kindInt {
		return operand{
			expr: p.intLit(-inner.num, start, inner.end), kind: kindInt, num: -inner.num,
			start: start, end: inner.end,
		}, nil
	}

	return operand{
		expr:  p.call(fn, start, inner.end, inner.expr),
		start: start,
		end:   inner.end,
	}, nil
}

// primary parses a single SECL operand.
func (p *parser) primary() (operand, *ParseError) {
	tok := p.peek()

	switch tok.kind {
	case tokIdent:
		p.next()
		return p.identOperand(tok)

	case tokInt:
		p.next()
		return operand{
			expr: p.intLit(tok.num, tok.start, tok.end), kind: kindInt, num: tok.num,
			start: tok.start, end: tok.end,
		}, nil

	case tokDuration:
		p.next()
		// A duration is an integer number of nanoseconds everywhere except on
		// the right of a comparison, which scalarComparison rewrites into a CEL
		// duration.
		return operand{
			expr: p.intLit(tok.num, tok.start, tok.end), kind: kindDuration,
			text: tok.val, num: tok.num,
			start: tok.start, end: tok.end,
		}, nil

	case tokString:
		p.next()
		return p.stringOperand(tok)

	case tokPattern:
		p.next()
		return operand{
			expr: p.strLit(tok.val, tok.start, tok.end), kind: kindPattern, text: tok.val,
			start: tok.start, end: tok.end,
		}, nil

	case tokRegexp:
		p.next()
		return operand{
			expr: p.strLit(tok.val, tok.start, tok.end), kind: kindRegexp, text: tok.val,
			start: tok.start, end: tok.end,
		}, nil

	case tokIP, tokCIDR:
		p.next()
		return p.cidrOperand(tok)

	case tokVariable:
		p.next()
		return p.variableOperand(tok)

	case tokFieldRef:
		p.next()
		return p.fieldRefOperand(tok)

	case tokPunct:
		if tok.val == "(" {
			p.next()
			inner, err := p.expression()
			if err != nil {
				return operand{}, err
			}
			if !p.matchPunct(")") {
				return operand{}, errorf(p.peek().start, "expected `)`, got %s", p.peek().describe())
			}
			// CEL has no grouping node: the AST shape already encodes it.
			inner.start, inner.end = tok.start, p.toks[p.i-1].end
			return inner, nil
		}
	}

	return operand{}, errorf(tok.start, "unexpected %s", tok.describe())
}

//
// operands
//

// identOperand translates a bare name: a boolean literal, a field, a macro, or
// a constant. Which one it is depends on declarations the translator does not
// have, and all three of the latter translate to the same CEL name.
func (p *parser) identOperand(tok token) (operand, *ParseError) {
	if tok.val == "true" || tok.val == "false" {
		return operand{
			expr:  p.fac.NewLiteral(p.id(tok.start, tok.end), types.Bool(tok.val == "true")),
			kind:  kindBool,
			start: tok.start, end: tok.end,
		}, nil
	}

	// `x.length` and `x.root_domain` are derived, not stored. SECL exposes them
	// as fields; CEL has size() for the first and a helper for the second.
	if p.types != nil && p.types.IsPseudoField(tok.val) {
		return p.pseudoFieldOperand(tok)
	}

	expr, err := p.nameExpr(tok.val, tok.start, tok.end)
	if err != nil {
		return operand{}, err
	}

	field := tok.val
	if strings.ContainsAny(field, "[]") {
		// A subscripted name is either indexed or bound to an iterator variable;
		// either way it no longer denotes the whole field.
		field = ""
	}
	return operand{expr: expr, field: field, start: tok.start, end: tok.end}, nil
}

// nameExpr translates a field name, resolving the `[…]` subscript that SECL
// uses for both numeric indexing and iterator variables.
func (p *parser) nameExpr(name string, start, end int) (celast.Expr, *ParseError) {
	subs := reSubscript.FindAllStringSubmatchIndex(name, -1)
	if len(subs) == 0 {
		return p.selectChain(name, start, end)
	}
	if len(subs) > 1 {
		return nil, errorf(start, "field %q has more than one subscript", name)
	}

	m := subs[0]
	base, index, suffix := name[:m[0]], name[m[2]:m[3]], name[m[1]:]
	if base == "" {
		return nil, errorf(start, "missing field name before the subscript in %q", name)
	}
	if index == "" {
		return nil, errorf(start, "empty subscript in %q", name)
	}

	if isDigits(index) {
		n, convErr := strconv.ParseInt(index, 10, 64)
		if convErr != nil {
			return nil, errorf(start, "invalid array index %q in %q", index, name)
		}
		baseExpr, err := p.selectChain(base, start, end)
		if err != nil {
			return nil, err
		}
		elem := p.call(operators.Index, start, end, baseExpr, p.intLit(n, start, end))
		return p.appendSelects(elem, suffix, start, end)
	}

	// An iterator variable: the field it subscripts becomes the range of an
	// exists() wrapped around the whole expression, and the name itself becomes
	// the bound variable.
	if err := p.useRegister(index, base, start, end); err != nil {
		return nil, err
	}
	return p.appendSelects(p.fac.NewIdent(p.id(start, end), index), suffix, start, end)
}

// useRegister records the iterator variable an expression binds. SECL allows
// only one per expression.
func (p *parser) useRegister(name, field string, start, end int) *ParseError {
	if name == "_" {
		return errorf(start, "`_` cannot be used as an iterator variable name")
	}

	if p.register == nil {
		p.register = &register{name: name, field: field, start: start, end: end}
		return nil
	}
	if p.register.name != name {
		return errorf(start, "only one iterator variable is supported per expression, found %q and %q",
			p.register.name, name)
	}
	if p.register.field != field {
		return errorf(start, "iterator variable %q is used with two different fields, %q and %q",
			name, p.register.field, field)
	}
	return nil
}

// variableOperand translates `${name}`. Variables live under their own root so
// that they cannot collide with a field of the same name.
func (p *parser) variableOperand(tok token) (operand, *ParseError) {
	name := tok.val
	length := false
	if base, ok := strings.CutSuffix(name, ".length"); ok {
		name, length = base, true
	}

	expr, err := p.selectChain(VariablesRoot+"."+name, tok.start, tok.end)
	if err != nil {
		return operand{}, err
	}
	if length {
		expr = p.call(sizeFunc, tok.start, tok.end, expr)
	}
	return operand{expr: expr, start: tok.start, end: tok.end}, nil
}

// fieldRefOperand translates `%{name}`, which in SECL resolves against the
// event fields only, never against macros, constants or variables. CEL name
// resolution does not draw that distinction, so both spellings produce the same
// qualified name.
func (p *parser) fieldRefOperand(tok token) (operand, *ParseError) {
	name := tok.val

	var wrap string
	if base, ok := strings.CutSuffix(name, ".length"); ok {
		name, wrap = base, sizeFunc
	} else if base, ok := strings.CutSuffix(name, ".root_domain"); ok {
		name, wrap = base, RootDomainFunc
	}

	if strings.ContainsAny(name, "[]") {
		return operand{}, errorf(tok.start, "subscripts are not supported in the field reference %q", tok.val)
	}

	expr, err := p.selectChain(name, tok.start, tok.end)
	if err != nil {
		return operand{}, err
	}
	if wrap != "" {
		expr = p.call(wrap, tok.start, tok.end, expr)
	}
	return operand{expr: expr, start: tok.start, end: tok.end}, nil
}

// cidrOperand translates an IP or CIDR literal into the corresponding
// constructor from the CEL network extension library.
func (p *parser) cidrOperand(tok token) (operand, *ParseError) {
	kind, fn := kindIP, ipFunc
	if tok.kind == tokCIDR {
		kind, fn = kindCIDR, cidrFunc

		if _, _, err := net.ParseCIDR(tok.val); err != nil {
			return operand{}, errorf(tok.start, "invalid CIDR %q", tok.val)
		}
	} else if net.ParseIP(tok.val) == nil {
		return operand{}, errorf(tok.start, "invalid IP %q", tok.val)
	}

	return operand{
		expr:  p.call(fn, tok.start, tok.end, p.strLit(tok.val, tok.start, tok.end)),
		kind:  kind,
		text:  tok.val,
		start: tok.start, end: tok.end,
	}, nil
}

// stringOperand translates a string literal, expanding the `${…}` variables and
// `%{…}` field references SECL substitutes into it as a CEL concatenation.
func (p *parser) stringOperand(tok token) (operand, *ParseError) {
	locs := reInterpolation.FindAllStringIndex(tok.val, -1)
	if len(locs) == 0 {
		return operand{
			expr: p.strLit(tok.val, tok.start, tok.end), kind: kindString, text: tok.val,
			start: tok.start, end: tok.end,
		}, nil
	}

	var parts []celast.Expr
	addLiteral := func(s string) {
		if s != "" {
			parts = append(parts, p.strLit(s, tok.start, tok.end))
		}
	}

	last := 0
	for _, loc := range locs {
		addLiteral(tok.val[last:loc[0]])

		ref := tok.val[loc[0]:loc[1]]
		inner := token{val: ref[2 : len(ref)-1], start: tok.start, end: tok.end}

		var sub operand
		var err *ParseError
		if ref[0] == '$' {
			inner.kind = tokVariable
			sub, err = p.variableOperand(inner)
		} else {
			inner.kind = tokFieldRef
			sub, err = p.fieldRefOperand(inner)
		}
		if err != nil {
			return operand{}, err
		}

		// SECL renders the substituted value as a string, joining lists with
		// commas, which is what StrFunc stands for.
		parts = append(parts, p.call(StrFunc, tok.start, tok.end, sub.expr))
		last = loc[1]
	}
	addLiteral(tok.val[last:])

	expr := parts[0]
	for _, part := range parts[1:] {
		expr = p.call(operators.Add, tok.start, tok.end, expr, part)
	}

	// The result is computed, so it can no longer act as a static pattern.
	return operand{expr: expr, start: tok.start, end: tok.end}, nil
}

//
// comparisons
//

// scalarComparison translates the SECL scalar operators. Which CEL form it
// produces depends on how the operands were written, because a pattern, a
// regexp, a CIDR and a duration each give the operator a different meaning.
func (p *parser) scalarComparison(op string, lhs, rhs operand) (operand, *ParseError) {
	// SECL reads a comparison against an array field as "some element matches".
	// Only one side can be quantified: when both are arrays the comparison is
	// left literal, and the checker reports it rather than the translator
	// guessing which side to iterate.
	start, end := lhs.start, rhs.end

	// Quantifying is recursive: a list valued leaf inside an iterated node needs
	// one exists() per level. Only one side is quantified at a time, and the left
	// goes first, so a comparison between two array fields is left literal for
	// the checker to report rather than guessed at.
	var compare func(l, r operand) (celast.Expr, *ParseError)
	compare = func(l, r operand) (celast.Expr, *ParseError) {
		if lr, ok := p.listRange(l); ok {
			return p.quantifyExpr("exists", lr, start, end, func(elem operand) (celast.Expr, *ParseError) {
				return compare(elem, r)
			})
		}
		if rr, ok := p.listRange(r); ok {
			return p.quantifyExpr("exists", rr, start, end, func(elem operand) (celast.Expr, *ParseError) {
				return compare(l, elem)
			})
		}
		return p.exprOf(p.scalarComparisonOf(op, l, r))
	}

	expr, err := compare(lhs, rhs)
	if err != nil {
		return operand{}, err
	}
	return operand{expr: expr, start: start, end: end}, nil
}

func (p *parser) scalarComparisonOf(op string, lhs, rhs operand) (operand, *ParseError) {
	start, end := lhs.start, rhs.end
	result := func(e celast.Expr) (operand, *ParseError) {
		return operand{expr: e, start: start, end: end}, nil
	}

	switch op {
	case "=~", "!~":
		// SECL requires a static right hand side here, and promotes a plain
		// string to a glob pattern.
		matcher := rhs
		switch matcher.kind {
		case kindString:
			matcher.kind = kindPattern
		case kindPattern, kindRegexp:
		default:
			return operand{}, errorf(rhs.start,
				"the right hand side of `%s` must be a string, a pattern or a regexp", op)
		}

		e, err := p.applyMatcher(lhs, matcher, start, end)
		if err != nil {
			return operand{}, err
		}
		if op == "!~" {
			e = p.not(e, start, end)
		}
		return result(e)

	case "==", "!=":
		// A pattern or regexp on either side turns the comparison into a match.
		subject, matcher := lhs, rhs
		if lhs.kind.isMatcher() && !rhs.kind.isMatcher() {
			subject, matcher = rhs, lhs
		}
		if matcher.kind.isMatcher() {
			if subject.kind.isMatcher() {
				return operand{}, errorf(start, "cannot compare two patterns")
			}
			e, err := p.applyMatcher(subject, matcher, start, end)
			if err != nil {
				return operand{}, err
			}
			if op == "!=" {
				e = p.not(e, start, end)
			}
			return result(e)
		}

		if lhs.kind.isCIDR() || rhs.kind.isCIDR() {
			e := p.call(CIDRMatchFunc, start, end, lhs.expr, rhs.expr)
			if op == "!=" {
				e = p.not(e, start, end)
			}
			return result(e)
		}

		if rhs.kind == kindDuration {
			return result(p.durationComparison(op, lhs, rhs, start, end))
		}

		fn := operators.Equals
		if op == "!=" {
			fn = operators.NotEquals
		}
		return result(p.call(fn, start, end, lhs.expr, rhs.expr))

	default: // "<", "<=", ">", ">="
		if rhs.kind == kindDuration {
			return result(p.durationComparison(op, lhs, rhs, start, end))
		}
		return result(p.call(relationalOp(op), start, end, lhs.expr, rhs.expr))
	}
}

// applyMatcher builds the CEL test that decides whether subject matches the
// literal on the other side of the operator.
func (p *parser) applyMatcher(subject, matcher operand, start, end int) (celast.Expr, *ParseError) {
	switch matcher.kind {
	case kindPattern:
		// CEL has no glob matching of its own.
		return p.call(GlobFunc, start, end, subject.expr, p.strLit(matcher.text, matcher.start, matcher.end)), nil
	case kindRegexp:
		// SECL and CEL agree here: both are unanchored RE2 matches.
		return p.memberCall(matchesFunc, subject.expr, start, end,
			p.strLit(matcher.text, matcher.start, matcher.end)), nil
	default:
		return p.call(operators.Equals, start, end, subject.expr, matcher.expr), nil
	}
}

// durationComparison translates a comparison against a duration literal. SECL
// reads such a comparison as "how long ago did this happen", unless the left
// hand side is itself an arithmetic expression, in which case the two operands
// are compared directly.
func (p *parser) durationComparison(op string, subject, dur operand, start, end int) celast.Expr {
	// A bare field is a timestamp, so the comparison is against how long ago it
	// was. An arithmetic result is already an interval and is compared directly.
	// Naming the instant rather than reading the clock inside a helper is what
	// keeps every comparison in one evaluation consistent, as SECL's cached
	// ctx.Now() does.
	measured := subject.expr
	if !subject.fromArithmetic {
		measured = p.call(operators.Subtract, start, end,
			p.fac.NewIdent(p.id(start, end), NowVar), subject.expr)
	}

	elapsed := p.call(NanosFunc, subject.start, subject.end, measured)
	literal := p.call(durationFunc, dur.start, dur.end, p.strLit(dur.text, dur.start, dur.end))

	if op == "!=" {
		return p.not(p.call(operators.Equals, start, end, elapsed, literal), start, end)
	}
	return p.call(relationalOp(op), start, end, elapsed, literal)
}

// arrayComparison translates `in`, `not in` and `allin`.
func (p *parser) arrayComparison(op string, lhs operand, arr arrayOperand) (operand, *ParseError) {
	start, end := lhs.start, arr.end

	// IP and CIDR membership follows SECL's own overlap rule rather than CEL
	// list membership, and its helpers already take a list on either side.
	if lhs.kind.isCIDR() || arr.cidr {
		fn := CIDRMatchFunc
		if op == "allin" {
			fn = CIDRMatchAllFunc
		}
		e := p.call(fn, start, end, lhs.expr, p.arrayExpr(arr))
		if op == "notin" {
			e = p.not(e, start, end)
		}
		return operand{expr: e, start: start, end: end}, nil
	}

	// `in` and `not in` over an array left hand side ask whether *some* element
	// is a member; `allin` asks whether *every* element is. `not in` negates the
	// whole quantifier rather than the membership test inside it.
	macro, negate := "exists", false
	switch op {
	case "notin":
		negate = true
	case "allin":
		macro = "all"
	}

	var member func(o operand) (celast.Expr, *ParseError)
	member = func(o operand) (celast.Expr, *ParseError) {
		if r, ok := p.listRange(o); ok {
			return p.quantifyExpr(macro, r, start, end, member)
		}
		// A single value: `allin` degenerates to membership.
		return p.membership(o, arr, start, end)
	}

	if p.types == nil && op == "allin" {
		// Without field types the left hand side of `allin` is assumed to be an
		// array, since that is the only shape the operator is meaningful for.
		e, err := p.quantifyExpr(macro, listRange{expr: lhs.expr}, start, end, member)
		if err != nil {
			return operand{}, err
		}
		return operand{expr: e, start: start, end: end}, nil
	}

	e, err := member(lhs)
	if err != nil {
		return operand{}, err
	}
	if negate {
		e = p.not(e, start, end)
	}
	return operand{expr: e, start: start, end: end}, nil
}

// listRange describes an operand that yields several values, and how to reach
// the compared value from one of them.
type listRange struct {
	// expr is the list to iterate.
	expr celast.Expr
	// remainder is the dotted path from an element to the compared value, empty
	// when the elements are the compared values themselves.
	remainder string
	// elemIsList reports whether the value reached in each element is itself a
	// list, and so needs quantifying again.
	elemIsList bool
	// wrap is applied to the value reached in each element.
	wrap string
}

// listRange reports whether an operand holds several values, which is what turns
// a comparison into a quantified one. It answers false without field types,
// leaving the comparison to be translated literally.
func (p *parser) listRange(o operand) (listRange, bool) {
	if o.listExpr {
		return listRange{expr: o.expr}, true
	}
	if p.types == nil || o.field == "" {
		return listRange{}, false
	}

	if prefix := p.types.ListPrefix(o.field); prefix != "" {
		remainder := strings.TrimPrefix(strings.TrimPrefix(o.field, prefix), ".")

		// When the field *is* the iterated node, it denotes the list itself, so a
		// pseudo field applies to the list: network_flow_monitor.flows.length is
		// the number of flows.
		if remainder == "" && o.wrap != "" {
			return listRange{}, false
		}

		expr, err := p.selectChain(prefix, o.start, o.end)
		if err != nil {
			return listRange{}, false
		}
		return listRange{
			expr:      expr,
			remainder: remainder,
			// A pseudo field consumes the list it derives from, so what is left
			// per element is a single value.
			elemIsList: o.wrap == "" && p.types.IsListLeaf(o.field),
			wrap:       o.wrap,
		}, true
	}

	// size() applies to the list itself, so process.argv.length is the number of
	// arguments rather than one length per argument.
	if o.wrap == "" && p.types.IsListLeaf(o.field) {
		return listRange{expr: o.expr}, true
	}

	return listRange{}, false
}

// quantify wraps a test in an exists() or all() over a list, binding a fresh
// variable and handing the caller the element to compare against.
func (p *parser) quantifyExpr(macro string, r listRange, start, end int, build func(elem operand) (celast.Expr, *ParseError)) (celast.Expr, *ParseError) {
	iterVar := p.nextIterVar()

	elemExpr, err := p.appendSelects(p.fac.NewIdent(p.id(start, end), iterVar), dotted(r.remainder), start, end)
	if err != nil {
		return nil, err
	}
	if r.wrap != "" {
		elemExpr = p.call(r.wrap, start, end, elemExpr)
	}

	// The element is no longer a named field, so only its own list-ness can make
	// it quantifiable again.
	pred, err := build(operand{expr: elemExpr, listExpr: r.elemIsList, start: start, end: end})
	if err != nil {
		return nil, err
	}

	return p.newComprehension(macro, r.expr, iterVar, pred, start, end), nil
}

// exprOf adapts an operand-returning translation to the expression the
// quantifier needs.
func (p *parser) exprOf(o operand, err *ParseError) (celast.Expr, *ParseError) {
	if err != nil {
		return nil, err
	}
	return o.expr, nil
}

// pseudoFieldOperand translates `x.length` and `x.root_domain`, which SECL
// derives from x rather than storing.
func (p *parser) pseudoFieldOperand(tok token) (operand, *ParseError) {
	name, wrap := tok.val, ""
	if base, ok := strings.CutSuffix(name, lengthSuffix); ok {
		name, wrap = base, sizeFunc
	} else if base, ok := strings.CutSuffix(name, rootDomainSuffix); ok {
		name, wrap = base, RootDomainFunc
	}

	expr, err := p.nameExpr(name, tok.start, tok.end)
	if err != nil {
		return operand{}, err
	}

	// The operand keeps the name it derives from and the function to apply, so
	// that a comparison against an iterated field can quantify first and apply
	// the function to each element.
	return operand{
		expr:  p.call(wrap, tok.start, tok.end, expr),
		field: name,
		wrap:  wrap,
		start: tok.start, end: tok.end,
	}, nil
}

// dotted prefixes a path remainder with the separator appendSelects expects.
func dotted(remainder string) string {
	if remainder == "" {
		return ""
	}
	return "." + remainder
}

// membership builds the test for `subject in arr`. A list of plain values maps
// onto CEL's `in`; a list holding patterns or regexps has to become a
// disjunction of matches, because CEL list membership only compares for
// equality.
func (p *parser) membership(subject operand, arr arrayOperand, start, end int) (celast.Expr, *ParseError) {
	if arr.expr != nil || !arr.hasMatcher() {
		return p.call(operators.In, start, end, subject.expr, p.arrayExpr(arr)), nil
	}

	var e celast.Expr
	for _, member := range arr.members {
		test, err := p.applyMatcher(subject, member, start, end)
		if err != nil {
			return nil, err
		}
		if e == nil {
			e = test
		} else {
			e = p.call(operators.LogicalOr, start, end, e, test)
		}
	}
	return e, nil
}

// arrayExpr renders an array operand as a single CEL expression.
func (p *parser) arrayExpr(arr arrayOperand) celast.Expr {
	if arr.expr != nil {
		return arr.expr
	}

	elems := make([]celast.Expr, 0, len(arr.members))
	for _, member := range arr.members {
		elems = append(elems, member.expr)
	}
	return p.fac.NewList(p.id(arr.start, arr.end), elems, nil)
}

// hasMatcher reports whether the array holds a pattern or a regexp.
func (a arrayOperand) hasMatcher() bool {
	for _, member := range a.members {
		if member.kind.isMatcher() {
			return true
		}
	}
	return false
}

// array parses the right hand side of an `in`, `not in` or `allin`.
func (p *parser) array() (arrayOperand, *ParseError) {
	tok := p.peek()

	if tok.kind == tokPunct && tok.val == "[" {
		return p.arrayLiteral()
	}

	// A single name: a macro, a variable, a field reference or a bare address.
	// A literal of any other kind is not a list, so it is rejected here rather
	// than translated into something meaningless.
	switch tok.kind {
	case tokIdent, tokVariable, tokFieldRef, tokCIDR, tokIP:
	default:
		return arrayOperand{}, errorf(tok.start,
			"expected a list, a name or a CIDR on the right of the operator, got %s", tok.describe())
	}

	single, err := p.primary()
	if err != nil {
		return arrayOperand{}, err
	}
	return arrayOperand{
		expr:  single.expr,
		cidr:  single.kind.isCIDR(),
		start: single.start, end: single.end,
	}, nil
}

func (p *parser) arrayLiteral() (arrayOperand, *ParseError) {
	start := p.next().start // consume "["

	var members []operand
	for {
		member, err := p.primary()
		if err != nil {
			return arrayOperand{}, err
		}
		members = append(members, member)

		if !p.matchPunct(",") {
			break
		}
	}

	if !p.matchPunct("]") {
		return arrayOperand{}, errorf(p.peek().start, "expected `]`, got %s", p.peek().describe())
	}
	end := p.toks[p.i-1].end

	cidr := false
	for _, member := range members {
		if member.kind.isCIDR() {
			cidr = true
			break
		}
	}

	return arrayOperand{members: members, cidr: cidr, start: start, end: end}, nil
}

//
// helpers
//

func relationalOp(op string) string {
	switch op {
	case "<":
		return operators.Less
	case "<=":
		return operators.LessEquals
	case ">":
		return operators.Greater
	case ">=":
		return operators.GreaterEquals
	case "==":
		return operators.Equals
	}
	return operators.NotEquals
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// describe renders a token for an error message.
func (t token) describe() string {
	if t.kind == tokEOF {
		return "<EOF>"
	}
	if t.kind == tokPunct {
		return "`" + t.val + "`"
	}
	return t.kind.String() + " " + strconv.Quote(t.val)
}
