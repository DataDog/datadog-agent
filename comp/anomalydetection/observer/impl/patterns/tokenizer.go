// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxTokenizedStringLength = 12500
	DefaultMaxNumTokens             = 250
	LatestSignatureVersion          = 8
)

type Tokenizer struct {
	MaxStringLen int
	MaxTokens    int
	ParseHexDump bool

	// scratch is a reusable working buffer for Tokenize. The returned slice is
	// always a fresh exact-sized copy, so callers can retain it safely; the
	// scratch only amortizes growth across calls. A single Tokenizer is not
	// safe for concurrent use.
	scratch []Token
}

func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		MaxStringLen: DefaultMaxTokenizedStringLength,
		MaxTokens:    DefaultMaxNumTokens,
		ParseHexDump: true,
	}
}

func (t *Tokenizer) Tokenize(message string) []Token {
	msg := strings.TrimRight(message, " \t\r\n")
	if len(msg) > t.MaxStringLen {
		msg = msg[:t.MaxStringLen]
	}
	if len(msg) == 0 {
		return nil
	}

	// Most logs produce ~len(msg)/4 tokens; size scratch so the inner appends
	// rarely grow it. Not a hard cap — it can still grow.
	estTokens := len(msg)/4 + 8
	if estTokens > t.MaxTokens {
		estTokens = t.MaxTokens
	}
	if cap(t.scratch) < estTokens {
		t.scratch = make([]Token, 0, estTokens)
	}
	tokens := t.scratch[:0]
	pos := 0

	for pos < len(msg) && len(tokens) < t.MaxTokens {
		tok, advance := t.matchAt(msg, pos)
		if advance <= 0 {
			tokens = append(tokens, SpecialCharToken(msg[pos]))
			pos++
			continue
		}
		tokens = append(tokens, tok)
		pos += advance
	}

	// Stash the (possibly grown) buffer for the next call.
	t.scratch = tokens
	// Return a fresh exact-sized slice so callers can safely retain it.
	out := make([]Token, len(tokens))
	copy(out, tokens)
	return out
}

func (t *Tokenizer) matchAt(msg string, pos int) (Token, int) {
	s := msg[pos:]
	c := s[0]
	cc := charClass[c]

	switch {
	case cc&ccDigit != 0:
		// Hex-dump heads start with a hex digit; ISO dates start with 4 digits;
		// CLF dates / local times / IPv4 all start with digits.
		if t.ParseHexDump {
			if tok, n := tryHexDump(s, msg, pos); n > 0 {
				return tok, n
			}
		}
		if tok, n := tryOffsetDateTime(s); n > 0 {
			return tok, n
		}
		if tok, n := tryLocalDateTime(s); n > 0 {
			return tok, n
		}
		if tok, n := tryCLFDateTime(s); n > 0 {
			return tok, n
		}
		if tok, n := tryLocalDate(s); n > 0 {
			return tok, n
		}
		if tok, n := tryCLFDate(s); n > 0 {
			return tok, n
		}
		if tok, n := tryLocalTime(s); n > 0 {
			return tok, n
		}
		// Email local-parts can start with a digit (e.g. 123@example.com)
		// or even look IPv4-shaped (e.g. 192.168.1.1@example.com); try
		// before tryIPv4Authority so the full address tokenizes as one email.
		if tok, n := tryEmail(s); n > 0 {
			return tok, n
		}
		if tok, n := tryIPv4Authority(s, msg, pos); n > 0 {
			return tok, n
		}
		if tok, n := tryAlphanumeric(s); n > 0 {
			return tok, n
		}

	case cc&ccAlpha != 0:
		// Hex-dump head can also start with a-f / A-F.
		if t.ParseHexDump && cc&ccHexAlpha != 0 {
			if tok, n := tryHexDump(s, msg, pos); n > 0 {
				return tok, n
			}
		}
		// URI scheme is one of "http", "https", "ftp" — only h/f can start one.
		if c == 'h' || c == 'f' {
			if tok, n := tryURI(s); n > 0 {
				return tok, n
			}
		}
		if tok, n := tryEmail(s); n > 0 {
			return tok, n
		}
		if tok, n := tryAlphanumeric(s); n > 0 {
			return tok, n
		}

	case cc&ccSpaceTab != 0:
		if tok, n := tryWhitespace(s); n > 0 {
			return tok, n
		}

	case cc&ccUnder != 0:
		// Underscore is a valid email local-part start (e.g. _svc@example.com).
		if tok, n := tryEmail(s); n > 0 {
			return tok, n
		}
		// Underscore is alphanumeric-extended; falls into tryWordOrKeyword.
		if tok, n := tryAlphanumeric(s); n > 0 {
			return tok, n
		}

	default:
		switch c {
		case '/':
			if tok, n := tryPath(s, msg, pos); n > 0 {
				return tok, n
			}
		case '-':
			// Negative number iff next byte is a digit.
			if tok, n := tryAlphanumeric(s); n > 0 {
				return tok, n
			}
		}
	}

	return SpecialCharToken(c), 1
}

// ---- Hex dump ----

// scanHexDumpHead matches `^[0-9A-Fa-f]{4,8}:[ \t]+(?:[0-9A-Fa-f]{2}[ \t]+)*[0-9A-Fa-f]{2}`,
// equivalent to the original hexDumpRe. Returns the displacement length, the
// number of hex byte groups parsed, and the total bytes consumed.
func scanHexDumpHead(s string) (dispLen, numBytes, n int, ok bool) {
	// Displacement: 4-8 hex digits.
	i := 0
	for i < len(s) && i < 8 && isHexByte(s[i]) {
		i++
	}
	if i < 4 {
		return
	}
	dispLen = i
	if i >= len(s) || s[i] != ':' {
		return
	}
	i++
	wsStart := i
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i == wsStart {
		return
	}
	// At least one hex byte (2 hex digits).
	if i+2 > len(s) || !isHexByte(s[i]) || !isHexByte(s[i+1]) {
		return
	}
	i += 2
	numBytes = 1
	// Subsequent (whitespace + 2 hex digits) groups.
	for {
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j == i {
			break
		}
		if j+2 > len(s) || !isHexByte(s[j]) || !isHexByte(s[j+1]) {
			break
		}
		i = j + 2
		numBytes++
	}
	return dispLen, numBytes, i, true
}

func tryHexDump(s, fullMsg string, pos int) (Token, int) {
	dispLen, numBytes, totalMatch, ok := scanHexDumpHead(s)
	if !ok {
		return Token{}, 0
	}

	// Skip trailing whitespace after last hex byte
	trailingWS := totalMatch
	for trailingWS < len(s) && (s[trailingWS] == ' ' || s[trailingWS] == '\t') {
		trailingWS++
	}

	rest := s[trailingWS:]
	hasASCII := false
	asciiEnd := 0

	if len(rest) > 0 && rest[0] != '\n' && rest[0] != '\r' {
		nextSpace := strings.IndexAny(rest, " \t\n\r")
		var candidate string
		if nextSpace < 0 {
			candidate = rest
		} else {
			candidate = rest[:nextSpace]
		}
		if len(candidate) > 0 && len(candidate) <= numBytes {
			allPrintable := true
			for _, c := range candidate {
				if c < 0x20 || c > 0x7e {
					allPrintable = false
					break
				}
			}
			if allPrintable {
				hasASCII = true
				asciiEnd = len(candidate)
			}
		}
	}

	endPos := trailingWS + asciiEnd
	text := s[:endPos]

	fullEnd := pos + endPos
	if fullEnd < len(fullMsg) {
		ch := fullMsg[fullEnd]
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' {
			if hasASCII {
				hasASCII = false
				endPos = trailingWS
				text = strings.TrimRight(s[:endPos], " \t")
				endPos = len(text)
			}
		}
	}

	return HexDumpToken(text, dispLen, hasASCII), endPos
}

// ---- Date/Time scanners ----
//
// Hand-coded byte scanners replacing the original regex matchers. Each scanner
// is a tiny prefix-match against a known shape; on a hit it returns enough
// information for the caller to build the same DateToken/LocalTimeToken value
// (raw text + format string) the regex version would have produced.

// readDigits reads up to maxN consecutive ASCII digits at s[i:i+maxN].
// Returns the parsed integer value and the count of digits read.
func readDigits(s string, i, maxN int) (val, n int) {
	end := i + maxN
	if end > len(s) {
		end = len(s)
	}
	for j := i; j < end && isDigit(s[j]); j++ {
		val = val*10 + int(s[j]-'0')
		n++
	}
	return val, n
}

// scanISODate matches `\d{4}[-/]\d{2}[-/]\d{2}` with both separators equal.
// Returns the chosen separator, the parsed year/month/day, and the number of
// bytes consumed (always 10 on success).
func scanISODate(s string) (sep byte, year, month, day, n int, ok bool) {
	if len(s) < 10 {
		return
	}
	var c int
	if year, c = readDigits(s, 0, 4); c != 4 {
		return
	}
	sep = s[4]
	if sep != '-' && sep != '/' {
		return
	}
	if month, c = readDigits(s, 5, 2); c != 2 || s[7] != sep {
		return
	}
	if day, c = readDigits(s, 8, 2); c != 2 {
		return
	}
	return sep, year, month, day, 10, true
}

// scanTime matches `\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?` at s[i:]. Returns the
// parsed hour/min/sec, the fractional-digits substring (zero-length when
// absent), and total bytes consumed from i.
func scanTime(s string, i int) (hour, min, sec int, frac string, n int, ok bool) {
	if i+8 > len(s) {
		return
	}
	var c int
	if hour, c = readDigits(s, i, 2); c != 2 || s[i+2] != ':' {
		return
	}
	if min, c = readDigits(s, i+3, 2); c != 2 || s[i+5] != ':' {
		return
	}
	if sec, c = readDigits(s, i+6, 2); c != 2 {
		return
	}
	end := i + 8
	if end < len(s) && s[end] == '.' {
		if _, fc := readDigits(s, end+1, 9); fc >= 1 {
			frac = s[end+1 : end+1+fc]
			end += 1 + fc
		}
	}
	return hour, min, sec, frac, end - i, true
}

// scanISOTZ matches `Z | [+-]\d{2}:?\d{2}` at s[i:].
func scanISOTZ(s string, i int) (tz string, n int, ok bool) {
	if i >= len(s) {
		return
	}
	if s[i] == 'Z' {
		return "Z", 1, true
	}
	if s[i] != '+' && s[i] != '-' {
		return
	}
	if i+5 > len(s) || !isDigit(s[i+1]) || !isDigit(s[i+2]) {
		return
	}
	j := i + 3
	if s[j] == ':' {
		j++
	}
	if j+2 > len(s) || !isDigit(s[j]) || !isDigit(s[j+1]) {
		return
	}
	end := j + 2
	return s[i:end], end - i, true
}

// scanCLFTZ matches `\s+[+-]\d{4}` at s[i:].
func scanCLFTZ(s string, i int) (tz string, n int, ok bool) {
	j := i
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	if j == i {
		return
	}
	if j+5 > len(s) {
		return
	}
	if s[j] != '+' && s[j] != '-' {
		return
	}
	for k := 1; k <= 4; k++ {
		if !isDigit(s[j+k]) {
			return
		}
	}
	end := j + 5
	return s[j:end], end - i, true
}

// appendISODateFormat writes "yyyy{sep}MM{sep}ddT{HH:mm:ss}" or its space-T
// variant into b. tSep is 'T' or ' '. The original code went through `q()` and
// `cleanDateFormat` to strip quote markers; we just build the cleaned form.
func appendISODateFormat(b *strings.Builder, sep, tSep byte) {
	b.WriteString("yyyy")
	b.WriteByte(sep)
	b.WriteString("MM")
	b.WriteByte(sep)
	b.WriteString("dd")
	b.WriteByte(tSep)
	b.WriteString("HH:mm:ss")
}

func appendCLFDateFormat(b *strings.Builder, tSep byte) {
	b.WriteString("dd/MMM/yyyy")
	b.WriteByte(tSep)
	b.WriteString("HH:mm:ss")
}

func appendFrac(b *strings.Builder, frac string) {
	if frac == "" {
		return
	}
	b.WriteByte('.')
	for range frac {
		b.WriteByte('S')
	}
}

func tryOffsetDateTime(s string) (Token, int) {
	sep, year, month, day, dn, ok := scanISODate(s)
	if !ok {
		return Token{}, 0
	}
	if dn >= len(s) {
		return Token{}, 0
	}
	tSep := s[dn]
	if tSep != 'T' && tSep != ' ' {
		return Token{}, 0
	}
	hour, min, sec, frac, tn, ok := scanTime(s, dn+1)
	if !ok {
		return Token{}, 0
	}
	end := dn + 1 + tn
	tzStr, tzN, ok := scanISOTZ(s, end)
	if !ok {
		return Token{}, 0
	}
	end += tzN
	if !validateDate(year, month, day) || hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}
	if isFollowedByAlphanumeric(s, end) {
		return Token{}, 0
	}

	var b strings.Builder
	b.Grow(32)
	appendISODateFormat(&b, sep, tSep)
	appendFrac(&b, frac)
	b.WriteString(tzFormat(tzStr))
	return DateToken(b.String(), s[:end]), end
}

func tryLocalDateTime(s string) (Token, int) {
	sep, year, month, day, dn, ok := scanISODate(s)
	if !ok {
		return Token{}, 0
	}
	if dn >= len(s) {
		return Token{}, 0
	}
	tSep := s[dn]
	if tSep != 'T' && tSep != ' ' {
		return Token{}, 0
	}
	hour, min, sec, frac, tn, ok := scanTime(s, dn+1)
	if !ok {
		return Token{}, 0
	}
	end := dn + 1 + tn
	if !validateDate(year, month, day) || hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}
	if isFollowedByAlphanumeric(s, end) {
		return Token{}, 0
	}

	var b strings.Builder
	b.Grow(24)
	appendISODateFormat(&b, sep, tSep)
	appendFrac(&b, frac)
	return DateToken(b.String(), s[:end]), end
}

// scanCLFDate matches `\d{2}/[A-Za-z]{3}/\d{4}`. Returns parsed day, month
// (1-12), year, and bytes consumed.
func scanCLFDate(s string) (day, month, year, n int, ok bool) {
	if len(s) < 11 {
		return
	}
	var c int
	if day, c = readDigits(s, 0, 2); c != 2 || s[2] != '/' {
		return
	}
	if !isAlpha(s[3]) || !isAlpha(s[4]) || !isAlpha(s[5]) || s[6] != '/' {
		return
	}
	m := parseMonthAbbr(s[3:6])
	if m == 0 {
		return
	}
	if year, c = readDigits(s, 7, 4); c != 4 {
		return
	}
	return day, int(m), year, 11, true
}

func tryCLFDateTime(s string) (Token, int) {
	day, month, year, dn, ok := scanCLFDate(s)
	if !ok {
		return Token{}, 0
	}
	if dn >= len(s) {
		return Token{}, 0
	}
	tSep := s[dn]
	if tSep != ':' && tSep != ' ' {
		return Token{}, 0
	}
	hour, min, sec, frac, tn, ok := scanTime(s, dn+1)
	if !ok {
		return Token{}, 0
	}
	end := dn + 1 + tn
	tzStr, tzN, hasTZ := scanCLFTZ(s, end)
	if hasTZ {
		end += tzN
	}
	if !validateDate(year, month, day) || hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}
	if isFollowedByAlphanumeric(s, end) {
		return Token{}, 0
	}

	var b strings.Builder
	b.Grow(32)
	appendCLFDateFormat(&b, tSep)
	appendFrac(&b, frac)
	if hasTZ {
		b.WriteByte(' ')
		b.WriteString(tzSigFormat(tzStr))
	}
	return DateToken(b.String(), s[:end]), end
}

func tryLocalDate(s string) (Token, int) {
	sep, year, month, day, dn, ok := scanISODate(s)
	if !ok {
		return Token{}, 0
	}
	if !validateDate(year, month, day) {
		return Token{}, 0
	}
	if isFollowedByAlphanumeric(s, dn) {
		return Token{}, 0
	}
	var b strings.Builder
	b.Grow(10)
	b.WriteString("yyyy")
	b.WriteByte(sep)
	b.WriteString("MM")
	b.WriteByte(sep)
	b.WriteString("dd")
	return DateToken(b.String(), s[:dn]), dn
}

func tryCLFDate(s string) (Token, int) {
	day, month, year, dn, ok := scanCLFDate(s)
	if !ok {
		return Token{}, 0
	}
	if !validateDate(year, month, day) {
		return Token{}, 0
	}
	if isFollowedByAlphanumeric(s, dn) {
		return Token{}, 0
	}
	return DateToken("dd/MMM/yyyy", s[:dn]), dn
}

func tryLocalTime(s string) (Token, int) {
	hour, min, sec, frac, n, ok := scanTime(s, 0)
	if !ok {
		return Token{}, 0
	}
	if hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}
	if isFollowedByAlphanumeric(s, n) {
		return Token{}, 0
	}
	if frac == "" {
		return LocalTimeToken("HH:mm:ss", s[:n]), n
	}
	var b strings.Builder
	b.Grow(8 + 1 + len(frac))
	b.WriteString("HH:mm:ss")
	appendFrac(&b, frac)
	return LocalTimeToken(b.String(), s[:n]), n
}

// ---- URI ----

// scanURIScheme matches `^(https?|ftp)://` and returns the scheme without "://"
// plus the total bytes consumed.
func scanURIScheme(s string) (scheme string, n int) {
	switch {
	case len(s) >= 8 && s[0] == 'h' && s[1] == 't' && s[2] == 't' && s[3] == 'p' && s[4] == 's' && s[5] == ':' && s[6] == '/' && s[7] == '/':
		return "https", 8
	case len(s) >= 7 && s[0] == 'h' && s[1] == 't' && s[2] == 't' && s[3] == 'p' && s[4] == ':' && s[5] == '/' && s[6] == '/':
		return "http", 7
	case len(s) >= 6 && s[0] == 'f' && s[1] == 't' && s[2] == 'p' && s[3] == ':' && s[4] == '/' && s[5] == '/':
		return "ftp", 6
	}
	return "", 0
}

func tryURI(s string) (Token, int) {
	scheme, schemeLen := scanURIScheme(s)
	if schemeLen == 0 {
		return Token{}, 0
	}
	rest := s[schemeLen:]

	authEnd := strings.IndexAny(rest, "/?# \t\n\r")
	var authStr string
	if authEnd < 0 {
		authStr = rest
		authEnd = len(rest)
	} else {
		authStr = rest[:authEnd]
	}
	if authStr == "" {
		return Token{}, 0
	}

	auth := parseAuthority(authStr)
	totalLen := schemeLen + authEnd

	afterAuth := rest[authEnd:]
	var pathTok *Token
	var query, fragment *string

	if len(afterAuth) > 0 && afterAuth[0] == '/' {
		pathEnd := strings.IndexAny(afterAuth, "?# \t\n\r")
		if pathEnd < 0 {
			pathEnd = len(afterAuth)
		}
		pathStr := afterAuth[:pathEnd]
		segs := parsePathSegments(pathStr)
		p := PathToken(segs)
		pathTok = &p
		totalLen += pathEnd
		afterAuth = afterAuth[pathEnd:]
	} else {
		segs := []string{""}
		p := PathToken(segs)
		pathTok = &p
	}

	if len(afterAuth) > 0 && afterAuth[0] == '?' {
		qEnd := strings.IndexAny(afterAuth[1:], "# \t\n\r")
		if qEnd < 0 {
			qEnd = len(afterAuth) - 1
		}
		q := afterAuth[1 : qEnd+1]
		query = &q
		totalLen += qEnd + 1
		afterAuth = afterAuth[qEnd+1:]
	}

	if len(afterAuth) > 0 && afterAuth[0] == '#' {
		fEnd := strings.IndexAny(afterAuth[1:], " \t\n\r")
		if fEnd < 0 {
			fEnd = len(afterAuth) - 1
		}
		f := afterAuth[1 : fEnd+1]
		fragment = &f
		totalLen += fEnd + 1
	}

	return uriTokenRaw(s[:totalLen], scheme, &auth, pathTok, query, fragment), totalLen
}

func parseAuthority(s string) Token {
	userInfo := ""
	hasUser := false
	atIdx := strings.Index(s, "@")
	host := s
	if atIdx >= 0 {
		userInfo = s[:atIdx]
		hasUser = true
		host = s[atIdx+1:]
	}

	portVal := 0
	hasPort := false
	if colonIdx := strings.LastIndex(host, ":"); colonIdx >= 0 {
		portStr := host[colonIdx+1:]
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
			portVal = p
			hasPort = true
			host = host[:colonIdx]
		}
	}

	hostTok := parseHost(host)
	return authorityTokenRaw(s, &hostTok, portVal, hasPort, userInfo, hasUser)
}

func parseHost(s string) Token {
	if isIPv4(s) {
		return Token{Type: TypeIPv4Address, Value: s}
	}
	if strings.Contains(s, ":") {
		return Token{Type: TypeIPv6Address, Value: s}
	}
	return Token{Type: TypeWord, Value: s}
}

// ---- Email ----

// isEmailLocalChar mirrors the original regex's local-part class
// `[a-zA-Z0-9._%+()"\ -]`.
func isEmailLocalChar(b byte) bool {
	if isAlphaNum(b) {
		return true
	}
	switch b {
	case '.', '%', '+', '(', ')', '"', ' ', '-':
		return true
	}
	return false
}

// isEmailDomainChar mirrors `[a-zA-Z0-9.-]`.
func isEmailDomainChar(b byte) bool {
	if isAlphaNum(b) {
		return true
	}
	return b == '.' || b == '-'
}

func tryEmail(s string) (Token, int) {
	// local-part: 1+ chars from the local-part class.
	i := 0
	for i < len(s) && isEmailLocalChar(s[i]) {
		i++
	}
	if i == 0 {
		return Token{}, 0
	}
	local := s[:i]
	// optional whitespace before '@'.
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) || s[i] != '@' {
		return Token{}, 0
	}
	i++
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	domStart := i
	for i < len(s) && isEmailDomainChar(s[i]) {
		i++
	}
	if i == domStart {
		return Token{}, 0
	}
	// Trailing whitespace is captured into the domain group by the original
	// regex (`([a-zA-Z0-9.-]+\s*)`); preserve that.
	domEnd := i
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	domain := s[domStart:i]
	_ = domEnd
	return emailTokenRaw(s[:i], local, domain), i
}

// ---- IPv4 with optional port (standalone Authority) ----

// scanIPv4 matches `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::(\d+))?`. Returns
// total bytes consumed, port (0 if absent), whether a port was matched, and
// the start index of the port digits in s (used by the caller to recover the
// canonical port string).
func scanIPv4(s string) (n, port int, hasPort, ok bool) {
	idx := 0
	for octet := 0; octet < 4; octet++ {
		val, c := readDigits(s, idx, 3)
		if c == 0 || val > 255 {
			return
		}
		idx += c
		if octet < 3 {
			if idx >= len(s) || s[idx] != '.' {
				return
			}
			idx++
		}
	}
	totalLen := idx
	if idx < len(s) && s[idx] == ':' {
		val, c := readDigits(s, idx+1, 10)
		if c > 0 {
			port = val
			hasPort = true
			idx += 1 + c
		}
	}
	return idx, port, hasPort, totalLen > 0
}

func tryIPv4Authority(s, _ string, _ int) (Token, int) {
	totalLen, portVal, hasPort, ok := scanIPv4(s)
	if !ok {
		return Token{}, 0
	}

	// If followed by "/" it's part of a path, not a standalone IP
	if totalLen < len(s) && s[totalLen] == '/' {
		return Token{}, 0
	}

	// If followed by alphanumeric (but not port), it's part of a word
	if isFollowedByAlphanumeric(s, totalLen) {
		return Token{}, 0
	}

	// IP string is the input slice up to (but not including) any ':port' suffix.
	ipEnd := totalLen
	if hasPort {
		// Walk back to the colon.
		for ipEnd > 0 && s[ipEnd-1] != ':' {
			ipEnd--
		}
		ipEnd-- // drop the ':'
	}
	ipTok := Token{Type: TypeIPv4Address, Value: s[:ipEnd]}

	return authorityTokenRaw(s[:totalLen], &ipTok, portVal, hasPort, "", false), totalLen
}

// ---- Path ----

func tryPath(s, fullMsg string, pos int) (Token, int) {
	if len(s) < 2 || s[0] != '/' {
		return Token{}, 0
	}

	// Don't start a path after a word character (e.g., "CUS-8007/path")
	if pos > 0 && isWordChar(fullMsg[pos-1]) {
		return Token{}, 0
	}

	if !isAlphaNum(s[1]) {
		return Token{}, 0
	}

	// Check if this looks like /IP:port (not a path)
	if isDigit(s[1]) && looksLikeIPAfterSlash(s[1:]) {
		return Token{}, 0 // let "/" be a special char, IP matched next
	}

	pathEnd := 1
	for pathEnd < len(s) && !isWhitespace(s[pathEnd]) &&
		s[pathEnd] != '#' && s[pathEnd] != '?' && s[pathEnd] != '"' {
		pathEnd++
	}
	pathStr := s[1:pathEnd]
	totalLen := pathEnd

	var query, fragment *string
	rest := s[totalLen:]

	if len(rest) > 0 && rest[0] == '?' {
		qEnd := strings.IndexAny(rest[1:], "# \t\n\r\"")
		if qEnd < 0 {
			qEnd = len(rest) - 1
		}
		q := rest[1 : qEnd+1]
		query = &q
		totalLen += qEnd + 1
		rest = s[totalLen:]
	}

	if len(rest) > 0 && rest[0] == '#' {
		fEnd := strings.IndexAny(rest[1:], " \t\n\r\"")
		if fEnd < 0 {
			fEnd = len(rest) - 1
		}
		f := rest[1 : fEnd+1]
		fragment = &f
		totalLen += fEnd + 1
	}

	segments := strings.Split(pathStr, "/")

	return pathQueryFragmentTokenRaw(s[:totalLen], segments, query, fragment), totalLen
}

func looksLikeIPAfterSlash(s string) bool {
	matchLen, _, _, ok := scanIPv4(s)
	if !ok {
		return false
	}
	// Only treat as IP if not followed by more path segments
	if matchLen < len(s) && s[matchLen] == '/' {
		return false
	}
	return true
}

func parsePathSegments(pathStr string) []string {
	if len(pathStr) > 0 && pathStr[0] == '/' {
		pathStr = pathStr[1:]
	}
	if pathStr == "" {
		return []string{""}
	}
	return strings.Split(pathStr, "/")
}

// ---- Whitespace ----

func tryWhitespace(s string) (Token, int) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i == 0 {
		return Token{}, 0
	}
	return WhitespaceToken(i), i
}

// ---- Alphanumeric (word, number, HTTP method, severity, status code) ----

func tryAlphanumeric(s string) (Token, int) {
	if !isAlphaNum(s[0]) && s[0] != '-' && s[0] != '"' {
		return Token{}, 0
	}

	negative := false
	start := 0
	if s[0] == '-' {
		if len(s) < 2 || !isDigit(s[1]) {
			return Token{}, 0
		}
		negative = true
		start = 1
	}

	if isDigit(s[start]) && !negative {
		return tryNumberOrStatusCode(s)
	}

	if negative {
		return tryNegativeNumber(s)
	}

	return tryWordOrKeyword(s)
}

func tryNegativeNumber(s string) (Token, int) {
	i := 1
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' {
		if i+1 < len(s) && isDigit(s[i+1]) {
			i++
			for i < len(s) && isDigit(s[i]) {
				i++
			}
		} else {
			i++
		}
	}
	return NumericValueToken(s[:i]), i
}

func tryNumberOrStatusCode(s string) (Token, int) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}

	intPart := s[:i]

	// Check for HTTP status code (exactly 3 digits, specific valid codes)
	// before looking at what follows (handles "200OK" → HttpStatusCode + Word)
	if len(intPart) == 3 {
		code, _ := strconv.Atoi(intPart)
		if isValidHTTPStatus(code) {
			if i >= len(s) || s[i] != '.' {
				return HTTPStatusCodeToken(intPart), len(intPart)
			}
		}
	}

	hasDecimal := false
	if i < len(s) && s[i] == '.' {
		if i+1 < len(s) && isDigit(s[i+1]) {
			i++
			for i < len(s) && isDigit(s[i]) {
				i++
			}
			hasDecimal = true
		} else if i+1 >= len(s) || !isAlpha(s[i+1]) {
			i++
			hasDecimal = true
		}
	}

	if isFollowedByAlpha(s, i) && !hasDecimal {
		return tryWordStartingWithDigits(s)
	}

	return NumericValueToken(s[:i]), i
}

func tryWordStartingWithDigits(s string) (Token, int) {
	i := 0
	for i < len(s) && isWordChar(s[i]) {
		i++
	}
	text := s[:i]
	return WordToken(text), i
}

func isHTTPMethod(s string) bool {
	switch s {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		return true
	}
	return false
}

func isSeverityKeyword(s string) bool {
	switch s {
	case "INFO", "WARN", "WARNING", "ERROR", "ERR", "DEBUG", "TRACE",
		"FATAL", "CRITICAL", "ALERT", "EMERGENCY", "EMERG", "NOTICE":
		return true
	}
	return false
}

func tryWordOrKeyword(s string) (Token, int) {
	i := 0
	for i < len(s) && isWordChar(s[i]) {
		i++
	}

	text := s[:i]

	if isHTTPMethod(text) {
		return HTTPMethodToken(text), i
	}
	if isSeverityKeyword(text) {
		return SeverityToken(text), i
	}

	return WordToken(text), i
}

// ---- Date helpers ----

func validateDate(year, month, day int) bool {
	if month < 1 || month > 12 || day < 1 {
		return false
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return t.Year() == year && t.Month() == time.Month(month) && t.Day() == day
}

func parseMonthAbbr(s string) time.Month {
	if len(s) != 3 {
		return 0
	}
	switch s {
	case "Jan":
		return time.January
	case "Feb":
		return time.February
	case "Mar":
		return time.March
	case "Apr":
		return time.April
	case "May":
		return time.May
	case "Jun":
		return time.June
	case "Jul":
		return time.July
	case "Aug":
		return time.August
	case "Sep":
		return time.September
	case "Oct":
		return time.October
	case "Nov":
		return time.November
	case "Dec":
		return time.December
	}
	return 0
}

func tzFormat(tz string) string {
	if tz == "Z" {
		return "XXX"
	}
	if strings.Contains(tz, ":") {
		return "xxx"
	}
	return "xx"
}

func tzSigFormat(tz string) string {
	if strings.Contains(tz, ":") {
		return "xxx"
	}
	return "xx"
}

// ---- Character classification ----
//
// charClass is a 256-byte lookup table with bit flags for the char tests we
// run hottest. A single table read is faster than the per-byte comparison
// chains we previously had, and the compiler can lift the global into a
// register-friendly indexed load.

const (
	ccDigit    byte = 1 << 0 // 0-9
	ccAlpha    byte = 1 << 1 // a-z, A-Z
	ccUnder    byte = 1 << 2 // _
	ccDash     byte = 1 << 3 // -
	ccHexAlpha byte = 1 << 4 // a-f, A-F (combine with ccDigit for hex)
	ccSpaceTab byte = 1 << 5 // ' ' | '\t'
	ccLineEnd  byte = 1 << 6 // '\n' | '\r'
)

var charClass = func() [256]byte {
	var t [256]byte
	for c := byte('0'); c <= '9'; c++ {
		t[c] |= ccDigit
	}
	for c := byte('a'); c <= 'z'; c++ {
		t[c] |= ccAlpha
	}
	for c := byte('A'); c <= 'Z'; c++ {
		t[c] |= ccAlpha
	}
	for c := byte('a'); c <= 'f'; c++ {
		t[c] |= ccHexAlpha
	}
	for c := byte('A'); c <= 'F'; c++ {
		t[c] |= ccHexAlpha
	}
	t['_'] |= ccUnder
	t['-'] |= ccDash
	t[' '] |= ccSpaceTab
	t['\t'] |= ccSpaceTab
	t['\n'] |= ccLineEnd
	t['\r'] |= ccLineEnd
	return t
}()

func isDigit(b byte) bool      { return charClass[b]&ccDigit != 0 }
func isAlpha(b byte) bool      { return charClass[b]&ccAlpha != 0 }
func isAlphaNum(b byte) bool   { return charClass[b]&(ccDigit|ccAlpha|ccUnder) != 0 }
func isWhitespace(b byte) bool { return charClass[b]&(ccSpaceTab|ccLineEnd) != 0 }
func isHexByte(b byte) bool    { return charClass[b]&(ccDigit|ccHexAlpha) != 0 }

func isWordChar(b byte) bool {
	return charClass[b]&(ccDigit|ccAlpha|ccUnder|ccDash) != 0
}

func isIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

func isValidHTTPStatus(code int) bool {
	switch code {
	case 100, 101,
		200, 201, 202, 203, 204, 206, 207,
		300, 301, 302, 303, 304, 307, 308,
		400, 401, 402, 403, 404, 405,
		406, 407, 408, 409, 410, 411,
		413, 414, 415, 416, 422, 425,
		426, 429, 431, 451,
		500, 501, 502, 503, 504, 505,
		507, 508, 510, 511:
		return true
	}
	return false
}

func isFollowedByAlphanumeric(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	return isAlphaNum(s[pos])
}

func isFollowedByAlpha(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	return isAlpha(s[pos])
}

const (
	SeverityUnknown = iota
	// SeverityDebug represents logs that should have ~ no impact on anomaly detection
	SeverityDebug
	// SeverityInfo represents informational logs
	SeverityInfo
	// SeverityError represents logs that should have the most impact on anomaly detection and RCA (warning+ logs)
	SeverityError
)

// GetSeverity returns the severity of a log
// It is based on the first severity token found
func GetSeverity(tokens []Token) int {
	for _, token := range tokens {
		// We cannot use severity token type because it's not always parsed for that
		switch strings.ToUpper(token.Value) {
		case "DEBUG", "D":
			return SeverityDebug
		case "INFO", "I":
			return SeverityInfo
		case "ERROR", "ERR", "E", "WARNING", "WARN", "W":
			return SeverityError
		}
	}
	return SeverityUnknown
}

func GetSeverityString(severity int) string {
	switch severity {
	case SeverityDebug:
		return "DEBUG"
	case SeverityInfo:
		return "INFO"
	case SeverityError:
		return "ERROR"
	case SeverityUnknown:
		return "UNKNOWN"
	default:
		fmt.Fprintf(os.Stderr, "warning: Invalid severity: %d\n", severity)

		return "UNKNOWN"
	}
}
