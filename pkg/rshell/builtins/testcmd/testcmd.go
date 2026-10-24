// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package testcmd implements the POSIX test and [ builtin commands.
//
// Usage:
//
//	test EXPRESSION
//	[ EXPRESSION ]
//
// Evaluate a conditional expression and exit with status 0 (true) or 1 (false).
// The [ form requires a closing ] as the last argument.
//
// Exit codes:
//
//	0 — expression evaluates to true
//	1 — expression evaluates to false
//	2 — syntax/usage error
//
// Supported operators:
//
// File tests (unary):
//
//	-a FILE    FILE exists (deprecated POSIX synonym for -e)
//	-e FILE    FILE exists
//	-f FILE    FILE exists and is a regular file
//	-d FILE    FILE exists and is a directory
//	-s FILE    FILE exists and has size greater than zero
//	-r FILE    FILE exists and is readable
//	-w FILE    FILE exists and is writable
//	-x FILE    FILE exists and is executable
//	-h FILE    FILE exists and is a symbolic link
//	-L FILE    FILE exists and is a symbolic link (same as -h)
//	-p FILE    FILE exists and is a named pipe (FIFO)
//
// File comparison (binary):
//
//	FILE1 -nt FILE2    FILE1 is newer (modification time) than FILE2
//	FILE1 -ot FILE2    FILE1 is older (modification time) than FILE2
//
// String tests (unary):
//
//	-z STRING    length of STRING is zero
//	-n STRING    length of STRING is non-zero
//	STRING       STRING is non-empty (equivalent to -n STRING)
//
// String comparison (binary):
//
//	S1 = S2     strings are equal
//	S1 == S2    strings are equal (synonym for =)
//	S1 != S2    strings are not equal
//	S1 < S2     S1 sorts before S2 (lexicographic)
//	S1 > S2     S1 sorts after S2 (lexicographic)
//
// Integer comparison (binary):
//
//	N1 -eq N2   integers are equal
//	N1 -ne N2   integers are not equal
//	N1 -lt N2   N1 is less than N2
//	N1 -le N2   N1 is less than or equal to N2
//	N1 -gt N2   N1 is greater than N2
//	N1 -ge N2   N1 is greater than or equal to N2
//
// Logical operators:
//
//	! EXPR          EXPR is false
//	EXPR1 -a EXPR2  both EXPR1 and EXPR2 are true
//	EXPR1 -o EXPR2  either EXPR1 or EXPR2 is true
//	( EXPR )        grouping (parentheses must be shell-escaped)
package testcmd

import (
	"context"
	"io/fs"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the "test" builtin command registration.
var Cmd = builtins.Command{
	Name:        "test",
	Description: "evaluate conditional expression",
	MakeFlags:   builtins.NoFlags(runTest),
}

// BracketCmd is the "[" builtin command registration.
var BracketCmd = builtins.Command{
	Name:        "[",
	Description: "evaluate conditional expression",
	MakeFlags:   builtins.NoFlags(runBracket),
}

const helpText = `Usage: test EXPRESSION
   or: [ EXPRESSION ]

Evaluate conditional expression.

Exit status:
  0  if EXPRESSION is true,
  1  if EXPRESSION is false,
  2  if an error occurred.

File tests:
  -a FILE   FILE exists (deprecated synonym for -e)
  -e FILE   FILE exists
  -f FILE   FILE is a regular file
  -d FILE   FILE is a directory
  -s FILE   FILE has size > 0
  -r FILE   FILE is readable
  -w FILE   FILE is writable
  -x FILE   FILE is executable
  -h FILE   FILE is a symbolic link
  -L FILE   FILE is a symbolic link (same as -h)
  -p FILE   FILE is a named pipe

File comparison:
  FILE1 -nt FILE2   FILE1 is newer than FILE2
  FILE1 -ot FILE2   FILE1 is older than FILE2

String tests:
  -z STRING         STRING has zero length
  -n STRING         STRING has non-zero length
  STRING            STRING is non-empty

String comparison:
  S1 = S2    strings are equal
  S1 == S2   strings are equal (synonym for =)
  S1 != S2   strings are not equal
  S1 < S2    S1 sorts before S2
  S1 > S2    S1 sorts after S2

Integer comparison:
  N1 -eq N2  N1 equals N2
  N1 -ne N2  N1 is not equal to N2
  N1 -lt N2  N1 is less than N2
  N1 -le N2  N1 is less or equal to N2
  N1 -gt N2  N1 is greater than N2
  N1 -ge N2  N1 is greater or equal to N2

Logical:
  ! EXPR            EXPR is false
  EXPR1 -a EXPR2   both true
  EXPR1 -o EXPR2   either true
  ( EXPR )         grouping
`

const exitSyntaxError = 2

func runTest(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	if len(args) == 1 && args[0] == "--help" {
		callCtx.Out(helpText)
		return builtins.Result{}
	}
	return evaluate(ctx, callCtx, "test", args)
}

func runBracket(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	if len(args) == 1 && args[0] == "--help" {
		callCtx.Out(helpText)
		return builtins.Result{}
	}
	if len(args) == 0 || args[len(args)-1] != "]" {
		callCtx.Errf("[: missing `]'\n")
		return builtins.Result{Code: exitSyntaxError}
	}
	return evaluate(ctx, callCtx, "[", args[:len(args)-1])
}

// parser holds the state for recursive-descent parsing of test expressions.
type parser struct {
	ctx     context.Context
	callCtx *builtins.CallContext
	cmdName string
	args    []string
	pos     int
	err     bool
	depth   int
	// subexprStart marks the beginning of the current subexpression for
	// POSIX disambiguation. It is set to 0 initially and updated when
	// entering a new subexpression boundary (after ! negation or inside
	// parentheses). subexprEnd marks the exclusive end of the current
	// subexpression (defaults to len(args), set to the position of ")"
	// inside parenthesized groups). The 3-arg disambiguation rule fires
	// when the subexpression length (subexprEnd - subexprStart) is exactly
	// 3, preventing it from triggering inside parseAnd/parseOr chains
	// while still allowing it inside nested ! or (...) contexts.
	subexprStart int
	subexprEnd   int
}

const maxParenDepth = 128

func evaluate(ctx context.Context, callCtx *builtins.CallContext, cmdName string, args []string) builtins.Result {
	if len(args) == 0 {
		return builtins.Result{Code: 1}
	}

	p := &parser{
		ctx:        ctx,
		callCtx:    callCtx,
		cmdName:    cmdName,
		args:       args,
		subexprEnd: len(args),
	}

	result := p.parseOr()

	if p.err {
		return builtins.Result{Code: exitSyntaxError}
	}
	if p.pos < len(p.args) {
		p.callCtx.Errf("%s: too many arguments\n", p.cmdName)
		return builtins.Result{Code: exitSyntaxError}
	}
	if result {
		return builtins.Result{}
	}
	return builtins.Result{Code: 1}
}

func (p *parser) peek() string {
	if p.pos >= len(p.args) {
		return ""
	}
	return p.args[p.pos]
}

func (p *parser) advance() string {
	s := p.args[p.pos]
	p.pos++
	return s
}

// parseOr handles EXPR1 -o EXPR2 (lowest precedence).
func (p *parser) parseOr() bool {
	left := p.parseAnd()
	for !p.err && p.ctx.Err() == nil && p.pos < len(p.args) && p.peek() == "-o" {
		p.advance()
		right := p.parseAnd()
		left = left || right
	}
	return left
}

// parseAnd handles EXPR1 -a EXPR2.
func (p *parser) parseAnd() bool {
	left := p.parseNot()
	for !p.err && p.ctx.Err() == nil && p.pos < len(p.args) && p.peek() == "-a" {
		p.advance()
		right := p.parseNot()
		left = left && right
	}
	return left
}

// parseNot handles ! EXPR. When ! is the last remaining token, it is
// treated as a non-empty string per POSIX single-argument rules.
// When ! is followed by a binary operator (e.g., "! = !"), it is treated
// as a literal string operand, not negation.
func (p *parser) parseNot() bool {
	if p.pos < len(p.args) && p.peek() == "!" {
		// When "!" is the only token in the current subexpression, treat
		// it as a non-empty string per POSIX single-argument rules.
		// We use subexpression bounds (not global remaining count) so
		// that "!" after -a/-o in a larger expression is still treated
		// as negation requiring an operand. e.g.:
		//   test !          → "!" is non-empty string → exit 0
		//   test -n x -a !  → "!" is negation, missing arg → exit 2
		//   test x -a !     → 3-arg rule handles it as binary -a
		if p.subexprEnd-p.subexprStart == 1 {
			p.advance()
			return true
		}
		// POSIX 3-arg rule: if the current subexpression has exactly 3
		// tokens and "!" is followed by a binary operator, treat "!" as a
		// literal string operand (fall through to parsePrimary for binary
		// expression). We use subexprStart to scope this to the current
		// subexpression, so it fires for both top-level 3-arg forms and
		// nested ones (e.g., "test ! ! = !") but not inside -a/-o chains.
		subexprLen := p.subexprEnd - p.subexprStart
		if subexprLen == 3 && isBinaryOpOrLogical(p.args[p.pos+1]) {
			return p.parsePrimary()
		}
		if p.depth >= maxParenDepth {
			p.callCtx.Errf("%s: expression too deeply nested\n", p.cmdName)
			p.err = true
			return false
		}
		p.depth++
		p.advance()
		saved := p.subexprStart
		p.subexprStart = p.pos // new subexpression after !
		result := !p.parseNot()
		p.subexprStart = saved
		p.depth--
		return result
	}
	return p.parsePrimary()
}

// parsePrimary handles parenthesized expressions, unary operators, binary
// operators, and bare strings. It uses lookahead to implement the POSIX
// disambiguation rules (e.g., with 1 remaining arg, everything is a string).
func (p *parser) parsePrimary() bool {
	if p.err || p.pos >= len(p.args) {
		p.callCtx.Errf("%s: missing argument\n", p.cmdName)
		p.err = true
		return false
	}

	cur := p.peek()
	// Use the subexpression boundary (not len(args)) so that lookahead
	// inside parenthesized groups does not read past the closing ')'.
	// At the top level subexprEnd == len(args); inside (...) it points
	// to the position of the matching ')'.
	remaining := p.subexprEnd - p.pos

	// POSIX 3-arg rule: when the subexpression is exactly "( X )" and X is
	// NOT a binary operator, treat the middle token as a string non-emptiness
	// test. This prevents bash-compat issues where X is "!", "-n", etc. that
	// would be misinterpreted as operators inside a group. e.g.,
	//   test "(" "!" ")" → 0 (non-empty string "!")
	//   test "(" "" ")" → 1 (empty string)
	// When X IS a binary operator (e.g., "="), the isThreeArgBinary check
	// below handles it as "(" = ")" (string comparison).
	subexprLen := p.subexprEnd - p.subexprStart
	if cur == "(" && subexprLen == 3 && p.pos+2 < len(p.args) && p.args[p.pos+2] == ")" && !isBinaryOpOrLogical(p.args[p.pos+1]) {
		p.advance() // skip "("
		s := p.advance()
		p.advance() // skip ")"
		return s != ""
	}

	// POSIX 4-arg rule: when the subexpression is exactly "( X Y )" where
	// the first token is "(" and the last is ")", evaluate the inner 2 tokens
	// as a 2-arg expression. This prevents findMatchingParen from incorrectly
	// matching a literal ")" in the data. e.g.,
	//   test "(" "!" ")" ")" → inner "! )" → NOT non-empty ")" → false → exit 1
	//   test "(" "-n" "x" ")" → inner "-n x" → true → exit 0
	if cur == "(" && subexprLen == 4 && p.pos+3 < len(p.args) && p.args[p.pos+3] == ")" {
		p.advance() // skip "("
		savedStart := p.subexprStart
		savedEnd := p.subexprEnd
		p.subexprStart = p.pos
		p.subexprEnd = p.pos + 2 // inner 2 tokens
		result := p.parseOr()
		p.subexprStart = savedStart
		p.subexprEnd = savedEnd
		if p.err {
			return false
		}
		if p.pos >= len(p.args) || p.peek() != ")" {
			p.callCtx.Errf("%s: missing ')'\n", p.cmdName)
			p.err = true
			return false
		}
		p.advance() // skip ")"
		return result
	}

	// Treat "(" as grouping when there are tokens after it, or when it
	// appears as the last token inside a compound expression (subexprLen > 1).
	// A lone "(" as the only argument (subexprLen == 1) is a bare non-empty
	// string per POSIX single-argument rules. When "(" is followed by a
	// binary operator (e.g., "(" = "("), treat it as a literal string operand.
	// In compound expressions like "test -f x -o (", the lone "(" triggers
	// grouping which correctly fails with a missing argument error.
	if cur == "(" && (remaining > 1 || subexprLen > 1) && !p.isThreeArgBinary(p.pos) {
		if p.depth >= maxParenDepth {
			p.callCtx.Errf("%s: expression too deeply nested\n", p.cmdName)
			p.err = true
			return false
		}
		p.depth++
		p.advance()
		if p.pos >= len(p.args) || p.peek() == ")" {
			p.callCtx.Errf("%s: missing argument\n", p.cmdName)
			p.err = true
			return false
		}
		savedStart := p.subexprStart
		savedEnd := p.subexprEnd
		p.subexprStart = p.pos // new subexpression inside parens
		// Find matching ')' to set the subexpression end boundary.
		// This allows the 3-arg disambiguation rule to correctly
		// count only tokens between '(' and ')'.
		p.subexprEnd = p.findMatchingParen(p.pos)
		result := p.parseOr()
		p.subexprStart = savedStart
		p.subexprEnd = savedEnd
		p.depth--
		if p.err {
			return false
		}
		if p.pos >= len(p.args) || p.peek() != ")" {
			p.callCtx.Errf("%s: missing ')'\n", p.cmdName)
			p.err = true
			return false
		}
		p.advance()
		return result
	}

	// POSIX disambiguation: if there are at least 3 remaining tokens and
	// the second one is a binary operator, parse as a binary expression.
	// This must be checked before unary operators to handle cases like
	// "test -f = -f" (string comparison, not file test).
	if remaining >= 3 {
		op := p.args[p.pos+1]
		if isBinaryOp(op) {
			return p.parseBinaryExpr()
		}
		// POSIX 3-arg rule: when the current subexpression has exactly 3
		// tokens and the middle token is -a/-o, treat as binary AND/OR
		// with string operands. e.g., "test -f -a -d" → "-f" AND "-d".
		// We use subexprStart (not remaining) so this fires for nested
		// subexpressions after ! but not inside -a/-o chains.
		subexprLen := p.subexprEnd - p.subexprStart
		if subexprLen == 3 && (op == "-a" || op == "-o") {
			return p.parseBinaryExpr()
		}
	}

	// With 2+ remaining tokens, check for unary operators.
	if remaining >= 2 {
		if isUnaryFileOp(cur) {
			return p.parseUnaryFileOp()
		}
		if cur == "-z" || cur == "-n" {
			return p.parseUnaryStringOp()
		}
		// -o as unary tests whether a shell option is set.
		// This restricted shell has no options, so always false.
		if cur == "-o" {
			p.advance() // consume -o
			p.advance() // consume operand
			return false
		}
	}

	// Single token or unrecognised: treat as bare string (true if non-empty).
	p.advance()
	return cur != ""
}

func isBinaryOp(op string) bool {
	switch op {
	case "=", "==", "!=", "<", ">",
		"-eq", "-ne", "-lt", "-le", "-gt", "-ge",
		"-nt", "-ot":
		return true
	}
	return false
}

// isBinaryOpOrLogical returns true if op is a binary comparison operator
// or a logical operator (-a/-o) that can act as a binary operator in the
// POSIX 3-argument form.
func isBinaryOpOrLogical(op string) bool {
	return isBinaryOp(op) || op == "-a" || op == "-o"
}

// isThreeArgBinary returns true when the current subexpression has exactly 3
// tokens and the token at pos+1 is a binary or logical operator. This
// implements the POSIX 3-argument disambiguation rule. The subexpression
// length is computed from p.subexprStart (set at the top level and updated
// when entering ! negation), so the rule fires for both top-level 3-arg
// forms and nested ones (e.g., "test ! ! = !") but not inside -a/-o chains.
func (p *parser) isThreeArgBinary(pos int) bool {
	subexprLen := p.subexprEnd - p.subexprStart
	return subexprLen == 3 && pos+1 < len(p.args) && isBinaryOpOrLogical(p.args[pos+1])
}

// findMatchingParen scans forward from start to find the position of the
// matching ')' token, accounting for nested '(' ... ')' groups. If no
// matching ')' is found, it returns len(p.args) as a fallback (the parse
// will later report a "missing ')'" error).
func (p *parser) findMatchingParen(start int) int {
	depth := 1
	for i := start; i < len(p.args); i++ {
		switch p.args[i] {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(p.args)
}

func isUnaryFileOp(op string) bool {
	switch op {
	case "-a", "-e", "-f", "-d", "-s", "-r", "-w", "-x", "-h", "-L", "-p":
		return true
	}
	return false
}

func (p *parser) consumeUnaryOperand(op string) (string, bool) {
	if p.pos >= len(p.args) {
		p.callCtx.Errf("%s: '%s': unary operator expected\n", p.cmdName, builtins.SafeOperand(op))
		p.err = true
		return "", false
	}
	return p.advance(), true
}

func (p *parser) parseUnaryFileOp() bool {
	op := p.advance()
	operand, ok := p.consumeUnaryOperand(op)
	if !ok {
		return false
	}
	return p.evalFileTest(op, operand)
}

func (p *parser) parseUnaryStringOp() bool {
	op := p.advance()
	s, ok := p.consumeUnaryOperand(op)
	if !ok {
		return false
	}
	return (op == "-z") == (s == "")
}

func (p *parser) parseBinaryExpr() bool {
	left := p.advance()
	op := p.advance()
	if p.pos >= len(p.args) {
		p.callCtx.Errf("%s: missing argument after '%s'\n", p.cmdName, builtins.SafeOperand(op))
		p.err = true
		return false
	}
	right := p.advance()

	switch op {
	case "=", "==":
		return left == right
	case "!=":
		return left != right
	case "<":
		return left < right
	case ">":
		return left > right
	case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		return p.evalIntCompare(left, op, right)
	case "-nt", "-ot":
		return p.evalFileCompare(left, op, right)
	case "-a":
		return left != "" && right != ""
	case "-o":
		return left != "" || right != ""
	default:
		p.callCtx.Errf("%s: unknown binary operator '%s'\n", p.cmdName, builtins.SafeOperand(op))
		p.err = true
		return false
	}
}

func (p *parser) evalFileTest(op, path string) bool {
	if path == "" {
		return false
	}
	// For -r/-w/-x, use real access checks instead of mode bits.
	if p.callCtx.AccessFile != nil {
		switch op {
		case "-r":
			return p.callCtx.AccessFile(p.ctx, path, 0x04) == nil
		case "-w":
			return p.callCtx.AccessFile(p.ctx, path, 0x02) == nil
		case "-x":
			return p.callCtx.AccessFile(p.ctx, path, 0x01) == nil
		}
	}

	switch op {
	case "-h", "-L":
		if p.callCtx.LstatFile == nil {
			return false
		}
		info, err := p.callCtx.LstatFile(p.ctx, path)
		if err != nil {
			return false
		}
		return info.Mode()&fs.ModeSymlink != 0
	default:
		if p.callCtx.StatFile == nil {
			return false
		}
		info, err := p.callCtx.StatFile(p.ctx, path)
		if err != nil {
			return false
		}
		return evalFileInfo(op, info)
	}
}

func evalFileInfo(op string, info fs.FileInfo) bool {
	switch op {
	case "-a", "-e":
		return true
	case "-f":
		return info.Mode().IsRegular()
	case "-d":
		return info.IsDir()
	case "-s":
		return info.Size() > 0
	case "-r":
		// NOTE: This fallback checks any permission bit (user/group/other) and does not
		// account for file ownership. In production AccessFile is always set and this path
		// is not reached; actual file access still goes through the sandbox.
		return info.Mode().Perm()&0444 != 0
	case "-w":
		return info.Mode().Perm()&0222 != 0
	case "-x":
		return info.Mode().Perm()&0111 != 0
	case "-p":
		return info.Mode()&fs.ModeNamedPipe != 0
	}
	return false
}

func (p *parser) evalIntCompare(left, op, right string) bool {
	l, ok := p.parseInt(left)
	if !ok {
		return false
	}
	r, ok := p.parseInt(right)
	if !ok {
		return false
	}
	switch op {
	case "-eq":
		return l == r
	case "-ne":
		return l != r
	case "-lt":
		return l < r
	case "-le":
		return l <= r
	case "-gt":
		return l > r
	case "-ge":
		return l >= r
	}
	return false
}

func (p *parser) parseInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		p.callCtx.Errf("%s: : integer expression expected\n", p.cmdName)
		p.err = true
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		p.callCtx.Errf("%s: %s: integer expression expected\n", p.cmdName, s)
		p.err = true
		return 0, false
	}
	return n, true
}

func (p *parser) evalFileCompare(left, op, right string) bool {
	if p.callCtx.StatFile == nil {
		return false
	}
	leftInfo, leftErr := p.callCtx.StatFile(p.ctx, left)
	rightInfo, rightErr := p.callCtx.StatFile(p.ctx, right)

	switch op {
	case "-nt":
		if leftErr != nil {
			return false
		}
		if rightErr != nil {
			return true
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	case "-ot":
		if rightErr != nil {
			return false
		}
		if leftErr != nil {
			return true
		}
		return leftInfo.ModTime().Before(rightInfo.ModTime())
	}
	return false
}
