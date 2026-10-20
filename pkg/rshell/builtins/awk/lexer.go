// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNewline
	tokIdent
	tokNumber
	tokString
	tokRegex
	tokLBrace
	tokRBrace
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokSemicolon
	tokComma
	tokQuestion
	tokColon
	tokDollar
	tokAssign
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokPercent
	tokCaret
	tokBang
	tokTilde
	tokLT
	tokGT
	tokLE
	tokGE
	tokEQ
	tokNE
	tokAnd
	tokOr
	tokMatch
	tokNotMatch
	tokPlusAssign
	tokMinusAssign
	tokStarAssign
	tokSlashAssign
	tokPercentAssign
	tokCaretAssign
	tokInc
	tokDec
	tokAppend
)

type token struct {
	kind tokenKind
	lit  string
	pos  int
}

type lexer struct {
	ctx     context.Context
	err     error
	src     string
	pos     int
	last    tokenKind
	lastLit string
}

func lex(src string) ([]token, error) {
	return lexContext(context.Background(), src)
}

func lexContext(ctx context.Context, src string) ([]token, error) {
	l := &lexer{ctx: ctx, src: src, last: tokEOF}
	var toks []token
	for {
		if l.checkContext() {
			return nil, l.err
		}
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.kind == tokEOF {
			return toks, nil
		}
		if tok.kind != tokEOF {
			l.last = tok.kind
			l.lastLit = tok.lit
		}
	}
}

func (l *lexer) next() (token, error) {
	for {
		l.skipLineContinuations()
		if l.checkContext() {
			return token{}, l.err
		}
		if l.pos >= len(l.src) {
			break
		}
		ch := l.src[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.pos++
			continue
		}
		if ch == '#' {
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				if l.checkContext() {
					return token{}, l.err
				}
				l.pos++
			}
			continue
		}
		break
	}
	start := l.pos
	if l.pos >= len(l.src) {
		return token{kind: tokEOF, pos: start}, nil
	}
	ch := l.src[l.pos]
	l.pos++
	switch ch {
	case '\n':
		return token{kind: tokNewline, lit: "\n", pos: start}, nil
	case '{':
		return token{kind: tokLBrace, lit: "{", pos: start}, nil
	case '}':
		return token{kind: tokRBrace, lit: "}", pos: start}, nil
	case '(':
		return token{kind: tokLParen, lit: "(", pos: start}, nil
	case ')':
		return token{kind: tokRParen, lit: ")", pos: start}, nil
	case '[':
		return token{kind: tokLBracket, lit: "[", pos: start}, nil
	case ']':
		return token{kind: tokRBracket, lit: "]", pos: start}, nil
	case ';':
		return token{kind: tokSemicolon, lit: ";", pos: start}, nil
	case ',':
		return token{kind: tokComma, lit: ",", pos: start}, nil
	case '?':
		return token{kind: tokQuestion, lit: "?", pos: start}, nil
	case ':':
		return token{kind: tokColon, lit: ":", pos: start}, nil
	case '$':
		return token{kind: tokDollar, lit: "$", pos: start}, nil
	case '~':
		return token{kind: tokMatch, lit: "~", pos: start}, nil
	case '+':
		if l.match('+') {
			return token{kind: tokInc, lit: "++", pos: start}, nil
		}
		if l.match('=') {
			return token{kind: tokPlusAssign, lit: "+=", pos: start}, nil
		}
		return token{kind: tokPlus, lit: "+", pos: start}, nil
	case '-':
		if l.match('-') {
			return token{kind: tokDec, lit: "--", pos: start}, nil
		}
		if l.match('=') {
			return token{kind: tokMinusAssign, lit: "-=", pos: start}, nil
		}
		return token{kind: tokMinus, lit: "-", pos: start}, nil
	case '*':
		if l.match('*') {
			if l.match('=') {
				return token{kind: tokCaretAssign, lit: "**=", pos: start}, nil
			}
			return token{kind: tokCaret, lit: "**", pos: start}, nil
		}
		if l.match('=') {
			return token{kind: tokStarAssign, lit: "*=", pos: start}, nil
		}
		return token{kind: tokStar, lit: "*", pos: start}, nil
	case '/':
		if canStartRegex(l.last, l.lastLit) {
			return l.scanRegex(start)
		}
		if l.match('=') {
			return token{kind: tokSlashAssign, lit: "/=", pos: start}, nil
		}
		return token{kind: tokSlash, lit: "/", pos: start}, nil
	case '%':
		if l.match('=') {
			return token{kind: tokPercentAssign, lit: "%=", pos: start}, nil
		}
		return token{kind: tokPercent, lit: "%", pos: start}, nil
	case '^':
		if l.match('=') {
			return token{kind: tokCaretAssign, lit: "^=", pos: start}, nil
		}
		return token{kind: tokCaret, lit: "^", pos: start}, nil
	case '!':
		if l.match('=') {
			return token{kind: tokNE, lit: "!=", pos: start}, nil
		}
		if l.match('~') {
			return token{kind: tokNotMatch, lit: "!~", pos: start}, nil
		}
		return token{kind: tokBang, lit: "!", pos: start}, nil
	case '<':
		if l.match('=') {
			return token{kind: tokLE, lit: "<=", pos: start}, nil
		}
		return token{kind: tokLT, lit: "<", pos: start}, nil
	case '>':
		if l.match('=') {
			return token{kind: tokGE, lit: ">=", pos: start}, nil
		}
		if l.match('>') {
			return token{kind: tokAppend, lit: ">>", pos: start}, nil
		}
		return token{kind: tokGT, lit: ">", pos: start}, nil
	case '=':
		if l.match('=') {
			return token{kind: tokEQ, lit: "==", pos: start}, nil
		}
		return token{kind: tokAssign, lit: "=", pos: start}, nil
	case '&':
		if l.match('&') {
			return token{kind: tokAnd, lit: "&&", pos: start}, nil
		}
	case '|':
		if l.match('|') {
			return token{kind: tokOr, lit: "||", pos: start}, nil
		}
		return token{}, fmt.Errorf("command pipes are not supported")
	case '"':
		return l.scanString(start)
	}
	next, hasNext := l.peek()
	if isDigit(rune(ch)) || (ch == '.' && hasNext && isDigit(rune(next))) {
		return l.scanNumber(start)
	}
	if isIdentStart(rune(ch)) {
		return l.scanIdent(start)
	}
	r, _ := utf8.DecodeRuneInString(l.src[start:])
	return token{}, fmt.Errorf("unexpected character %q", r)
}

func (l *lexer) match(ch byte) bool {
	next, ok := l.peek()
	if !ok || next != ch {
		return false
	}
	l.pos++
	return true
}

func (l *lexer) peek() (byte, bool) {
	l.skipLineContinuations()
	if l.checkContext() {
		return 0, false
	}
	if l.pos >= len(l.src) {
		return 0, false
	}
	return l.src[l.pos], true
}

func (l *lexer) skipLineContinuations() {
	for l.pos+1 < len(l.src) && l.src[l.pos] == '\\' {
		if l.checkContext() {
			return
		}
		switch {
		case l.src[l.pos+1] == '\n':
			l.pos += 2
		case l.pos+2 < len(l.src) && l.src[l.pos+1] == '\r' && l.src[l.pos+2] == '\n':
			l.pos += 3
		default:
			return
		}
	}
}

func (l *lexer) checkContext() bool {
	if l.err != nil || l.ctx == nil {
		return l.err != nil
	}
	l.err = l.ctx.Err()
	return l.err != nil
}

func (l *lexer) tokenLiteral(start int) string {
	lit := strings.ReplaceAll(l.src[start:l.pos], "\\\r\n", "")
	return strings.ReplaceAll(lit, "\\\n", "")
}

func (l *lexer) scanIdent(start int) (token, error) {
	for {
		if l.checkContext() {
			return token{}, l.err
		}
		ch, ok := l.peek()
		if !ok || !isIdentPart(rune(ch)) {
			break
		}
		l.pos++
	}
	lit := l.tokenLiteral(start)
	return token{kind: tokIdent, lit: lit, pos: start}, nil
}

func (l *lexer) scanNumber(start int) (token, error) {
	if l.src[start] == '0' {
		if ch, ok := l.peek(); ok && (ch == 'x' || ch == 'X') {
			return token{}, fmt.Errorf("non-decimal numeric literals are not supported")
		}
	}
	if l.src[start] == '.' {
		for {
			if l.checkContext() {
				return token{}, l.err
			}
			ch, ok := l.peek()
			if !ok || !isDigit(rune(ch)) {
				break
			}
			l.pos++
		}
	} else {
		for {
			if l.checkContext() {
				return token{}, l.err
			}
			ch, ok := l.peek()
			if !ok || !isDigit(rune(ch)) {
				break
			}
			l.pos++
		}
		if ch, ok := l.peek(); ok && ch == '.' {
			l.pos++
			for {
				if l.checkContext() {
					return token{}, l.err
				}
				ch, ok := l.peek()
				if !ok || !isDigit(rune(ch)) {
					break
				}
				l.pos++
			}
		}
	}
	if ch, ok := l.peek(); ok && (ch == 'e' || ch == 'E') {
		save := l.pos
		l.pos++
		if ch, ok := l.peek(); ok && (ch == '+' || ch == '-') {
			l.pos++
		}
		ch, ok := l.peek()
		if !ok || !isDigit(rune(ch)) {
			l.pos = save
		} else {
			for {
				if l.checkContext() {
					return token{}, l.err
				}
				ch, ok := l.peek()
				if !ok || !isDigit(rune(ch)) {
					break
				}
				l.pos++
			}
		}
	}
	return token{kind: tokNumber, lit: l.tokenLiteral(start), pos: start}, nil
}

func (l *lexer) scanString(start int) (token, error) {
	var b strings.Builder
	for {
		if l.checkContext() {
			return token{}, l.err
		}
		l.skipLineContinuations()
		if l.pos >= len(l.src) {
			break
		}
		ch := l.src[l.pos]
		l.pos++
		if ch == '"' {
			return token{kind: tokString, lit: b.String(), pos: start}, nil
		}
		if ch == '\n' {
			return token{}, fmt.Errorf("unterminated string")
		}
		if ch == '\\' {
			next, ok := l.peek()
			if !ok {
				return token{}, fmt.Errorf("unterminated string escape")
			}
			if isOctalDigit(rune(next)) {
				value := 0
				for digits := 0; digits < 3; digits++ {
					digit, ok := l.peek()
					if !ok || !isOctalDigit(rune(digit)) {
						break
					}
					value = value*8 + int(digit-'0')
					l.pos++
				}
				b.WriteByte(byte(value))
				continue
			}
			if value, size, ok := decodeAwkHexEscape(l.src[l.pos:]); ok {
				l.pos += size
				b.WriteByte(value)
				continue
			}
			esc := next
			l.pos++
			if esc < 0x80 {
				b.WriteByte(byte(decodeSimpleEscape(rune(esc))))
			} else {
				b.WriteByte(esc)
			}
			continue
		}
		b.WriteByte(ch)
	}
	return token{}, fmt.Errorf("unterminated string")
}

func (l *lexer) scanRegex(start int) (token, error) {
	var b strings.Builder
	inClass := false
	classStart := false
	classHasMember := false
	var classSubexpression byte
	classSubexpressionEnd := false
	for {
		if l.checkContext() {
			return token{}, l.err
		}
		l.skipLineContinuations()
		if l.pos >= len(l.src) {
			break
		}
		ch := l.src[l.pos]
		l.pos++
		if ch == '/' && !inClass {
			return token{kind: tokRegex, lit: b.String(), pos: start}, nil
		}
		if ch == '\n' {
			return token{}, fmt.Errorf("unterminated regular expression")
		}
		if ch == '\\' {
			next, ok := l.peek()
			if !ok {
				return token{}, fmt.Errorf("unterminated regular expression escape")
			}
			l.pos++
			b.WriteByte('\\')
			b.WriteByte(next)
			if inClass {
				classStart = false
				classHasMember = true
			}
			continue
		}
		if ch == '[' && !inClass {
			inClass = true
			classStart = true
			classHasMember = false
		} else if inClass {
			switch {
			case classSubexpression != 0:
				if ch == ']' && classSubexpressionEnd {
					classSubexpression = 0
					classSubexpressionEnd = false
				} else {
					classSubexpressionEnd = ch == classSubexpression
				}
				classStart = false
				classHasMember = true
			case ch == '[':
				if next, ok := l.peek(); ok && (next == ':' || next == '.' || next == '=') {
					classSubexpression = next
				}
				classStart = false
				classHasMember = true
			case classStart && ch == '^':
				classStart = false
			case ch == ']' && classHasMember:
				inClass = false
			default:
				classStart = false
				classHasMember = true
			}
		}
		b.WriteByte(ch)
	}
	return token{}, fmt.Errorf("unterminated regular expression")
}

func canStartRegex(prev tokenKind, prevLit string) bool {
	if prev == tokIdent {
		switch prevLit {
		case "print", "printf", "return", "exit":
			return true
		}
	}
	switch prev {
	case tokEOF, tokNewline, tokLBrace, tokRBrace, tokLParen, tokLBracket, tokComma, tokSemicolon,
		tokQuestion, tokColon, tokAssign, tokPlus, tokMinus, tokStar, tokSlash, tokPercent, tokCaret, tokBang,
		tokLT, tokGT, tokLE, tokGE, tokEQ, tokNE, tokAnd, tokOr, tokMatch,
		tokNotMatch, tokPlusAssign, tokMinusAssign, tokStarAssign,
		tokSlashAssign, tokPercentAssign, tokCaretAssign:
		return true
	default:
		return false
	}
}

func DecodeAwkEscapes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		raw := s[:size]
		s = s[size:]
		if r != '\\' || len(s) == 0 {
			b.WriteString(raw)
			continue
		}
		if isOctalDigit(rune(s[0])) {
			value := 0
			for digits := 0; digits < 3 && len(s) > 0 && isOctalDigit(rune(s[0])); digits++ {
				value = value*8 + int(s[0]-'0')
				s = s[1:]
			}
			b.WriteByte(byte(value))
			continue
		}
		if value, size, ok := decodeAwkHexEscape(s); ok {
			s = s[size:]
			b.WriteByte(value)
			continue
		}
		esc, escSize := utf8.DecodeRuneInString(s)
		raw = s[:escSize]
		s = s[escSize:]
		if esc == utf8.RuneError && escSize == 1 {
			b.WriteString(raw)
		} else {
			b.WriteRune(decodeSimpleEscape(esc))
		}
	}
	return b.String()
}

func decodeAwkHexEscape(s string) (byte, int, bool) {
	if len(s) < 2 || s[0] != 'x' {
		return 0, 0, false
	}
	value := 0
	digits := 0
	for digits < 2 && digits+1 < len(s) {
		digit, ok := digitValue(s[digits+1])
		if !ok {
			break
		}
		value = value*16 + digit
		digits++
	}
	if digits == 0 {
		return 0, 0, false
	}
	return byte(value), digits + 1, true
}

func isOctalDigit(ch rune) bool {
	return ch >= '0' && ch <= '7'
}

func decodeSimpleEscape(esc rune) rune {
	switch esc {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'a':
		return '\a'
	case 'v':
		return '\v'
	default:
		return esc
	}
}

func isIdentStart(ch rune) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isIdentPart(ch rune) bool {
	return isIdentStart(ch) || isDigit(ch)
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}
