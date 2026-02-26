// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
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

	var tokens []Token
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

	return tokens
}

func (t *Tokenizer) matchAt(msg string, pos int) (Token, int) {
	s := msg[pos:]

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

	if tok, n := tryURI(s); n > 0 {
		return tok, n
	}

	if tok, n := tryEmail(s); n > 0 {
		return tok, n
	}

	if tok, n := tryIPv4Authority(s, msg, pos); n > 0 {
		return tok, n
	}

	if tok, n := tryPath(s, msg, pos); n > 0 {
		return tok, n
	}

	if tok, n := tryWhitespace(s); n > 0 {
		return tok, n
	}

	if tok, n := tryAlphanumeric(s); n > 0 {
		return tok, n
	}

	return SpecialCharToken(s[0]), 1
}

// ---- Hex dump ----

var hexDumpRe = regexp.MustCompile(`^([0-9A-Fa-f]{4,8}):\s+((?:[0-9A-Fa-f]{2}\s+)*[0-9A-Fa-f]{2})`)

func tryHexDump(s, fullMsg string, pos int) (Token, int) {
	m := hexDumpRe.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	displacement := m[1]
	dispLen := len(displacement)
	hexPart := strings.TrimRight(m[2], " \t")
	byteParts := strings.Fields(hexPart)
	totalMatch := len(m[0])

	// Skip trailing whitespace after last hex byte
	trailingWS := totalMatch
	for trailingWS < len(s) && (s[trailingWS] == ' ' || s[trailingWS] == '\t') {
		trailingWS++
	}

	rest := s[trailingWS:]
	hasAscii := false
	asciiEnd := 0

	if len(rest) > 0 && rest[0] != '\n' && rest[0] != '\r' {
		nextSpace := strings.IndexAny(rest, " \t\n\r")
		var candidate string
		if nextSpace < 0 {
			candidate = rest
		} else {
			candidate = rest[:nextSpace]
		}
		if len(candidate) > 0 && len(candidate) <= len(byteParts) {
			allPrintable := true
			for _, c := range candidate {
				if c < 0x20 || c > 0x7e {
					allPrintable = false
					break
				}
			}
			if allPrintable {
				hasAscii = true
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
			if hasAscii {
				hasAscii = false
				endPos = trailingWS
				text = strings.TrimRight(s[:endPos], " \t")
				endPos = len(text)
			}
		}
	}

	return HexDumpToken(text, dispLen, hasAscii), endPos
}

// ---- Date/Time patterns ----

var reOffsetDateTime = regexp.MustCompile(
	`^(\d{4})([-/])(\d{2})[-/](\d{2})([T ])(\d{2}):(\d{2}):(\d{2})` +
		`(?:\.(\d{1,9}))?` +
		`(Z|[+-]\d{2}:?\d{2})`)

var reLocalDateTime = regexp.MustCompile(
	`^(\d{4})([-/])(\d{2})[-/](\d{2})([T ])(\d{2}):(\d{2}):(\d{2})` +
		`(?:\.(\d{1,9}))?`)

var reLocalDate = regexp.MustCompile(
	`^(\d{4})([-/])(\d{2})[-/](\d{2})`)

var reCLFDateTime = regexp.MustCompile(
	`^(\d{2})/([A-Za-z]{3})/(\d{4})([: ])(\d{2}):(\d{2}):(\d{2})` +
		`(?:\.(\d{1,9}))?` +
		`(?:\s+([+-]\d{4}))?`)

var reCLFDate = regexp.MustCompile(
	`^(\d{2})/([A-Za-z]{3})/(\d{4})`)

var reLocalTime = regexp.MustCompile(
	`^(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?`)

func tryOffsetDateTime(s string) (Token, int) {
	m := reOffsetDateTime.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	year, _ := strconv.Atoi(m[1])
	sep := m[2]
	month, _ := strconv.Atoi(m[3])
	day, _ := strconv.Atoi(m[4])
	tSep := m[5]
	hour, _ := strconv.Atoi(m[6])
	min, _ := strconv.Atoi(m[7])
	sec, _ := strconv.Atoi(m[8])
	fracStr := m[9]
	tzStr := m[10]

	// Verify both date separators match
	secondSep := m[0][len(m[1])+len(m[2])+len(m[3]) : len(m[1])+len(m[2])+len(m[3])+1]
	if sep != secondSep {
		return Token{}, 0
	}

	if !validateDate(year, month, day) || hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}

	if isFollowedByAlphanumeric(s, len(m[0])) {
		return Token{}, 0
	}

	format := "yyyy" + q(sep) + "MM" + q(sep) + "dd" + q(tSep) + "HH" + q(":") + "mm" + q(":") + "ss"
	if fracStr != "" {
		format += q(".") + fracFormat(fracStr)
	}
	format += tzFormat(tzStr)

	sigFmt := cleanDateFormat(format)
	return DateToken(sigFmt, m[0]), len(m[0])
}

func tryLocalDateTime(s string) (Token, int) {
	m := reLocalDateTime.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	year, _ := strconv.Atoi(m[1])
	sep := m[2]
	month, _ := strconv.Atoi(m[3])
	day, _ := strconv.Atoi(m[4])
	tSep := m[5]
	hour, _ := strconv.Atoi(m[6])
	min, _ := strconv.Atoi(m[7])
	sec, _ := strconv.Atoi(m[8])
	fracStr := m[9]

	secondSep := m[0][len(m[1])+len(m[2])+len(m[3]) : len(m[1])+len(m[2])+len(m[3])+1]
	if sep != secondSep {
		return Token{}, 0
	}

	if !validateDate(year, month, day) || hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}

	if isFollowedByAlphanumeric(s, len(m[0])) {
		return Token{}, 0
	}

	format := "yyyy" + q(sep) + "MM" + q(sep) + "dd" + q(tSep) + "HH" + q(":") + "mm" + q(":") + "ss"
	if fracStr != "" {
		format += q(".") + fracFormat(fracStr)
	}

	sigFmt := cleanDateFormat(format)
	return DateToken(sigFmt, m[0]), len(m[0])
}

func tryCLFDateTime(s string) (Token, int) {
	m := reCLFDateTime.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	day, _ := strconv.Atoi(m[1])
	monthStr := m[2]
	year, _ := strconv.Atoi(m[3])
	tSep := m[4]
	hour, _ := strconv.Atoi(m[5])
	min, _ := strconv.Atoi(m[6])
	sec, _ := strconv.Atoi(m[7])
	fracStr := m[8]
	tzStr := m[9]

	month := parseMonthAbbr(monthStr)
	if month == 0 || !validateDate(year, int(month), day) || hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}

	if isFollowedByAlphanumeric(s, len(m[0])) {
		return Token{}, 0
	}

	format := "dd" + q("/") + "MMM" + q("/") + "yyyy" + q(tSep) + "HH" + q(":") + "mm" + q(":") + "ss"
	if fracStr != "" {
		format += q(".") + fracFormat(fracStr)
	}

	sigFmt := cleanDateFormat(format)

	if tzStr != "" {
		sigFmt += " " + tzSigFormat(tzStr)
		return DateToken(sigFmt, m[0]), len(m[0])
	}

	return DateToken(sigFmt, m[0]), len(m[0])
}

func tryLocalDate(s string) (Token, int) {
	m := reLocalDate.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	year, _ := strconv.Atoi(m[1])
	sep := m[2]
	month, _ := strconv.Atoi(m[3])
	day, _ := strconv.Atoi(m[4])

	secondSep := m[0][len(m[1])+len(m[2])+len(m[3]) : len(m[1])+len(m[2])+len(m[3])+1]
	if sep != secondSep {
		return Token{}, 0
	}

	if !validateDate(year, month, day) {
		return Token{}, 0
	}

	if isFollowedByAlphanumeric(s, len(m[0])) {
		return Token{}, 0
	}

	format := "yyyy" + q(sep) + "MM" + q(sep) + "dd"
	sigFmt := cleanDateFormat(format)
	return DateToken(sigFmt, m[0]), len(m[0])
}

func tryCLFDate(s string) (Token, int) {
	m := reCLFDate.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	day, _ := strconv.Atoi(m[1])
	monthStr := m[2]
	year, _ := strconv.Atoi(m[3])

	month := parseMonthAbbr(monthStr)
	if month == 0 || !validateDate(year, int(month), day) {
		return Token{}, 0
	}

	if isFollowedByAlphanumeric(s, len(m[0])) {
		return Token{}, 0
	}

	sigFmt := "dd/MMM/yyyy"
	return DateToken(sigFmt, m[0]), len(m[0])
}

func tryLocalTime(s string) (Token, int) {
	m := reLocalTime.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	hour, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	fracStr := m[4]

	if hour > 23 || min > 59 || sec > 59 {
		return Token{}, 0
	}

	if isFollowedByAlphanumeric(s, len(m[0])) {
		return Token{}, 0
	}

	format := "HH:mm:ss"
	if fracStr != "" {
		format += "." + fracFormat(fracStr)
	}
	return LocalTimeToken(format, m[0]), len(m[0])
}

// ---- URI ----

var reURIScheme = regexp.MustCompile(`^(https?|ftp)://`)

func tryURI(s string) (Token, int) {
	sm := reURIScheme.FindString(s)
	if sm == "" {
		return Token{}, 0
	}
	scheme := sm[:len(sm)-3]
	rest := s[len(sm):]

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
	totalLen := len(sm) + authEnd

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

	tok := URIToken(scheme, &auth, pathTok, query, fragment)
	tok.Value = s[:totalLen]
	return tok, totalLen
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
	return AuthorityToken(&hostTok, portVal, hasPort, userInfo, hasUser)
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

var reEmail = regexp.MustCompile(`^([a-zA-Z0-9._%+()"\ -]+)\s*@\s*([a-zA-Z0-9.-]+\s*)`)

func tryEmail(s string) (Token, int) {
	m := reEmail.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}
	return EmailToken(m[1], m[2]), len(m[0])
}

// ---- IPv4 with optional port (standalone Authority) ----

var reIPv4WithPort = regexp.MustCompile(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?::(\d+))?`)

func tryIPv4Authority(s, fullMsg string, pos int) (Token, int) {
	m := reIPv4WithPort.FindStringSubmatch(s)
	if m == nil {
		return Token{}, 0
	}

	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	d, _ := strconv.Atoi(m[4])
	if a > 255 || b > 255 || c > 255 || d > 255 {
		return Token{}, 0
	}

	totalLen := len(m[0])

	// If followed by "/" it's part of a path, not a standalone IP
	if totalLen < len(s) && s[totalLen] == '/' {
		return Token{}, 0
	}

	// If followed by alphanumeric (but not port), it's part of a word
	if isFollowedByAlphanumeric(s, totalLen) {
		return Token{}, 0
	}

	ipStr := m[1] + "." + m[2] + "." + m[3] + "." + m[4]
	ipTok := Token{Type: TypeIPv4Address, Value: ipStr}

	portVal := 0
	hasPort := false
	if m[5] != "" {
		portVal, _ = strconv.Atoi(m[5])
		hasPort = true
	}

	auth := AuthorityToken(&ipTok, portVal, hasPort, "", false)
	auth.Value = m[0]
	return auth, totalLen
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

	tok := PathQueryFragmentToken(segments, query, fragment)
	tok.Value = s[:totalLen]
	return tok, totalLen
}

func looksLikeIPAfterSlash(s string) bool {
	m := reIPv4WithPort.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	d, _ := strconv.Atoi(m[4])
	if a > 255 || b > 255 || c > 255 || d > 255 {
		return false
	}
	// Only treat as IP if not followed by more path segments
	matchLen := len(m[0])
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
				return HttpStatusCodeToken(intPart), len(intPart)
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

var httpMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true,
	"PATCH": true, "HEAD": true, "OPTIONS": true, "CONNECT": true, "TRACE": true,
}

var severityKeywords = map[string]bool{
	"INFO": true, "WARN": true, "WARNING": true,
	"ERROR": true, "ERR": true,
	"DEBUG": true, "TRACE": true,
	"FATAL": true, "CRITICAL": true,
	"ALERT": true, "EMERGENCY": true, "EMERG": true,
	"NOTICE": true,
}

func tryWordOrKeyword(s string) (Token, int) {
	i := 0
	for i < len(s) && isWordChar(s[i]) {
		i++
	}

	text := s[:i]

	if httpMethods[text] {
		return HttpMethodToken(text), i
	}
	if severityKeywords[text] {
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
	months := map[string]time.Month{
		"Jan": time.January, "Feb": time.February, "Mar": time.March,
		"Apr": time.April, "May": time.May, "Jun": time.June,
		"Jul": time.July, "Aug": time.August, "Sep": time.September,
		"Oct": time.October, "Nov": time.November, "Dec": time.December,
	}
	return months[s]
}

func q(s string) string {
	return "'" + s + "'"
}

func fracFormat(frac string) string {
	switch len(frac) {
	case 1:
		return "S"
	case 2:
		return "SS"
	case 3:
		return "SSS"
	case 4:
		return "SSSS"
	case 5:
		return "SSSSS"
	case 6:
		return "SSSSSS"
	case 7:
		return "SSSSSSS"
	case 8:
		return "SSSSSSSS"
	case 9:
		return "SSSSSSSSS"
	default:
		return strings.Repeat("S", len(frac))
	}
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

func cleanDateFormat(format string) string {
	return strings.ReplaceAll(format, "'", "")
}

// ---- Character classification ----

func isDigit(b byte) bool         { return b >= '0' && b <= '9' }
func isAlpha(b byte) bool         { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
func isAlphaNum(b byte) bool      { return isAlpha(b) || isDigit(b) || b == '_' }
func isWhitespace(b byte) bool    { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
func isAlphaNumOrDot(b byte) bool { return isAlphaNum(b) || b == '.' }

func isWordChar(b byte) bool {
	return isAlpha(b) || isDigit(b) || b == '_' || b == '-'
}

func isWordCharNoDot(b byte) bool {
	return isAlpha(b) || isDigit(b) || b == '_' || b == '-'
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

var validHTTPStatusCodes = map[int]bool{
	100: true, 101: true,
	200: true, 201: true, 202: true, 203: true, 204: true, 206: true, 207: true,
	300: true, 301: true, 302: true, 303: true, 304: true, 307: true, 308: true,
	400: true, 401: true, 402: true, 403: true, 404: true, 405: true,
	406: true, 407: true, 408: true, 409: true, 410: true, 411: true,
	413: true, 414: true, 415: true, 416: true, 422: true, 425: true,
	426: true, 429: true, 431: true, 451: true,
	500: true, 501: true, 502: true, 503: true, 504: true, 505: true,
	507: true, 508: true, 510: true, 511: true,
}

func isValidHTTPStatus(code int) bool {
	return validHTTPStatusCodes[code]
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

func isFollowedByWordChar(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	return isWordChar(s[pos])
}

func hasOnlyDigits(s string) bool {
	for _, c := range s {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return len(s) > 0
}

const (
	SEVERITY_UNKNOWN = iota
	// Logs that should have ~ no impact on anomaly detection
	SEVERITY_DEBUG
	SEVERITY_INFO
	// Logs that should have the most impact on anomaly detection and RCA (warning+ logs)
	SEVERITY_ERROR
)

// GetSeverity returns the severity of a log
// It is based on the first severity token found
func GetSeverity(tokens []Token) int {
	for _, token := range tokens {
		// We cannot use severity token type because it's not always parsed for that
		switch strings.ToUpper(token.Value) {
		case "DEBUG", "D":
			return SEVERITY_DEBUG
		case "INFO", "I":
			return SEVERITY_INFO
		case "ERROR", "ERR", "E", "WARNING", "WARN", "W":
			return SEVERITY_ERROR
		}
	}
	return SEVERITY_UNKNOWN
}

func GetSeverityString(severity int) string {
	switch severity {
	case SEVERITY_DEBUG:
		return "DEBUG"
	case SEVERITY_INFO:
		return "INFO"
	case SEVERITY_ERROR:
		return "ERROR"
	case SEVERITY_UNKNOWN:
		return "UNKNOWN"
	default:
		fmt.Fprintf(os.Stderr, "warning: Invalid severity: %d\n", severity)

		return "UNKNOWN"
	}
}
