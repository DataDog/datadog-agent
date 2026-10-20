// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// AST limits to prevent resource exhaustion.
const (
	maxExprDepth = 64
	maxExprNodes = 256
)

// exprKind identifies the type of expression node.
type exprKind int

const (
	exprName    exprKind = iota // -name pattern
	exprIName                   // -iname pattern
	exprPath                    // -path pattern
	exprIPath                   // -ipath pattern
	exprType                    // -type c
	exprSize                    // -size n[cwbkMG]
	exprEmpty                   // -empty
	exprNewer                   // -newer file
	exprMtime                   // -mtime n
	exprMmin                    // -mmin n
	exprPerm                    // -perm mode
	exprQuit                    // -quit
	exprPrint                   // -print
	exprPrint0                  // -print0
	exprPrune                   // -prune
	exprTrue                    // -true
	exprFalse                   // -false
	exprExecDir                 // -execdir command {} ;
	exprExec                    // -exec command {} ;
	exprAnd                     // expr -a expr  or  expr expr (implicit)
	exprOr                      // expr -o expr
	exprNot                     // ! expr  or  -not expr
)

// cmpOp represents a comparison operator for numeric predicates.
type cmpOp int

const (
	cmpLess  cmpOp = -1
	cmpExact cmpOp = 0
	cmpMore  cmpOp = 1
)

func (c cmpOp) String() string {
	switch c {
	case cmpLess:
		return "-N"
	case cmpExact:
		return "N"
	case cmpMore:
		return "+N"
	default:
		return "unknown"
	}
}

// sizeUnit holds a parsed -size predicate value.
type sizeUnit struct {
	n    int64 // magnitude (always positive)
	cmp  cmpOp // comparison operator
	unit byte  // one of: c w b k M G (default 'b' if omitted)
}

// expr is a node in the find expression AST.
type expr struct {
	kind     exprKind
	strVal   string   // pattern for name/iname/path/ipath, type char, file path for newer/samefile, format for printf
	sizeVal  sizeUnit // for -size
	numVal   int64    // for -mtime, -mmin, -atime, -amin, -ctime, -cmin, -uid, -gid, -links, -inum
	numCmp   cmpOp    // comparison operator for numeric predicates
	permVal  uint32   // for -perm: permission bits
	permCmp  byte     // for -perm: '=' exact, '-' all bits, '/' any bit
	execCmd  string   // command name for -exec/-execdir
	execArgs []string // argument template for -exec/-execdir (each element is literal or "{}")
	left     *expr    // for and/or
	right    *expr    // for and/or
	operand  *expr    // for not
}

// isAction returns true if this expression is an output action.
// Only actual output actions suppress implicit -print; -quit is
// control flow (handled at evaluation time by checking quit before
// implicit print) and does not affect the implicit-print decision.
func (e *expr) isAction() bool {
	return e.kind == exprPrint || e.kind == exprPrint0 || e.kind == exprExecDir || e.kind == exprExec
}

// hasAction checks if any node in the expression tree is an action.
func hasAction(e *expr) bool {
	if e == nil {
		return false
	}
	if e.isAction() {
		return true
	}
	return hasAction(e.left) || hasAction(e.right) || hasAction(e.operand)
}

// parser is a recursive-descent parser for find expressions.
type parser struct {
	args     []string
	pos      int
	depth    int
	nodes    int
	maxDepth int // -1 = not specified
	minDepth int // -1 = not specified
}

// parseResult holds the output of parseExpression.
type parseResult struct {
	expr     *expr
	maxDepth int // -1 = not specified
	minDepth int // -1 = not specified
}

// errHelpRequested is a sentinel error returned by parsePrimary when --help
// appears as a standalone predicate token (not consumed as an argument by
// another predicate like -name).
var errHelpRequested = errors.New("find: help requested")

// blocked predicates that are forbidden for sandbox safety.
var blockedPredicates = map[string]string{
	"-delete":  "file deletion is blocked",
	"-ok":      "interactive execution is blocked",
	"-okdir":   "interactive execution is blocked",
	"-fls":     "file writes are blocked",
	"-fprint":  "file writes are blocked",
	"-fprint0": "file writes are blocked",
	"-fprintf": "file writes are blocked",
	"-regex":   "regular expressions are blocked (ReDoS risk)",
	"-iregex":  "regular expressions are blocked (ReDoS risk)",
}

// parseExpression parses the find expression from args, including
// -maxdepth/-mindepth which are integrated into the recursive-descent parser.
// This avoids the argument-stealing problem: each predicate's own argument
// consumption naturally prevents depth options from capturing tokens that
// belong to other predicates (e.g. "find . -name -maxdepth" correctly treats
// "-maxdepth" as the -name pattern, not as a depth option).
func parseExpression(args []string) (parseResult, error) {
	if len(args) == 0 {
		return parseResult{maxDepth: -1, minDepth: -1}, nil
	}

	p := &parser{args: args, maxDepth: -1, minDepth: -1}
	e, err := p.parseOr()
	if err != nil {
		return parseResult{}, err
	}
	if p.pos < len(p.args) {
		return parseResult{}, fmt.Errorf("find: unexpected argument '%s'", builtins.SafeOperand(p.args[p.pos]))
	}
	return parseResult{expr: e, maxDepth: p.maxDepth, minDepth: p.minDepth}, nil
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

func (p *parser) expect(s string) error {
	if p.pos >= len(p.args) {
		return fmt.Errorf("find: expected '%s'", s)
	}
	if p.args[p.pos] != s {
		return fmt.Errorf("find: expected '%s', got '%s'", s, builtins.SafeOperand(p.args[p.pos]))
	}
	p.pos++
	return nil
}

func (p *parser) addNode() error {
	p.nodes++
	if p.nodes > maxExprNodes {
		return errors.New("find: expression too complex (too many nodes)")
	}
	return nil
}

// parseOr handles: expr -o expr
func (p *parser) parseOr() (*expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "-o" || p.peek() == "-or" {
		p.advance()
		if err := p.addNode(); err != nil {
			return nil, err
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &expr{kind: exprOr, left: left, right: right}
	}
	return left, nil
}

// parseAnd handles: expr -a expr  or  expr expr (implicit AND)
func (p *parser) parseAnd() (*expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.peek()
		if tok == "-a" || tok == "-and" {
			p.advance()
		} else if tok == "" || tok == "-o" || tok == "-or" || tok == ")" {
			break
		}
		if err := p.addNode(); err != nil {
			return nil, err
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &expr{kind: exprAnd, left: left, right: right}
	}
	return left, nil
}

// parseUnary handles: ! expr  or  -not expr  or  ( expr )  or  primary
func (p *parser) parseUnary() (*expr, error) {
	tok := p.peek()
	if tok == "!" || tok == "-not" {
		p.advance()
		p.depth++
		if p.depth > maxExprDepth {
			return nil, errors.New("find: expression too deeply nested")
		}
		if err := p.addNode(); err != nil {
			return nil, err
		}
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		p.depth--
		return &expr{kind: exprNot, operand: operand}, nil
	}
	if tok == "(" {
		p.advance()
		if p.peek() == ")" {
			return nil, errors.New("find: invalid expression; empty parentheses are not allowed.")
		}
		p.depth++
		if p.depth > maxExprDepth {
			return nil, errors.New("find: expression too deeply nested")
		}
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.depth--
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		return e, nil
	}
	return p.parsePrimary()
}

// parsePrimary handles leaf predicates.
func (p *parser) parsePrimary() (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, errors.New("find: expected expression")
	}

	if err := p.addNode(); err != nil {
		return nil, err
	}

	tok := p.advance()

	// --help as a standalone predicate triggers help output.
	if tok == "--help" {
		return nil, errHelpRequested
	}

	// Check blocked predicates.
	if reason, blocked := blockedPredicates[tok]; blocked {
		return nil, fmt.Errorf("find: %s: %s", tok, reason)
	}

	switch tok {
	case "-name":
		return p.parseStringPredicate(exprName)
	case "-iname":
		return p.parseStringPredicate(exprIName)
	case "-path", "-wholename":
		return p.parsePathPredicate(exprPath)
	case "-ipath", "-iwholename":
		return p.parsePathPredicate(exprIPath)
	case "-type":
		return p.parseTypePredicate()
	case "-size":
		return p.parseSizePredicate()
	case "-empty":
		return &expr{kind: exprEmpty}, nil
	case "-newer":
		return p.parsePathPredicate(exprNewer)
	case "-mtime":
		return p.parseNumericPredicate(exprMtime)
	case "-mmin":
		return p.parseNumericPredicate(exprMmin)
	case "-perm":
		return p.parsePermPredicate()
	case "-quit":
		return &expr{kind: exprQuit}, nil
	case "-print":
		return &expr{kind: exprPrint}, nil
	case "-print0":
		return &expr{kind: exprPrint0}, nil
	case "-prune":
		return &expr{kind: exprPrune}, nil
	case "-maxdepth":
		return p.parseDepthOption(true)
	case "-mindepth":
		return p.parseDepthOption(false)
	case "-exec":
		return p.parseExecPredicate()
	case "-execdir":
		return p.parseExecDirPredicate()
	case "-true":
		return &expr{kind: exprTrue}, nil
	case "-false":
		return &expr{kind: exprFalse}, nil
	default:
		return nil, fmt.Errorf("find: unknown predicate '%s'", builtins.SafeOperand(tok))
	}
}

func (p *parser) parseStringPredicate(kind exprKind) (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, fmt.Errorf("find: missing argument for %s", kind.String())
	}
	val := p.advance()
	return &expr{kind: kind, strVal: val}, nil
}

// parsePathPredicate is like parseStringPredicate but normalizes the value
// with filepath.ToSlash so that backslash path separators on Windows are
// converted to forward slashes, matching the internal path representation.
// Used for -path, -ipath, and -newer (all of which take filesystem paths
// or path-glob patterns as arguments).
func (p *parser) parsePathPredicate(kind exprKind) (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, fmt.Errorf("find: missing argument for %s", kind.String())
	}
	val := filepath.ToSlash(p.advance())
	return &expr{kind: kind, strVal: val}, nil
}

func (p *parser) parseTypePredicate() (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, errors.New("find: missing argument for -type")
	}
	val := p.advance()
	if len(val) == 0 {
		return nil, errors.New("find: Unknown argument to -type: ")
	}
	// Validate type character(s). GNU find allows comma-separated types
	// like "f,d" but rejects malformed lists like ",", "f,", ",d", or "fd".
	expectType := true
	for i := 0; i < len(val); i++ {
		c := val[i]
		if c == ',' {
			if expectType {
				// Leading or consecutive comma.
				return nil, fmt.Errorf("find: Unknown argument to -type: %s", builtins.SafeOperand(val))
			}
			expectType = true
			continue
		}
		switch c {
		case 'b', 'c', 'f', 'd', 'l', 'p', 's':
			if !expectType {
				// Adjacent type chars without comma (e.g. "fd").
				return nil, fmt.Errorf("find: Unknown argument to -type: %s", builtins.SafeOperand(val))
			}
			expectType = false
		default:
			return nil, fmt.Errorf("find: Unknown argument to -type: %s", builtins.SafeOperand(val))
		}
	}
	if expectType {
		// Trailing comma.
		return nil, fmt.Errorf("find: Unknown argument to -type: %s", builtins.SafeOperand(val))
	}
	return &expr{kind: exprType, strVal: val}, nil
}

func (p *parser) parseSizePredicate() (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, errors.New("find: missing argument for -size")
	}
	val := p.advance()
	su, err := parseSize(val)
	if err != nil {
		return nil, err
	}
	return &expr{kind: exprSize, sizeVal: su}, nil
}

func (p *parser) parseNumericPredicate(kind exprKind) (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, fmt.Errorf("find: missing argument for %s", kind.String())
	}
	val := p.advance()
	cmp := cmpExact
	numStr := val
	if strings.HasPrefix(numStr, "+") {
		cmp = cmpMore
		numStr = numStr[1:]
	} else if strings.HasPrefix(numStr, "-") {
		cmp = cmpLess
		numStr = numStr[1:]
	}
	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		// If the number overflows int64 but is otherwise valid, clamp to
		// MaxInt64. The evaluation functions handle huge values correctly:
		// +huge → nothing matches, -huge → everything matches, exact → no
		// match. This matches GNU find behavior for very large arguments.
		if errors.Is(err, strconv.ErrRange) {
			n = math.MaxInt64
			err = nil
		}
		if err != nil {
			return nil, fmt.Errorf("find: invalid argument '%s' to %s", builtins.SafeOperand(val), kind.String())
		}
	}
	return &expr{kind: kind, numVal: n, numCmp: cmp}, nil
}

func (p *parser) parseDepthOption(isMax bool) (*expr, error) {
	name := "-mindepth"
	if isMax {
		name = "-maxdepth"
	}
	if p.pos >= len(p.args) {
		return nil, fmt.Errorf("find: missing argument to '%s'", name)
	}
	val := p.advance()
	// Reject non-decimal forms like "+1" or "-1" that strconv.Atoi accepts.
	// GNU find requires a positive decimal integer.
	if len(val) > 0 && (val[0] == '+' || val[0] == '-') {
		return nil, fmt.Errorf("find: invalid argument '%s' to %s", builtins.SafeOperand(val), name)
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("find: invalid argument '%s' to %s", builtins.SafeOperand(val), name)
	}
	if isMax {
		p.maxDepth = n
	} else {
		p.minDepth = n
	}
	return &expr{kind: exprTrue}, nil
}

// parsePermPredicate parses -perm MODE where MODE is:
//   - 0644     exact match (octal)
//   - -0644    all bits must be set
//   - /0644    any bit must be set
//   - u=rwx,g=rx,o=rx  symbolic mode (parsed to octal)
func (p *parser) parsePermPredicate() (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, errors.New("find: missing argument for -perm")
	}
	val := p.advance()
	if len(val) == 0 {
		return nil, errors.New("find: missing argument for -perm")
	}

	var cmpMode byte = '=' // default: exact match
	modeStr := val
	if modeStr[0] == '-' {
		cmpMode = '-'
		modeStr = modeStr[1:]
	} else if modeStr[0] == '/' {
		cmpMode = '/'
		modeStr = modeStr[1:]
	}

	if len(modeStr) == 0 {
		return nil, fmt.Errorf("find: invalid mode '%s'", builtins.SafeOperand(val))
	}

	// Try octal parse first.
	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		// Try symbolic mode parse.
		mode64, serr := parseSymbolicMode(modeStr)
		if serr != nil {
			return nil, fmt.Errorf("find: invalid mode '%s'", builtins.SafeOperand(val))
		}
		mode = mode64
	}
	if mode > 07777 {
		return nil, fmt.Errorf("find: invalid mode '%s'", builtins.SafeOperand(val))
	}

	return &expr{kind: exprPerm, permVal: uint32(mode), permCmp: cmpMode}, nil
}

// parseSymbolicMode parses a symbolic permission string like "u=rwx,g=rx,o=rx"
// or "a=r" into an octal permission value.
//
// Supports: who (u/g/o/a), operators (=/+/-), perms (r/w/x/X/s/t),
// copy-bits (g=u, o=g, etc.), and conditional execute (X).
func parseSymbolicMode(s string) (uint64, error) {
	var mode uint64
	for _, clause := range strings.Split(s, ",") {
		if len(clause) == 0 {
			return 0, fmt.Errorf("empty clause")
		}
		// Parse who: u, g, o, a (default: a)
		i := 0
		who := byte(0) // bitmask: 4=u, 2=g, 1=o
	whoLoop:
		for i < len(clause) {
			switch clause[i] {
			case 'u':
				who |= 4
			case 'g':
				who |= 2
			case 'o':
				who |= 1
			case 'a':
				who |= 7
			default:
				break whoLoop
			}
			i++
		}
		if who == 0 {
			who = 7 // default: a (all)
		}
		if i >= len(clause) {
			return 0, fmt.Errorf("missing operator")
		}
		op := clause[i]
		if op != '=' && op != '+' && op != '-' {
			return 0, fmt.Errorf("invalid operator '%c'", op)
		}
		i++
		// Parse perms: r, w, x, X, s, t, or copy-bits (u/g/o).
		var bits uint64    // rwx bits (applied per-class)
		var special uint64 // special bits (setuid/setgid/sticky, applied globally)

		// Check for copy-bits source (u/g/o). Copy-bits must be the
		// sole perm token — no mixing with rwxXst (GNU rejects "g=ur").
		if i < len(clause) && (clause[i] == 'u' || clause[i] == 'g' || clause[i] == 'o') {
			switch clause[i] {
			case 'u':
				bits = uint64((mode >> 6) & 7)
			case 'g':
				bits = uint64((mode >> 3) & 7)
			case 'o':
				bits = uint64(mode & 7)
			}
			i++
			if i < len(clause) {
				return 0, fmt.Errorf("invalid permission '%c'", clause[i])
			}
		} else {
			for i < len(clause) {
				switch clause[i] {
				case 'r':
					bits |= 4
				case 'w':
					bits |= 2
				case 'x':
					bits |= 1
				case 'X':
					// Conditional execute: set x only if the mode
					// being built already has any execute bit set.
					if mode&0111 != 0 {
						bits |= 1
					}
				case 's':
					// setuid for user, setgid for group
					if who&4 != 0 {
						special |= 0o4000
					}
					if who&2 != 0 {
						special |= 0o2000
					}
				case 't':
					if who&1 != 0 {
						special |= 0o1000 // sticky
					}
				default:
					return 0, fmt.Errorf("invalid permission '%c'", clause[i])
				}
				i++
			}
		}
		// Apply rwx bits to the appropriate class positions.
		// '=' clears all bits for the class first, then sets the new ones.
		// '+' only sets bits (OR). '-' only clears bits (AND NOT).
		if who&4 != 0 { // user
			switch op {
			case '=':
				mode &^= 7 << 6   // clear user bits
				mode |= bits << 6 // set new bits
			case '+':
				mode |= bits << 6
			case '-':
				mode &^= bits << 6
			}
		}
		if who&2 != 0 { // group
			switch op {
			case '=':
				mode &^= 7 << 3
				mode |= bits << 3
			case '+':
				mode |= bits << 3
			case '-':
				mode &^= bits << 3
			}
		}
		if who&1 != 0 { // other
			switch op {
			case '=':
				mode &^= 7
				mode |= bits
			case '+':
				mode |= bits
			case '-':
				mode &^= bits
			}
		}
		// Apply special bits (setuid/setgid/sticky).
		// For '=', clear the class's special bits first so that e.g.
		// "u=s,u=rwx" correctly drops setuid on the second clause.
		if op == '=' {
			if who&4 != 0 {
				mode &^= 0o4000 // clear setuid
			}
			if who&2 != 0 {
				mode &^= 0o2000 // clear setgid
			}
			// sticky is associated with 'other' or 'all'
			if who&1 != 0 {
				mode &^= 0o1000 // clear sticky
			}
		}
		switch op {
		case '=', '+':
			mode |= special
		case '-':
			mode &^= special
		}
	}
	return mode, nil
}

func (p *parser) parseExecPredicate() (*expr, error) {
	return p.parseExecLikePredicate(exprExec, "-exec")
}

func (p *parser) parseExecDirPredicate() (*expr, error) {
	return p.parseExecLikePredicate(exprExecDir, "-execdir")
}

// parseExecLikePredicate parses -exec/-execdir command [args...] ;
// Only \; mode is supported. {} + (batch mode) is rejected with a clear error.
// A literal "+" that does not follow "{}" is treated as a normal argument.
func (p *parser) parseExecLikePredicate(kind exprKind, name string) (*expr, error) {
	if p.pos >= len(p.args) {
		return nil, fmt.Errorf("find: missing argument to '%s'", name)
	}

	// Collect tokens until ";" terminator, or "+" after "{}" (batch mode).
	// In find syntax, "+" is only special as a terminator in the {} + form;
	// otherwise it is a normal argument (e.g. -exec echo + {} \;).
	startPos := p.pos
	for p.pos < len(p.args) {
		tok := p.args[p.pos]
		if tok == ";" {
			break
		}
		if tok == "+" && p.pos > startPos && p.args[p.pos-1] == "{}" {
			return nil, fmt.Errorf("find: %s ... + (batch mode) is not yet supported", name)
		}
		p.pos++
	}

	if p.pos >= len(p.args) {
		return nil, fmt.Errorf("find: missing argument to '%s'", name)
	}

	// Consume the ";" terminator.
	p.pos++

	tokens := p.args[startPos : p.pos-1]
	if len(tokens) == 0 {
		return nil, fmt.Errorf("find: missing argument to '%s'", name)
	}

	cmd := tokens[0]
	args := tokens[1:]

	return &expr{kind: kind, execCmd: cmd, execArgs: args}, nil
}

// parseSize parses a -size argument like "+10k", "-5M", "100c".
func parseSize(s string) (sizeUnit, error) {
	if len(s) == 0 {
		return sizeUnit{}, errors.New("find: invalid argument '' to -size")
	}
	var su sizeUnit

	numStr := s
	if s[0] == '+' {
		su.cmp = cmpMore
		numStr = s[1:]
	} else if s[0] == '-' {
		su.cmp = cmpLess
		numStr = s[1:]
	}

	if len(numStr) == 0 {
		return sizeUnit{}, fmt.Errorf("find: invalid argument '%s' to -size", builtins.SafeOperand(s))
	}

	// Check for unit suffix.
	su.unit = 'b' // default: 512-byte blocks
	last := numStr[len(numStr)-1]
	switch last {
	case 'c', 'w', 'b', 'k', 'M', 'G':
		su.unit = last
		numStr = numStr[:len(numStr)-1]
	}

	if len(numStr) == 0 {
		return sizeUnit{}, fmt.Errorf("find: invalid argument '%s' to -size", builtins.SafeOperand(s))
	}

	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return sizeUnit{}, fmt.Errorf("find: invalid argument '%s' to -size", builtins.SafeOperand(s))
	}
	if n < 0 {
		return sizeUnit{}, fmt.Errorf("find: invalid argument '%s' to -size", builtins.SafeOperand(s))
	}
	su.n = n
	return su, nil
}

func (k exprKind) String() string {
	switch k {
	case exprName:
		return "-name"
	case exprIName:
		return "-iname"
	case exprPath:
		return "-path"
	case exprIPath:
		return "-ipath"
	case exprType:
		return "-type"
	case exprSize:
		return "-size"
	case exprEmpty:
		return "-empty"
	case exprNewer:
		return "-newer"
	case exprMtime:
		return "-mtime"
	case exprMmin:
		return "-mmin"
	case exprPerm:
		return "-perm"
	case exprExecDir:
		return "-execdir"
	case exprExec:
		return "-exec"
	case exprQuit:
		return "-quit"
	case exprPrint:
		return "-print"
	case exprPrint0:
		return "-print0"
	case exprPrune:
		return "-prune"
	case exprTrue:
		return "-true"
	case exprFalse:
		return "-false"
	case exprAnd:
		return "-and"
	case exprOr:
		return "-or"
	case exprNot:
		return "-not"
	default:
		return "unknown"
	}
}
