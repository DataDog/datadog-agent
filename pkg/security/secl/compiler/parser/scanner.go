// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parser holds a hand written parser of the SECL language.
package parser

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// scanner splits an expression into tokens, one at a time, on demand of the
// parser.
//
// It never fails: a character it can't make sense of becomes an Illegal token
// and an error, and scanning resumes right after it.
type scanner struct {
	src string
	// pos is the position of the next character to read.
	pos ast.Position

	errs *ErrorList
}

func newScanner(src string, errs *ErrorList) *scanner {
	return &scanner{
		src:  src,
		pos:  ast.Position{Line: 1, Column: 1},
		errs: errs,
	}
}

// next returns the next token, skipping whitespaces and comments.
func (s *scanner) next() Token {
	s.skipWhitespacesAndComments()

	start := s.pos
	if s.eof() {
		return Token{Kind: EOF, Pos: start}
	}

	c := s.peek(0)
	switch {
	case c == '"':
		return s.scanQuoted(start, String)
	case c == 'r' && s.peek(1) == '"':
		s.advance(1)
		return s.scanQuoted(start, Regexp)
	case c == '~' && s.peek(1) == '"':
		s.advance(1)
		return s.scanQuoted(start, Pattern)
	case s.atIPv6():
		return s.scanIP(start, s.hexRunLen())
	case isIdentStart(c):
		return s.scanIdent(start)
	case isDigit(c):
		return s.scanNumber(start)
	case c == '$' && s.peek(1) == '{':
		return s.scanDelimited(start, Variable, isVariablePart)
	case c == '%' && s.peek(1) == '{':
		return s.scanDelimited(start, FieldRef, isFieldRefPart)
	}

	return s.scanOperator(start)
}

// scanOperator scans operators and punctuation. Two character operators are
// single tokens: `& &` is not `&&`.
func (s *scanner) scanOperator(start ast.Position) Token {
	c, c1 := s.peek(0), s.peek(1)

	two := func(kind Kind) Token {
		s.advance(2)
		return s.token(kind, start)
	}
	one := func(kind Kind) Token {
		s.advance(1)
		return s.token(kind, start)
	}

	switch c {
	case '&':
		if c1 == '&' {
			return two(AndAnd)
		}
		return one(Amp)
	case '|':
		if c1 == '|' {
			return two(OrOr)
		}
		return one(Pipe)
	case '=':
		switch c1 {
		case '=':
			return two(Eq)
		case '~':
			return two(Match)
		}
		s.advance(1)
		return s.illegal(start, "unexpected `=`", "did you mean `==`?")
	case '!':
		switch c1 {
		case '=':
			return two(NotEq)
		case '~':
			return two(NotMatch)
		}
		return one(Bang)
	case '<':
		if c1 == '=' {
			return two(Le)
		}
		return one(Lt)
	case '>':
		if c1 == '=' {
			return two(Ge)
		}
		return one(Gt)
	case '^':
		return one(Caret)
	case '+':
		return one(Plus)
	case '-':
		// a sign is always an operator, `-1` is folded into a negative
		// literal by the parser
		return one(Minus)
	case '(':
		return one(LParen)
	case ')':
		return one(RParen)
	case '[':
		return one(LBracket)
	case ']':
		return one(RBracket)
	case ',':
		return one(Comma)
	}

	_, size := utf8.DecodeRuneInString(s.src[s.pos.Offset:])
	s.advance(size)
	return s.illegal(start, fmt.Sprintf("unexpected character `%s`", s.src[start.Offset:s.pos.Offset]), "")
}

// scanQuoted scans a double quoted literal, the opening quote being the next
// character. Escape sequences are kept as written: the parser unquotes the
// literal.
func (s *scanner) scanQuoted(start ast.Position, kind Kind) Token {
	s.advance(1) // opening quote

	for !s.eof() {
		switch s.peek(0) {
		case '\\':
			s.advance(2)
		case '"':
			s.advance(1)
			return s.token(kind, start)
		default:
			s.advance(1)
		}
	}

	// the error points to the opening quote, not to the end of the
	// expression where the scanner noticed it
	s.errs.add(&Error{Pos: start, End: start.Offset + 1, Msg: fmt.Sprintf("unterminated %s", kind)})
	return s.token(Illegal, start)
}

// scanIdent scans an identifier, or a keyword.
//
// A `[` is only part of an identifier when it opens a subscript,
// `ancestors[A]`, so that the identifiers of an array literal don't need
// to be separated from the closing bracket: `[O_RDONLY, O_WRONLY]`.
func (s *scanner) scanIdent(start ast.Position) Token {
	for !s.eof() {
		c := s.peek(0)
		switch {
		case isIdentPart(c) || c == '.':
			s.advance(1)
		case c == '[':
			n := s.subscriptLen()
			if n == 0 {
				return s.identOrKeyword(start)
			}
			s.advance(n)
		default:
			return s.identOrKeyword(start)
		}
	}
	return s.identOrKeyword(start)
}

// subscriptLen returns the length of the `[A]` subscript starting at the next
// character, 0 if there is none.
func (s *scanner) subscriptLen() int {
	n := 1
	for isIdentPart(s.peek(n)) {
		n++
	}
	if n == 1 || s.peek(n) != ']' {
		return 0
	}
	return n + 1
}

func (s *scanner) identOrKeyword(start ast.Position) Token {
	tok := s.token(Ident, start)
	if kind, ok := keywords[tok.Text]; ok {
		tok.Kind = kind
	}
	return tok
}

// scanNumber scans the tokens starting with a digit: an integer, a duration,
// an IPv4 or a CIDR.
func (s *scanner) scanNumber(start ast.Position) Token {
	n := 0
	for isDigit(s.peek(n)) {
		n++
	}

	switch c := s.peek(n); {
	case c == '.' && isDigit(s.peek(n+1)):
		return s.scanIP(start, s.hexRunLen())
	case isIdentStart(c):
		return s.scanDuration(start)
	}

	s.advance(n)
	return s.token(Int, start)
}

// scanDuration scans a duration: an integer followed by a unit, `10s`, or a
// sequence of them, `1h30m`.
func (s *scanner) scanDuration(start ast.Position) Token {
	for isIdentPart(s.peek(0)) {
		s.advance(1)
	}

	tok := s.token(Duration, start)
	if _, err := time.ParseDuration(tok.Text); err != nil {
		return s.illegal(start, fmt.Sprintf("invalid duration `%s`", tok.Text), "the units are `ns`, `us`, `ms`, `s`, `m` and `h`")
	}
	return tok
}

// atIPv6 returns whether an IPv6 starts at the next character. A colon
// appears nowhere else in the language: any run of hexadecimal digits, dots
// and colons holding one is an IPv6.
func (s *scanner) atIPv6() bool {
	n := s.hexRunLen()
	return strings.IndexByte(s.src[s.pos.Offset:s.pos.Offset+n], ':') >= 0
}

// hexRunLen returns the length of the run of hexadecimal digits, dots and
// colons starting at the next character.
func (s *scanner) hexRunLen() int {
	n := 0
	for c := s.peek(n); isHexDigit(c) || c == '.' || c == ':'; c = s.peek(n) {
		n++
	}
	return n
}

// scanIP scans an IP of n characters, and the prefix length making it a CIDR,
// if any: `/24`.
//
// Only the shape of the IP is checked, its validity is checked by the
// compilation.
func (s *scanner) scanIP(start ast.Position, n int) Token {
	s.advance(n)

	if s.peek(0) != '/' || !isDigit(s.peek(1)) {
		return s.token(IP, start)
	}

	s.advance(1)
	for isDigit(s.peek(0)) {
		s.advance(1)
	}
	return s.token(CIDR, start)
}

// scanDelimited scans a `${name}` or a `%{name}`, the characters of name
// being the ones accepted by isPart.
func (s *scanner) scanDelimited(start ast.Position, kind Kind, isPart func(byte) bool) Token {
	s.advance(2)

	if !isIdentStart(s.peek(0)) {
		return s.unterminatedDelimited(start, kind)
	}
	for isPart(s.peek(0)) {
		s.advance(1)
	}

	if s.peek(0) != '}' {
		return s.unterminatedDelimited(start, kind)
	}
	s.advance(1)

	return s.token(kind, start)
}

// unterminatedDelimited reports a `${` or a `%{` that isn't followed by a name
// and a closing brace.
func (s *scanner) unterminatedDelimited(start ast.Position, kind Kind) Token {
	s.errs.add(&Error{
		Pos:  start,
		End:  s.pos.Offset,
		Msg:  fmt.Sprintf("invalid %s `%s`", kind, s.src[start.Offset:s.pos.Offset]),
		Hint: fmt.Sprintf("expected `%s{name}`", s.src[start.Offset:start.Offset+1]),
	})
	return s.token(Illegal, start)
}

func (s *scanner) skipWhitespacesAndComments() {
	for !s.eof() {
		switch c := s.peek(0); {
		case c == ' ' || c == '\t' || c == '\n':
			s.advance(1)
		case c == '#' || (c == '/' && s.peek(1) == '/'):
			for !s.eof() && s.peek(0) != '\n' {
				s.advance(1)
			}
		default:
			return
		}
	}
}

// illegal reports an error over the characters scanned since start, and
// returns them as an Illegal token.
func (s *scanner) illegal(start ast.Position, msg, hint string) Token {
	s.errs.add(&Error{Pos: start, End: s.pos.Offset, Msg: msg, Hint: hint})
	return s.token(Illegal, start)
}

// token returns a token holding the characters scanned since start.
func (s *scanner) token(kind Kind, start ast.Position) Token {
	return Token{Kind: kind, Text: s.src[start.Offset:s.pos.Offset], Pos: start}
}

func (s *scanner) eof() bool {
	return s.pos.Offset >= len(s.src)
}

// peek returns the byte n characters ahead, 0 past the end of the source.
func (s *scanner) peek(n int) byte {
	if s.pos.Offset+n >= len(s.src) {
		return 0
	}
	return s.src[s.pos.Offset+n]
}

// advance moves n bytes forward, clamped to the end of the source.
//
// Columns count runes, not bytes: only the first byte of a character moves to
// the next column, so that advancing over a multibyte character one byte at a
// time, as scanQuoted does, counts it once.
func (s *scanner) advance(n int) {
	end := min(s.pos.Offset+n, len(s.src))
	for i := s.pos.Offset; i < end; i++ {
		switch c := s.src[i]; {
		case c == '\n':
			s.pos.Line++
			s.pos.Column = 1
		case utf8.RuneStart(c):
			s.pos.Column++
		}
	}
	s.pos.Offset = end
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isIdentStart(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_'
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func isHexDigit(c byte) bool {
	return isDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func isVariablePart(c byte) bool {
	return isIdentPart(c) || c == '.'
}

func isFieldRefPart(c byte) bool {
	return isIdentPart(c) || c == '.' || c == '[' || c == ']'
}
