// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tokenKind identifies the lexical class of a token. The classes mirror the
// terminals of the SECL EBNF grammar in pkg/security/secl/compiler/ast.
type tokenKind uint8

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokPattern
	tokRegexp
	tokInt
	tokDuration
	tokIP
	tokCIDR
	tokVariable
	tokFieldRef
	tokPunct
)

func (k tokenKind) String() string {
	switch k {
	case tokEOF:
		return "<EOF>"
	case tokIdent:
		return "identifier"
	case tokString:
		return "string"
	case tokPattern:
		return "pattern"
	case tokRegexp:
		return "regexp"
	case tokInt:
		return "integer"
	case tokDuration:
		return "duration"
	case tokIP:
		return "IP"
	case tokCIDR:
		return "CIDR"
	case tokVariable:
		return "variable"
	case tokFieldRef:
		return "field reference"
	case tokPunct:
		return "operator"
	}
	return "unknown"
}

// token is a single SECL lexeme.
type token struct {
	kind tokenKind

	// val is the semantic value of the token: the outer quotes are stripped from
	// strings, patterns and regexps, and the `${`/`%{` wrapper is stripped from
	// variables and field references. Escape sequences are *not* expanded, which
	// matches the SECL lexer.
	val string

	// num holds the value of a tokInt, and the nanosecond value of a tokDuration.
	num int64

	// start and end are byte offsets into the source, end being exclusive.
	start, end int
}

// The SECL lexer is an ordered EBNF lexer: at every position the first
// production that matches wins. These expressions replicate those productions,
// and lex() applies them in the same order.
var (
	reIPv4     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+`)
	reIPv6     = regexp.MustCompile(`^[0-9a-fA-F]*:[0-9a-fA-F]*:[0-9a-fA-F]*(?:[:.][0-9a-fA-F]*){0,5}`)
	rePrefix   = regexp.MustCompile(`^/[0-9]+`)
	reVariable = regexp.MustCompile(`^\$\{[a-zA-Z_][a-zA-Z0-9_.]*\}`)
	reFieldRef = regexp.MustCompile(`^%\{[a-zA-Z_][a-zA-Z0-9_.\[\]]*\}`)
	reDuration = regexp.MustCompile(`^[0-9]+[msh]s*`)
	reIdent    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.\[\]]*`)
	reInt      = regexp.MustCompile(`^[-+]?[0-9]+`)
)

// punctuation lists the single characters SECL lexes as operators. Note that a
// bare `/` is absent: it only ever appears inside a CIDR or a `//` comment.
const punctuation = "!=<>+-[](),&|~^%"

// lex splits a SECL expression into tokens.
func lex(src string) ([]token, *ParseError) {
	var toks []token
	pos := 0

	for {
		pos = skipIgnored(src, pos)
		if pos >= len(src) {
			break
		}

		tok, err := lexOne(src, pos)
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		pos = tok.end
	}

	return append(toks, token{kind: tokEOF, start: pos, end: pos}), nil
}

// skipIgnored advances past whitespace and comments. SECL treats only space,
// tab and newline as whitespace.
func skipIgnored(src string, pos int) int {
	for pos < len(src) {
		switch {
		case src[pos] == ' ' || src[pos] == '\t' || src[pos] == '\n':
			pos++
		case src[pos] == '#', strings.HasPrefix(src[pos:], "//"):
			if end := strings.IndexByte(src[pos:], '\n'); end >= 0 {
				pos += end
			} else {
				pos = len(src)
			}
		default:
			return pos
		}
	}
	return pos
}

func lexOne(src string, pos int) (token, *ParseError) {
	rest := src[pos:]

	// CIDR and IP come first so that `10.0.0.0/8` and `10.0.0.1` are not lexed
	// as integers.
	if n := matchIP(rest); n > 0 {
		if p := rePrefix.FindString(rest[n:]); p != "" {
			return token{kind: tokCIDR, val: rest[:n+len(p)], start: pos, end: pos + n + len(p)}, nil
		}
		return token{kind: tokIP, val: rest[:n], start: pos, end: pos + n}, nil
	}

	if m := reVariable.FindString(rest); m != "" {
		return token{kind: tokVariable, val: m[2 : len(m)-1], start: pos, end: pos + len(m)}, nil
	}

	if m := reFieldRef.FindString(rest); m != "" {
		return token{kind: tokFieldRef, val: m[2 : len(m)-1], start: pos, end: pos + len(m)}, nil
	}

	// Durations precede identifiers and integers: `10m` is a duration, not the
	// integer 10 followed by the identifier m.
	if m := reDuration.FindString(rest); m != "" {
		d, err := time.ParseDuration(m)
		if err != nil {
			return token{}, &ParseError{Offset: pos, Message: "invalid duration " + strconv.Quote(m)}
		}
		return token{kind: tokDuration, val: m, num: d.Nanoseconds(), start: pos, end: pos + len(m)}, nil
	}

	// Regexps precede identifiers so that `r"..."` is not lexed as the
	// identifier r.
	if strings.HasPrefix(rest, `r"`) {
		return lexQuoted(src, pos, tokRegexp, 1)
	}

	if m := reIdent.FindString(rest); m != "" {
		return token{kind: tokIdent, val: m, start: pos, end: pos + len(m)}, nil
	}

	if rest[0] == '"' {
		return lexQuoted(src, pos, tokString, 0)
	}

	if strings.HasPrefix(rest, `~"`) {
		return lexQuoted(src, pos, tokPattern, 1)
	}

	if m := reInt.FindString(rest); m != "" {
		n, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			return token{}, &ParseError{Offset: pos, Message: "integer " + m + " out of range"}
		}
		return token{kind: tokInt, val: m, num: n, start: pos, end: pos + len(m)}, nil
	}

	if strings.IndexByte(punctuation, rest[0]) >= 0 {
		return token{kind: tokPunct, val: rest[:1], start: pos, end: pos + 1}, nil
	}

	return token{}, &ParseError{Offset: pos, Message: "unexpected character " + strconv.QuoteRune(rune(rest[0]))}
}

// matchIP returns the length of the IP address at the start of s, or 0.
func matchIP(s string) int {
	if m := reIPv4.FindString(s); m != "" {
		return len(m)
	}
	return len(reIPv6.FindString(s))
}

// lexQuoted reads a double quoted literal that starts prefixLen bytes into the
// token (0 for a string, 1 for the `r` of a regexp or the `~` of a pattern).
// Like the SECL lexer it recognises `\x` pairs when looking for the closing
// quote but leaves them in the value untouched.
func lexQuoted(src string, pos int, kind tokenKind, prefixLen int) (token, *ParseError) {
	i := pos + prefixLen + 1 // skip the prefix and the opening quote
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '"':
			return token{
				kind:  kind,
				val:   src[pos+prefixLen+1 : i],
				start: pos,
				end:   i + 1,
			}, nil
		default:
			i++
		}
	}
	return token{}, &ParseError{Offset: pos, Message: "unterminated " + kind.String()}
}
