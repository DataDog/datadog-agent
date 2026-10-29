// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// MaxNestingDepth bounds how deeply a script may nest shell constructs
// (parentheses, command/parameter substitutions, brace groups, pipelines,
// logical lists, and compound commands such as if/while/for/case).
//
// The bound exists purely for safety, not for compatibility: the recursive
// descent parser in mvdan.cc/sh, rshell's own recursive AST passes
// ([validateNode] via [syntax.Walk]), and the recursive executor all consume
// Go stack frames per nesting level. A sufficiently deep script — still well
// under [MaxScriptBytes] — grows the goroutine stack past the Go runtime's
// 1 GiB limit, which aborts the process with an *uncatchable* "fatal error:
// stack overflow". That throw is not a panic, so the recover() guard in
// [Runner.Run] cannot intercept it, and it happens before any command, path,
// mode, or timeout policy is consulted.
//
// The pre-parse lexical guard ([checkScriptNestingDepth]) measures this in
// source bracket depth; the post-parse guard ([checkNestingDepth]) measures it
// in AST node depth, which is a small constant multiple of the source nesting
// (each nested construct expands to a few AST levels). 5000 is far below the
// depth at which any of those recursions overflows (empirically the parser
// alone tolerates tens of thousands of bracket levels, and the crash is at
// hundreds of thousands) yet comfortably above the nesting depth of any
// realistic script — including long pipelines — so legitimate input is never
// rejected while the crash is prevented.
const MaxNestingDepth = 5000

// errNestingTooDeep is the error reported when a script exceeds
// [MaxNestingDepth]. It is shared by the pre-parse lexical guard and the
// post-parse AST guard so callers see a consistent message.
func errNestingTooDeep() error {
	return fmt.Errorf("script nesting exceeds maximum depth of %d; simplify deeply nested constructs", MaxNestingDepth)
}

// Frame kinds tracked by the lexical pre-scan's context stack. The top frame
// determines the scanner's mode; each frame also records whether it is a true
// nesting level (counted toward depth) or merely a lexical context.
type nestKind uint8

const (
	kindParen          nestKind = iota // ( ... ) / $( ... ) / <( ... ) — counted
	kindParam                          // ${ ... } — counted
	kindBrace                          // { ... ; } command group — counted
	kindBacktick                       // ` ... ` — counted
	kindIf                             // if ... (closed by fi) — counted
	kindElif                           // elif ... (part of an if ladder) — counted
	kindLoop                           // while/until/for/select (closed by done) — counted
	kindCase                           // case ... (closed by esac); on top: inside a case body — counted
	kindArith                          // $(( ... )) / (( ... )) — counted
	kindArithSq                        // $[ ... ] — counted
	kindArithParen                     // ( ... ) inside arithmetic — counted
	kindArithIndex                     // [ ... ] inside arithmetic, or an assignment's a[ ... ] index — counted
	kindPatGroup                       // ( ... ) extglob group inside a case pattern — counted
	kindSingle                         // ' ... ' — lexical only
	kindAnsi                           // $' ... ' — lexical only
	kindDouble                         // " ... " — lexical only
	kindHeredocExpand                  // unquoted heredoc body — lexical only
	kindHeredocLiteral                 // quoted heredoc body — lexical only
	kindCaseHead0                      // after "case", before the subject word — lexical only
	kindCaseHead1                      // after the case subject, before "in" — lexical only
	kindCasePat                        // a case pattern list, up to its ")" — lexical only
	kindLet                            // the arithmetic arguments of "let" — lexical only
	kindNone           = 0xff          // sentinel for "empty stack"
)

// counted reports whether a frame kind is a real nesting level that the parser
// descends into (and therefore must be bounded), as opposed to a quoting,
// heredoc, or grammar-position context that only changes how subsequent bytes
// are interpreted.
func (k nestKind) counted() bool {
	return k < kindSingle
}

func (k nestKind) isHeredoc() bool {
	return k == kindHeredocExpand || k == kindHeredocLiteral
}

type heredocInfo struct {
	delim     string
	stripTabs bool // <<- form: strip leading tabs before comparing the delimiter
	expand    bool // unquoted delimiter: body still performs $(...)/${...}/`...` expansion
}

// checkScriptNestingDepth is a lexical pre-scan run by [ParseScript] before the
// script text is handed to the recursive-descent parser. Its job is to stop the
// parser itself from overflowing the stack on deeply nested constructs —
// subshells and substitutions "(" / "$(" / "${" / "$((" / "$[", backquotes,
// brace command groups "{ … ; }", arithmetic parentheses and indexes, and the
// compound keyword commands if/elif, while/until/for/select, and case — each of
// which the parser descends into once per nesting level, before any AST (and
// therefore the post-parse [checkNestingDepth] guard) exists. Pipelines and
// &&/|| lists are not counted: the parser builds those iteratively, so they
// cannot overflow it; their depth is bounded after parsing by
// [checkNestingDepth].
//
// It is a quoting- and context-aware state machine over a stack of frames. The
// top frame selects the mode, so that bytes are only treated as structural
// syntax where mvdan.cc/sh's lexer would treat them so:
//
//   - single quotes and quoted heredoc bodies are fully literal; $'…' strings
//     are literal apart from backslash escapes;
//   - double quotes, unquoted heredoc bodies, and ${…} words are literal except
//     for "$(", "${", "$((", "$[" and backquotes, which still open nested
//     contexts (${…} additionally honours quotes);
//   - arithmetic — "$((…))", "((…))", "$[…]", "let" arguments, and the a[…]
//     index of an assignment — treats "<<" as a shift and keywords as plain
//     names, so neither can open a phantom heredoc or compound command;
//   - reserved words (if/fi/while/done/case/esac/…) and brace command groups
//     ("{" / "}") are recognised only in command position, including after
//     "!", "time", a case pattern's ")" and a function name, so a keyword- or
//     brace-looking *argument* (echo fi, echo }) is not mistaken for a closer;
//     continuation words (then/else/do) restore command position only inside
//     the compound they belong to;
//   - case pattern lists are scanned as patterns, not commands;
//   - heredoc delimiter lines are matched with a trailing CR removed, as the
//     parser does for CRLF scripts.
//
// The invariant that matters for safety is that the scan never undercounts:
// any byte the parser lexes as structure must not be skipped as literal text,
// and literal text must not be taken for a closer. Every context above exists
// because getting it wrong in one direction or the other lets a deeply nested
// script through (undercount) or rejects a valid one (overcount).
func checkScriptNestingDepth(script string) error {
	sc := nestScanner{s: script, lineStart: true}
	sc.startCmd()
	for sc.i < len(sc.s) {
		if err := sc.step(); err != nil {
			return err
		}
	}
	return nil
}

// nestScanner holds the lexical pre-scan state; see [checkScriptNestingDepth].
type nestScanner struct {
	s     string
	i     int
	kinds []nestKind
	depth int

	heredocs []heredocInfo // parallel stack for heredoc-body frames
	pending  []heredocInfo // heredocs declared on the current line, bodies not yet consumed

	lineStart bool // at the start of a line (heredoc delimiter lines are matched here)

	// Command-context state.
	cmdPos      bool // next word begins a command (so reserved words are recognised)
	wordStart   bool // next non-separator byte begins a new word
	argWords    int  // non-assignment, non-redirect words seen in the current simple command
	inDecl      bool // current simple command is declare/local/export/readonly/typeset/nameref
	redirTarget bool // next word is a redirect target
	afterTime   bool // previous word was the "time" keyword (so "-p" keeps command position)

	patStart  bool // in a case pattern list, nothing of the current pattern consumed yet
	prevSpace bool // in arithmetic, the previous byte was a blank (ends a "let" argument list before an operator)
}

func (sc *nestScanner) top() nestKind {
	if len(sc.kinds) == 0 {
		return kindNone
	}
	return sc.kinds[len(sc.kinds)-1]
}

func (sc *nestScanner) push(k nestKind) error {
	sc.kinds = append(sc.kinds, k)
	if !k.counted() {
		return nil
	}
	sc.depth++
	if sc.depth > MaxNestingDepth {
		return errNestingTooDeep()
	}
	return nil
}

func (sc *nestScanner) replaceTop(k nestKind) { sc.kinds[len(sc.kinds)-1] = k }

func (sc *nestScanner) pop() {
	k := sc.kinds[len(sc.kinds)-1]
	sc.kinds = sc.kinds[:len(sc.kinds)-1]
	if k.isHeredoc() {
		sc.heredocs = sc.heredocs[:len(sc.heredocs)-1]
	} else if k.counted() {
		sc.depth--
	}
}

// startCmd records that the next word begins a new simple command.
func (sc *nestScanner) startCmd() {
	sc.cmdPos = true
	sc.wordStart = true
	sc.argWords = 0
	sc.inDecl = false
	sc.redirTarget = false
	sc.afterTime = false
}

// startNextHeredoc begins the body of the next pending heredoc, if any and if a
// heredoc body is not already open on top. Bodies are consumed in the order
// declared.
func (sc *nestScanner) startNextHeredoc() {
	if len(sc.pending) == 0 || sc.top().isHeredoc() {
		return
	}
	info := sc.pending[0]
	sc.pending = sc.pending[1:]
	k := kindHeredocLiteral
	if info.expand {
		k = kindHeredocExpand
	}
	sc.kinds = append(sc.kinds, k)
	sc.heredocs = append(sc.heredocs, info)
}

func (sc *nestScanner) peek(off int) byte {
	if j := sc.i + off; j < len(sc.s) {
		return sc.s[j]
	}
	return 0
}

// step consumes at least one byte (or changes the frame stack) according to the
// mode selected by the top frame.
func (sc *nestScanner) step() error {
	k := sc.top()
	if sc.lineStart && k.isHeredoc() {
		if sc.heredocLine(k) {
			return nil
		}
		// Expanding body line: fall through to per-byte handling, which
		// treats it like a double-quoted context until the newline.
	}
	sc.lineStart = false
	switch k {
	case kindSingle:
		if sc.s[sc.i] == '\'' {
			sc.pop()
		}
		sc.i++
		return nil
	case kindAnsi:
		switch sc.s[sc.i] {
		case '\\':
			sc.i++
		case '\'':
			sc.pop()
		}
		sc.i++
		return nil
	case kindDouble, kindHeredocExpand:
		return sc.stepDouble(k)
	case kindParam:
		return sc.stepParam()
	case kindArith, kindArithSq, kindArithParen, kindArithIndex, kindLet:
		return sc.stepArith(k)
	case kindCasePat, kindPatGroup:
		return sc.stepPattern(k)
	}
	return sc.stepCommand(k)
}

// heredocLine handles a heredoc body line starting at sc.i. It reports whether
// it consumed the line (a delimiter line, or any line of a literal body).
func (sc *nestScanner) heredocLine(k nestKind) bool {
	eol := strings.IndexByte(sc.s[sc.i:], '\n')
	if eol < 0 {
		eol = len(sc.s)
	} else {
		eol += sc.i
	}
	info := sc.heredocs[len(sc.heredocs)-1]
	line := sc.s[sc.i:eol]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1] // the parser closes "E\r\n" in CRLF scripts
	}
	if info.stripTabs {
		line = strings.TrimLeft(line, "\t")
	}
	consume := func() {
		sc.i = eol
		if sc.i < len(sc.s) {
			sc.i++ // the newline
		}
		sc.lineStart = true
	}
	if line == info.delim {
		sc.pop()
		consume()
		sc.startNextHeredoc()
		return true
	}
	if k == kindHeredocLiteral {
		consume()
		return true
	}
	return false
}

// dollar handles a '$' at sc.i in any context where expansions are active.
// ansi reports whether $'…' is an ANSI-C string there (it is not inside double
// quotes or heredoc bodies).
func (sc *nestScanner) dollar(ansi bool) error {
	switch sc.peek(1) {
	case '(':
		if sc.peek(2) == '(' {
			sc.i += 3
			return sc.push(kindArith)
		}
		sc.i += 2
		sc.startCmd()
		return sc.push(kindParen)
	case '{':
		sc.i += 2
		return sc.push(kindParam)
	case '[':
		sc.i += 2
		return sc.push(kindArithSq)
	case '\'':
		if ansi {
			sc.i += 2
			return sc.push(kindAnsi)
		}
	}
	sc.i++
	return nil
}

func (sc *nestScanner) backtick() error {
	sc.i++
	sc.startCmd()
	return sc.push(kindBacktick)
}

// stepDouble scans a double-quoted string or an unquoted heredoc body: only
// substitutions open nested contexts.
func (sc *nestScanner) stepDouble(k nestKind) error {
	switch sc.s[sc.i] {
	case '\\':
		sc.i++ // escape next byte (a backslash-newline joins lines)
	case '"':
		if k == kindDouble {
			sc.pop()
		}
	case '`':
		return sc.backtick()
	case '$':
		return sc.dollar(false)
	case '\n':
		sc.lineStart = true
	}
	sc.i++
	return nil
}

// stepParam scans the word inside ${…}: everything is literal (including
// "(", "{", "<<" and keywords) except quotes, substitutions, and the closing
// brace. The parser does not nest bare braces here: ${x:-{}} ends at the first
// "}".
func (sc *nestScanner) stepParam() error {
	switch sc.s[sc.i] {
	case '}':
		sc.pop()
	case '\\':
		sc.i++
	case '\'':
		sc.i++
		return sc.push(kindSingle)
	case '"':
		sc.i++
		return sc.push(kindDouble)
	case '`':
		return sc.backtick()
	case '$':
		return sc.dollar(true)
	}
	sc.i++
	return nil
}

// stepArith scans arithmetic: "<<" is a shift, words are variable names, and
// only parentheses, brackets, quotes and substitutions change context. A "let"
// argument list additionally ends at a command terminator, or at an operator
// or redirect that follows a blank (the parser reads "let x && y" as a list
// but "let x&&y" as one expression).
func (sc *nestScanner) stepArith(k nestKind) error {
	c := sc.s[sc.i]
	if k == kindLet {
		switch {
		case c == ';' || c == '\n' || c == ')' || c == '`',
			sc.prevSpace && (c == '&' || c == '|' || c == '<' || c == '>'):
			sc.pop() // reprocess c in the enclosing context
			sc.cmdPos = false
			return nil
		case c == ' ' || c == '\t' || c == '\r':
			sc.prevSpace = true
			sc.i++
			return nil
		}
	}
	sc.prevSpace = false
	switch c {
	case '(':
		sc.i++
		return sc.push(kindArithParen)
	case ')':
		switch {
		case k == kindArithParen:
			sc.pop()
		case k == kindArith && sc.peek(1) == ')':
			sc.pop()
			sc.i++
			sc.cmdPos = false
		case k == kindArith:
			sc.pop() // malformed; the parser stops here
		}
	case '[':
		sc.i++
		return sc.push(kindArithIndex)
	case ']':
		if k == kindArithIndex || k == kindArithSq {
			sc.pop()
		}
	case '\\':
		sc.i++
	case '\'':
		sc.i++
		return sc.push(kindSingle)
	case '"':
		sc.i++
		return sc.push(kindDouble)
	case '`':
		return sc.backtick()
	case '$':
		return sc.dollar(true)
	}
	sc.i++
	return nil
}

// stepPattern scans a case pattern list ("a|b)") or an extglob group inside
// one. Patterns are words, not commands: reserved words are not recognised,
// except a leading "esac" that closes the case statement. The ")" that ends
// the list puts the scanner in command position for the case body.
func (sc *nestScanner) stepPattern(k nestKind) error {
	c := sc.s[sc.i]
	switch {
	case c == ' ' || c == '\t' || c == '\r':
		sc.i++
		return nil
	case c == '\n':
		sc.i++
		sc.lineStart = true
		sc.startNextHeredoc()
		return nil
	case c == '#' && cmdPosOrSep(sc.s, sc.i):
		sc.skipComment()
		return nil
	case isASCIILetter(c):
		j := sc.i + 1
		for j < len(sc.s) && isASCIILetter(sc.s[j]) {
			j++
		}
		if k == kindCasePat && sc.patStart && sc.s[sc.i:j] == "esac" && (j >= len(sc.s) || isWordSep(sc.s[j])) {
			sc.pop() // pattern list
			if sc.top() == kindCase {
				sc.pop()
			}
			sc.startCmd()
			sc.cmdPos = false
		}
		sc.patStart = false
		sc.i = j
		return nil
	}
	start := sc.patStart
	sc.patStart = false
	switch c {
	case '(':
		sc.i++
		if k == kindCasePat && start {
			return nil // optional leading "(" of a pattern list
		}
		return sc.push(kindPatGroup)
	case ')':
		sc.i++
		sc.pop()
		if k == kindCasePat {
			sc.startCmd() // case body
		}
		return nil
	case '\\':
		sc.i += 2
		return nil
	case '\'':
		sc.i++
		return sc.push(kindSingle)
	case '"':
		sc.i++
		return sc.push(kindDouble)
	case '`':
		return sc.backtick()
	case '$':
		return sc.dollar(true)
	}
	sc.i++
	return nil
}

func (sc *nestScanner) skipComment() {
	for sc.i < len(sc.s) && sc.s[sc.i] != '\n' {
		sc.i++
	}
}

// stepCommand scans command context: the top level, or a frame whose body is
// a command list (subshell, command substitution, brace group, backquotes,
// compound command, case body), or a case statement's head.
func (sc *nestScanner) stepCommand(k nestKind) error {
	c := sc.s[sc.i]
	head := k == kindCaseHead0 || k == kindCaseHead1
	subjectSeen := k == kindCaseHead1 // "in" is only the keyword after the subject word
	if head {
		sc.cmdPos = false // the case subject is a word, not a command
		if k == kindCaseHead0 && !isBlank(c) && c != '\n' {
			sc.replaceTop(kindCaseHead1)
			k = kindCaseHead1
		}
	}

	switch c {
	case ' ', '\t', '\r':
		sc.i++
		sc.wordStart = true
		return nil
	case '\n':
		sc.i++
		sc.startCmd()
		sc.lineStart = true
		sc.startNextHeredoc()
		return nil
	case ';':
		if k == kindCase && (sc.peek(1) == ';' || sc.peek(1) == '&') {
			// ";;", ";;&" or ";&" ends a case item; a pattern list follows.
			sc.i += 2
			if sc.peek(0) == '&' {
				sc.i++
			}
			sc.patStart = true
			return sc.push(kindCasePat)
		}
		sc.i++
		sc.startCmd()
		return nil
	case '&':
		if sc.peek(1) == '>' { // &> / &>> redirect
			sc.i += 2
			if sc.peek(0) == '>' {
				sc.i++
			}
			sc.redirTarget = true
			sc.wordStart = true
			sc.cmdPos = false
			return nil
		}
		sc.i++
		sc.startCmd()
		return nil
	case '|':
		sc.i++
		sc.startCmd()
		return nil
	case '<', '>':
		if sc.peek(1) == '(' { // process substitution
			sc.i += 2
			sc.startCmd()
			return sc.push(kindParen)
		}
		if c == '<' {
			sc.i = scanRedirOrHeredoc(sc.s, sc.i, &sc.pending)
		} else {
			sc.i++
		}
		if sc.i < len(sc.s) && (sc.s[sc.i] == '>' || sc.s[sc.i] == '&' || sc.s[sc.i] == '|') {
			sc.i++ // >>, >&, <&, >|
		}
		sc.redirTarget = true
		sc.wordStart = true
		sc.cmdPos = false
		return nil
	case '(':
		sc.wordStart = true
		// An empty "()" (optionally spaced) is a function-definition
		// parameter list — the body that follows is a command, so stay in
		// command position so that its "{ … ; }" group is counted.
		j := sc.i + 1
		for j < len(sc.s) && isBlank(sc.s[j]) {
			j++
		}
		if j < len(sc.s) && sc.s[j] == ')' {
			sc.i = j + 1
			sc.startCmd()
			return nil
		}
		if sc.peek(1) == '(' { // the parser always reads "((" as arithmetic
			sc.i += 2
			sc.cmdPos = false
			return sc.push(kindArith)
		}
		sc.i++
		sc.startCmd()
		return sc.push(kindParen)
	case ')':
		if k == kindParen {
			sc.pop()
		}
		sc.i++
		sc.wordStart = true
		sc.cmdPos = false
		return nil
	}

	if sc.wordStart {
		sc.wordStart = false
		if c == '!' && sc.cmdPos && (sc.i+1 >= len(sc.s) || isWordSep(sc.s[sc.i+1])) {
			sc.i++ // pipeline negation: a command still follows
			sc.wordStart = true
			return nil
		}
		if !head {
			if handled, err := sc.beginWord(); handled || err != nil {
				return err
			}
		}
	}

	switch {
	case c == '\\':
		if sc.peek(1) == '\n' {
			sc.i += 2 // line continuation: does not end the word or change position
			return nil
		}
		sc.i += 2
		sc.cmdPos = false
		return nil
	case c == '\'':
		sc.i++
		sc.cmdPos = false
		return sc.push(kindSingle)
	case c == '"':
		sc.i++
		sc.cmdPos = false
		return sc.push(kindDouble)
	case c == '`':
		if k == kindBacktick {
			sc.pop()
			sc.i++
			sc.cmdPos = false
			return nil
		}
		return sc.backtick()
	case c == '#' && cmdPosOrSep(sc.s, sc.i):
		sc.skipComment()
		return nil
	case c == '$':
		sc.cmdPos = false
		return sc.dollar(true)
	case c == '{':
		sc.i++
		if sc.cmdPos && (sc.i >= len(sc.s) || isWordSep(sc.s[sc.i])) {
			sc.startCmd()
			return sc.push(kindBrace)
		}
		sc.cmdPos = false
		return nil
	case c == '}':
		sc.i++
		if sc.cmdPos && k == kindBrace && (sc.i >= len(sc.s) || isWordSep(sc.s[sc.i])) {
			sc.pop()
		}
		sc.cmdPos = false
		return nil
	case isASCIILetter(c):
		j := sc.i + 1
		for j < len(sc.s) && isASCIILetter(sc.s[j]) {
			j++
		}
		word := sc.s[sc.i:j]
		standalone := j >= len(sc.s) || isWordSep(sc.s[j])
		sc.i = j
		if head {
			if subjectSeen && word == "in" && standalone {
				sc.replaceTop(kindCasePat)
				sc.patStart = true
			}
			return nil
		}
		if sc.cmdPos && standalone {
			return sc.keyword(word)
		}
		sc.cmdPos = false
		return nil
	}
	sc.i++
	sc.cmdPos = false
	return nil
}

// beginWord classifies the word starting at sc.i in command context. Words
// that the parser lexes as assignments with an a[…] index — the prefix
// assignments of a simple command, and every argument of a declaration
// builtin — have that index parsed as arithmetic, so it is scanned as such
// ("<<" there is a shift, not a heredoc). It reports whether it consumed
// input.
func (sc *nestScanner) beginWord() (bool, error) {
	if sc.redirTarget {
		sc.redirTarget = false
		return false, nil
	}
	afterTime := sc.afterTime
	sc.afterTime = false
	j := sc.i
	for j < len(sc.s) && (isASCIILetter(sc.s[j]) || sc.s[j] == '_' || (j > sc.i && isASCIIDigit(sc.s[j]))) {
		j++
	}
	name := sc.s[sc.i:j]
	next := byte(0)
	if j < len(sc.s) {
		next = sc.s[j]
	}
	switch {
	case name != "" && next == '[' && (sc.argWords == 0 || sc.inDecl):
		sc.i = j + 1
		sc.cmdPos = false
		return true, sc.push(kindArithIndex)
	case name != "" && (next == '=' || (next == '+' && j+1 < len(sc.s) && sc.s[j+1] == '=')):
		return false, nil // assignment
	case name == "" && isASCIIDigit(sc.s[sc.i]):
		d := sc.i
		for d < len(sc.s) && isASCIIDigit(sc.s[d]) {
			d++
		}
		if d < len(sc.s) && (sc.s[d] == '<' || sc.s[d] == '>') {
			return false, nil // file-descriptor number of a redirect
		}
	case afterTime && sc.cmdPos && strings.HasPrefix(sc.s[sc.i:], "-p") &&
		(sc.i+2 >= len(sc.s) || isWordSep(sc.s[sc.i+2])):
		sc.i += 2 // time -p: a command still follows
		sc.wordStart = true
		return true, nil
	}
	sc.argWords++
	if sc.argWords == 1 && sc.cmdPos {
		switch name {
		case "declare", "local", "export", "readonly", "typeset", "nameref":
			if next == 0 || isWordSep(next) {
				sc.inDecl = true
			}
		}
	}
	return false, nil
}

// keyword updates the context stack for a standalone word seen in command
// position. Compound-command openers push a counted frame; their closers pop
// the matching frame; continuation words (then/else/do) restore command
// position only inside the compound they belong to. Anything else is an
// ordinary command name.
func (sc *nestScanner) keyword(word string) error {
	top := sc.top()
	switch word {
	case "if":
		sc.startCmd()
		return sc.push(kindIf)
	case "elif":
		sc.startCmd()
		return sc.push(kindElif)
	case "while", "until":
		sc.startCmd()
		return sc.push(kindLoop)
	case "for", "select":
		sc.cmdPos = false // followed by a loop variable name, not a command
		return sc.push(kindLoop)
	case "case":
		sc.cmdPos = false // followed by the word being matched
		if err := sc.push(kindCase); err != nil {
			return err
		}
		return sc.push(kindCaseHead0)
	case "fi":
		// Close an if ladder: pop any elif frames, then the matching if.
		for sc.top() == kindElif {
			sc.pop()
		}
		if sc.top() == kindIf {
			sc.pop()
		}
		sc.cmdPos = false
	case "done":
		if top == kindLoop {
			sc.pop()
		}
		sc.cmdPos = false
	case "esac":
		if top == kindCase {
			sc.pop()
		}
		sc.cmdPos = false
	case "then", "else":
		if top == kindIf || top == kindElif {
			sc.startCmd()
		} else {
			sc.cmdPos = false
		}
	case "do":
		if top == kindLoop {
			sc.startCmd()
		} else {
			sc.cmdPos = false
		}
	case "time":
		sc.startCmd()
		sc.afterTime = true
	case "function", "coproc":
		// function NAME [()] BODY / coproc [NAME] COMMAND: skip the name
		// so the body is scanned in command position. (For coproc the
		// first word may instead be the command itself; skipping it only
		// ever over-counts a keyword-looking argument.)
		j := sc.i
		for j < len(sc.s) && isBlank(sc.s[j]) {
			j++
		}
		if word == "coproc" && !coprocNamed(sc.s, j) {
			sc.startCmd()
			return nil
		}
		for j < len(sc.s) && !isWordSep(sc.s[j]) {
			j++
		}
		sc.i = j
		sc.startCmd()
	case "let":
		sc.cmdPos = false
		sc.prevSpace = false
		return sc.push(kindLet)
	default:
		sc.cmdPos = false // ordinary command name
	}
	return nil
}

// coprocNamed reports whether the coproc at script[j:] is "coproc NAME
// COMPOUND", i.e. a plain word followed by a compound command opener.
func coprocNamed(script string, j int) bool {
	k := j
	for k < len(script) && !isWordSep(script[k]) {
		k++
	}
	if k == j {
		return false
	}
	switch script[j:k] {
	case "if", "while", "until", "for", "select", "case", "{":
		return false
	}
	for k < len(script) && isBlank(script[k]) {
		k++
	}
	if k >= len(script) {
		return false
	}
	if script[k] == '(' || script[k] == '{' {
		return true
	}
	e := k
	for e < len(script) && isASCIILetter(script[e]) {
		e++
	}
	switch script[k:e] {
	case "if", "while", "until", "for", "select", "case":
		return true
	}
	return false
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\r' }

// isWordSep reports whether c terminates a shell word, so that a letter run
// bounded by separators can be recognised as a standalone reserved word.
func isWordSep(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ';', '&', '|', '(', ')', '<', '>', '`':
		return true
	}
	return false
}

// cmdPosOrSep reports whether the '#' at index i begins a comment: true at the
// start of input or when preceded by an unquoted separator. Being conservative
// here only ever skips genuine comments; a '#' glued to a word (a#b, ${#x}) is
// not treated as a comment.
func cmdPosOrSep(script string, i int) bool {
	if i == 0 {
		return true
	}
	switch script[i-1] {
	case ' ', '\t', '\n', '\r', ';', '&', '|', '(':
		return true
	}
	return false
}

// scanRedirOrHeredoc handles a '<' at index i. It recognises "<<-"/"<<" heredoc
// operators (recording the body delimiter in pending so the following lines are
// treated as a heredoc body) and the "<<<" here-string and plain "<" redirects
// (which have no body). It returns the index just past what it consumed.
func scanRedirOrHeredoc(script string, i int, pending *[]heredocInfo) int {
	n := len(script)
	if i+1 >= n || script[i+1] != '<' {
		return i + 1 // plain "<" redirect
	}
	if i+2 < n && script[i+2] == '<' {
		return i + 3 // "<<<" here-string: no heredoc body
	}
	// Heredoc: "<<" or "<<-".
	j := i + 2
	stripTabs := false
	if j < n && script[j] == '-' {
		stripTabs = true
		j++
	}
	for j < n && (script[j] == ' ' || script[j] == '\t') {
		j++
	}
	delim, expand, end := readHeredocDelim(script, j)
	if delim != "" {
		*pending = append(*pending, heredocInfo{delim: delim, stripTabs: stripTabs, expand: expand})
	}
	return end
}

// readHeredocDelim reads a heredoc delimiter word starting at i, following the
// quoting that decides whether the body expands. A delimiter containing any
// quote or backslash makes the body literal (expand == false); a bare delimiter
// leaves $(...)/${...}/`...` active in the body. It returns the unquoted
// delimiter text, whether the body expands, and the index past the word.
func readHeredocDelim(script string, i int) (string, bool, int) {
	n := len(script)
	var b strings.Builder
	expand := true
	for i < n {
		c := script[i]
		switch c {
		case ' ', '\t', '\n', '\r', ';', '&', '|', '<', '>', '(', ')':
			return b.String(), expand, i
		case '\'':
			expand = false
			i++
			for i < n && script[i] != '\'' {
				b.WriteByte(script[i])
				i++
			}
			if i < n {
				i++ // closing quote
			}
		case '"':
			expand = false
			i++
			for i < n && script[i] != '"' {
				b.WriteByte(script[i])
				i++
			}
			if i < n {
				i++
			}
		case '\\':
			expand = false
			i++
			if i < n {
				b.WriteByte(script[i])
				i++
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), expand, i
}

// checkNestingDepth bounds the structural depth of a parsed AST. It is called
// from [validateNode] — and therefore on every [Runner.Run] and [Runner.Check]
// path — before the recursive [syntax.Walk] validation pass and before any
// recursive execution, so that none of those recursions can exceed
// [MaxNestingDepth] stack frames.
//
// It is itself iterative: it never recurses deeper than one level (to read a
// node's direct children), so it cannot overflow the stack regardless of how
// deep the AST is. Constructs that the recursive-descent parser accepts
// without recursing per level — notably long pipelines (a|b|c…) and logical
// lists (a&&b&&c…), which build a left-nested BinaryCmd tree whose depth the
// parser does not pay for in stack but whose later walks and execution do —
// are bounded here even though the lexical pre-scan does not see them.
func checkNestingDepth(root syntax.Node) error {
	if root == nil {
		return nil
	}
	type item struct {
		node  syntax.Node
		depth int
	}
	stack := []item{{root, 0}}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if it.depth > MaxNestingDepth {
			return errNestingTooDeep()
		}
		for _, child := range directChildren(it.node) {
			stack = append(stack, item{child, it.depth + 1})
		}
	}
	return nil
}

// directChildren returns the immediate child nodes of parent without
// descending further. It uses [syntax.Walk], whose visitor is invoked for the
// node itself first and then for each child; returning true only for parent
// and false for every child stops the walk at depth one, so this call recurses
// a single frame regardless of the overall tree depth.
func directChildren(parent syntax.Node) []syntax.Node {
	var children []syntax.Node
	syntax.Walk(parent, func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		if n == parent {
			return true // descend into parent's direct children
		}
		children = append(children, n)
		return false // but no deeper
	})
	return children
}
