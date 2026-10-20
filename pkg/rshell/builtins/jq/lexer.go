// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jq

import (
	"encoding/json"
	"fmt"
)

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenDot
	tokenIdentifier
	tokenVariable
	tokenNumber
	tokenString
	tokenLeftParen
	tokenRightParen
	tokenLeftBracket
	tokenRightBracket
	tokenLeftBrace
	tokenRightBrace
	tokenColon
	tokenComma
	tokenPipe
	tokenOptional
	tokenAlternative
	tokenPlus
	tokenMinus
	tokenMultiply
	tokenDivide
	tokenModulo
	tokenEqual
	tokenNotEqual
	tokenLess
	tokenLessEqual
	tokenGreater
	tokenGreaterEqual
)

type token struct {
	kind tokenKind
	text string
	pos  int
}

type lexer struct {
	input string
	pos   int
}

func lexFilter(input string) ([]token, error) {
	if len(input) > MaxFilterBytes {
		return nil, fmt.Errorf("filter exceeds the %d-byte limit", MaxFilterBytes)
	}
	l := &lexer{input: input}
	tokens := make([]token, 0, 32)
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.kind == tokenEOF {
			return tokens, nil
		}
	}
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.input) && isFilterSpace(l.input[l.pos]) {
		l.pos++
	}
	if l.pos == len(l.input) {
		return token{kind: tokenEOF, pos: l.pos}, nil
	}

	start := l.pos
	c := l.input[l.pos]
	l.pos++
	if l.pos < len(l.input) && l.input[l.pos] == '=' {
		switch c {
		case '|', '+', '-', '*', '%':
			return token{}, fmt.Errorf("assignments are not supported at byte %d", start)
		}
	}
	switch c {
	case '.':
		if l.pos < len(l.input) && isASCIIDigit(l.input[l.pos]) {
			l.pos = start
			return l.number()
		}
		if l.pos < len(l.input) && l.input[l.pos] == '.' {
			return token{}, fmt.Errorf("recursion is not supported at byte %d", start)
		}
		return token{kind: tokenDot, text: ".", pos: start}, nil
	case '(':
		return token{kind: tokenLeftParen, text: "(", pos: start}, nil
	case ')':
		return token{kind: tokenRightParen, text: ")", pos: start}, nil
	case '[':
		return token{kind: tokenLeftBracket, text: "[", pos: start}, nil
	case ']':
		return token{kind: tokenRightBracket, text: "]", pos: start}, nil
	case '{':
		return token{kind: tokenLeftBrace, text: "{", pos: start}, nil
	case '}':
		return token{kind: tokenRightBrace, text: "}", pos: start}, nil
	case ':':
		return token{kind: tokenColon, text: ":", pos: start}, nil
	case ',':
		return token{kind: tokenComma, text: ",", pos: start}, nil
	case '|':
		return token{kind: tokenPipe, text: "|", pos: start}, nil
	case '?':
		if l.pos+1 < len(l.input) && l.input[l.pos:l.pos+2] == "//" {
			return token{}, fmt.Errorf("?// is not supported at byte %d", start)
		}
		return token{kind: tokenOptional, text: "?", pos: start}, nil
	case '/':
		if l.pos < len(l.input) && l.input[l.pos] == '/' {
			l.pos++
			return token{kind: tokenAlternative, text: "//", pos: start}, nil
		}
		return token{kind: tokenDivide, text: "/", pos: start}, nil
	case '+':
		return token{kind: tokenPlus, text: "+", pos: start}, nil
	case '-':
		return token{kind: tokenMinus, text: "-", pos: start}, nil
	case '*':
		return token{kind: tokenMultiply, text: "*", pos: start}, nil
	case '%':
		return token{kind: tokenModulo, text: "%", pos: start}, nil
	case '=':
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return token{kind: tokenEqual, text: "==", pos: start}, nil
		}
		return token{}, fmt.Errorf("assignments are not supported at byte %d", start)
	case '!':
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return token{kind: tokenNotEqual, text: "!=", pos: start}, nil
		}
		return token{}, fmt.Errorf("use 'not' for boolean negation at byte %d", start)
	case '<':
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return token{kind: tokenLessEqual, text: "<=", pos: start}, nil
		}
		return token{kind: tokenLess, text: "<", pos: start}, nil
	case '>':
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return token{kind: tokenGreaterEqual, text: ">=", pos: start}, nil
		}
		return token{kind: tokenGreater, text: ">", pos: start}, nil
	case '$':
		if l.pos == len(l.input) || !isIdentifierStart(l.input[l.pos]) {
			return token{}, fmt.Errorf("invalid variable at byte %d", start)
		}
		nameStart := l.pos
		for l.pos < len(l.input) && isIdentifierContinue(l.input[l.pos]) {
			l.pos++
		}
		return token{kind: tokenVariable, text: l.input[nameStart:l.pos], pos: start}, nil
	case '"':
		for l.pos < len(l.input) {
			switch l.input[l.pos] {
			case '\\':
				l.pos += 2
			case '"':
				l.pos++
				raw := l.input[start:l.pos]
				if err := validateSurrogates(raw); err != nil {
					return token{}, fmt.Errorf("invalid string literal at byte %d: %w", start, err)
				}
				var decoded string
				if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
					return token{}, fmt.Errorf("invalid string literal at byte %d", start)
				}
				return token{kind: tokenString, text: decoded, pos: start}, nil
			default:
				l.pos++
			}
		}
		return token{}, fmt.Errorf("unterminated string at byte %d", start)
	default:
		if isASCIIDigit(c) {
			l.pos = start
			return l.number()
		}
		if isIdentifierStart(c) {
			for l.pos < len(l.input) && isIdentifierContinue(l.input[l.pos]) {
				l.pos++
			}
			return token{kind: tokenIdentifier, text: l.input[start:l.pos], pos: start}, nil
		}
		return token{}, fmt.Errorf("unexpected character %q at byte %d", c, start)
	}
}

func (l *lexer) number() (token, error) {
	start := l.pos
	leadingDot := l.input[l.pos] == '.'
	if leadingDot {
		l.pos++
	}
	for l.pos < len(l.input) && isASCIIDigit(l.input[l.pos]) {
		l.pos++
	}
	if !leadingDot && l.pos < len(l.input) && l.input[l.pos] == '.' {
		l.pos++
		for l.pos < len(l.input) && isASCIIDigit(l.input[l.pos]) {
			l.pos++
		}
	}
	if l.pos < len(l.input) && (l.input[l.pos] == 'e' || l.input[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.input) && (l.input[l.pos] == '+' || l.input[l.pos] == '-') {
			l.pos++
		}
		if l.pos == len(l.input) || !isASCIIDigit(l.input[l.pos]) {
			return token{}, fmt.Errorf("invalid number at byte %d", start)
		}
		for l.pos < len(l.input) && isASCIIDigit(l.input[l.pos]) {
			l.pos++
		}
	}
	return token{kind: tokenNumber, text: l.input[start:l.pos], pos: start}, nil
}

func isFilterSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentifierStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentifierContinue(c byte) bool {
	return isIdentifierStart(c) || isASCIIDigit(c)
}
