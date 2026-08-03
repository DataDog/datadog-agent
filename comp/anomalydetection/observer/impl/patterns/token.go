// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"fmt"
	"strings"
)

type TokenType int

const (
	TypeWord TokenType = iota
	TypeNumericValue
	TypeSpecialCharacter
	TypeWhitespace
	TypeDate
	TypeLocalTime
	TypeIPv4Address
	TypeIPv6Address
	TypeAbsolutePath
	TypePathQueryFragment
	TypeURI
	TypeAuthority
	TypeEmailAddress
	TypeHTTPMethod
	TypeHTTPStatusCode
	TypeSeverity
	TypeHexDump
	TypeKVSequence
)

func (t TokenType) String() string {
	switch t {
	case TypeWord:
		return "Word"
	case TypeNumericValue:
		return "NumericValue"
	case TypeSpecialCharacter:
		return "SpecialCharacter"
	case TypeWhitespace:
		return "Whitespace"
	case TypeDate:
		return "Date"
	case TypeLocalTime:
		return "LocalTime"
	case TypeIPv4Address:
		return "IPv4Address"
	case TypeIPv6Address:
		return "IPv6Address"
	case TypeAbsolutePath:
		return "AbsolutePath"
	case TypePathQueryFragment:
		return "PathWithQueryAndFragment"
	case TypeURI:
		return "URI"
	case TypeAuthority:
		return "Authority"
	case TypeEmailAddress:
		return "EmailAddress"
	case TypeHTTPMethod:
		return "HttpMethod"
	case TypeHTTPStatusCode:
		return "HttpStatusCode"
	case TypeSeverity:
		return "Severity"
	case TypeHexDump:
		return "HexDump"
	case TypeKVSequence:
		return "KeyValueSequence"
	default:
		return fmt.Sprintf("Unknown(%d)", int(t))
	}
}

// Token is the lean, public representation stored in cluster patterns.
// Simple token types (word, number, special character, whitespace, severity,
// HTTP method/status, IPv4/IPv6) carry no extra allocation.
// Complex types (date, path, URI, authority, email, hex-dump, KV) store
// their type-specific fields in the private tokenExtra pointer.
type Token struct {
	Type  TokenType
	Value string

	// TypeWord: used for signature ("specialWord" vs "word") and merge guard.
	HasDigits     bool
	NeverWildcard bool

	// Set by mergeTokenLists when two differing values collapse to a wildcard.
	IsWild bool

	// Non-nil only for complex token types; nil for word/number/special/whitespace/
	// severity/http-method/http-status/ipv4/ipv6.
	extra *tokenExtra
}

// tokenExtra holds all type-specific fields for complex token types.
// Keeping them in a separate heap-allocated struct lets the common simple
// tokens (TypeWord etc.) avoid carrying ~300 bytes of zero-valued fields.
type tokenExtra struct {
	// TypeDate, TypeLocalTime
	DateFormat string

	// TypeAbsolutePath, TypePathQueryFragment, TypeURI
	Segments []string
	Query    *string
	Fragment *string

	// TypeURI
	Scheme    string
	Authority *Token
	Path      *Token

	// TypeAuthority
	Host     *Token
	Port     int
	HasPort  bool
	UserInfo string
	HasUser  bool

	// TypeEmailAddress
	LocalPart string
	Domain    string

	// TypeHexDump
	DispLen  int
	HasASCII bool

	// TypeKVSequence
	KVKeys    []string
	KVSep     string
	KVPairSep string
	KVQuote   string
}

func WordToken(text string) Token {
	return Token{
		Type:      TypeWord,
		Value:     text,
		HasDigits: containsDigit(text),
	}
}

func NumericValueToken(text string) Token {
	return Token{Type: TypeNumericValue, Value: text}
}

// specialCharStrings interns single-byte strings so SpecialCharToken — called
// once per non-token byte during tokenization — does not allocate.
var specialCharStrings = func() [256]string {
	var arr [256]string
	for i := range arr {
		arr[i] = string([]byte{byte(i)})
	}
	return arr
}()

func SpecialCharToken(ch byte) Token {
	return Token{Type: TypeSpecialCharacter, Value: specialCharStrings[ch]}
}

// whitespacePool holds shared "space-only" strings for common whitespace runs.
// Avoids per-call allocations from strings.Repeat for typical small runs.
var whitespacePool = func() [33]string {
	var arr [33]string
	for i := range arr {
		arr[i] = strings.Repeat(" ", i)
	}
	return arr
}()

func WhitespaceToken(count int) Token {
	if count >= 0 && count < len(whitespacePool) {
		return Token{Type: TypeWhitespace, Value: whitespacePool[count]}
	}
	return Token{Type: TypeWhitespace, Value: strings.Repeat(" ", count)}
}

func DateToken(format, rawText string) Token {
	return Token{Type: TypeDate, Value: rawText, extra: &tokenExtra{DateFormat: format}}
}

func LocalTimeToken(format, rawText string) Token {
	return Token{Type: TypeLocalTime, Value: rawText, extra: &tokenExtra{DateFormat: format}}
}

func IPv4Token(text string, _, _, _, _ int) Token {
	return Token{Type: TypeIPv4Address, Value: text}
}

func PathToken(segments []string) Token {
	return pathTokenRaw("", segments)
}

func pathTokenRaw(value string, segments []string) Token {
	if value == "" {
		var b strings.Builder
		b.WriteByte('/')
		for i, seg := range segments {
			if i > 0 {
				b.WriteByte('/')
			}
			b.WriteString(seg)
		}
		value = b.String()
	}
	return Token{
		Type:  TypeAbsolutePath,
		Value: value,
		extra: &tokenExtra{Segments: segments},
	}
}

func PathQueryFragmentToken(segments []string, query, fragment *string) Token {
	return pathQueryFragmentTokenRaw("", segments, query, fragment)
}

// pathQueryFragmentTokenRaw is like PathQueryFragmentToken but takes a
// pre-built Value. The tokenizer always already has the matched substring at
// hand, so it can skip the strings.Join + concat the public constructor would
// do (the caller used to overwrite Value anyway).
func pathQueryFragmentTokenRaw(value string, segments []string, query, fragment *string) Token {
	if value == "" {
		var b strings.Builder
		b.WriteByte('/')
		for i, seg := range segments {
			if i > 0 {
				b.WriteByte('/')
			}
			b.WriteString(seg)
		}
		if query != nil {
			b.WriteByte('?')
			b.WriteString(*query)
		}
		if fragment != nil {
			b.WriteByte('#')
			b.WriteString(*fragment)
		}
		value = b.String()
	}
	return Token{
		Type:  TypePathQueryFragment,
		Value: value,
		extra: &tokenExtra{Segments: segments, Query: query, Fragment: fragment},
	}
}

func URIToken(scheme string, authority *Token, path *Token, query, fragment *string) Token {
	return uriTokenRaw("", scheme, authority, path, query, fragment)
}

func uriTokenRaw(value string, scheme string, authority *Token, path *Token, query, fragment *string) Token {
	if value == "" {
		var b strings.Builder
		b.WriteString(scheme)
		b.WriteString("://")
		if authority != nil {
			b.WriteString(authority.Value)
		}
		if path != nil {
			b.WriteString(path.Value)
		}
		if query != nil {
			b.WriteByte('?')
			b.WriteString(*query)
		}
		if fragment != nil {
			b.WriteByte('#')
			b.WriteString(*fragment)
		}
		value = b.String()
	}
	return Token{
		Type:  TypeURI,
		Value: value,
		extra: &tokenExtra{Scheme: scheme, Authority: authority, Path: path, Query: query, Fragment: fragment},
	}
}

func AuthorityToken(host *Token, port int, hasPort bool, userInfo string, hasUser bool) Token {
	return authorityTokenRaw("", host, port, hasPort, userInfo, hasUser)
}

func authorityTokenRaw(value string, host *Token, port int, hasPort bool, userInfo string, hasUser bool) Token {
	if value == "" {
		var b strings.Builder
		if hasUser {
			b.WriteString(userInfo)
			b.WriteByte('@')
		}
		if host != nil {
			b.WriteString(host.Value)
		}
		if hasPort {
			fmt.Fprintf(&b, ":%d", port)
		}
		value = b.String()
	}
	return Token{
		Type:  TypeAuthority,
		Value: value,
		extra: &tokenExtra{Host: host, Port: port, HasPort: hasPort, UserInfo: userInfo, HasUser: hasUser},
	}
}

func EmailToken(localPart, domain string) Token {
	return emailTokenRaw("", localPart, domain)
}

func emailTokenRaw(value, localPart, domain string) Token {
	if value == "" {
		value = localPart + "@" + domain
	}
	return Token{
		Type:  TypeEmailAddress,
		Value: value,
		extra: &tokenExtra{LocalPart: localPart, Domain: domain},
	}
}

func HTTPMethodToken(method string) Token {
	return Token{Type: TypeHTTPMethod, Value: method}
}

func HTTPStatusCodeToken(code string) Token {
	return Token{Type: TypeHTTPStatusCode, Value: code}
}

func SeverityToken(level string) Token {
	return Token{Type: TypeSeverity, Value: level}
}

func HexDumpToken(text string, dispLen int, hasASCII bool) Token {
	return Token{
		Type:  TypeHexDump,
		Value: text,
		extra: &tokenExtra{DispLen: dispLen, HasASCII: hasASCII},
	}
}

func KVSequenceToken(keys []string, sep, pairSep, quote string) Token {
	return Token{
		Type:  TypeKVSequence,
		extra: &tokenExtra{KVKeys: keys, KVSep: sep, KVPairSep: pairSep, KVQuote: quote},
	}
}

// Signature returns the signature string for this token.
func (t Token) Signature() string {
	switch t.Type {
	case TypeWord:
		return wordSignature(t)
	case TypeNumericValue:
		return "numericValue"
	case TypeSpecialCharacter:
		return t.Value
	case TypeWhitespace:
		return " "
	case TypeDate:
		return t.extra.DateFormat
	case TypeLocalTime:
		return "local-time:" + t.extra.DateFormat
	case TypeIPv4Address:
		return "ipv4"
	case TypeIPv6Address:
		return "ipv6"
	case TypeAbsolutePath:
		return "AbsolutePath"
	case TypePathQueryFragment:
		return pathQueryFragSignature(t)
	case TypeURI:
		return uriSignature(t)
	case TypeAuthority:
		return authoritySignature(t)
	case TypeEmailAddress:
		return "localPart@domain"
	case TypeHTTPMethod:
		return "HttpMethod{value=*}"
	case TypeHTTPStatusCode:
		return "HttpStatusCode{value=*}"
	case TypeSeverity:
		return "Severity{value=*}"
	case TypeHexDump:
		return hexDumpSignature(t)
	case TypeKVSequence:
		return kvSignature(t)
	default:
		return t.Value
	}
}

func wordSignature(t Token) string {
	if t.HasDigits {
		return "specialWord"
	}
	return "word"
}

func pathQueryFragSignature(t Token) string {
	sig := "AbsolutePath"
	if t.extra.Query != nil {
		sig += "?" + *t.extra.Query
	}
	if t.extra.Fragment != nil {
		sig += "#" + *t.extra.Fragment
	}
	return sig
}

func uriSignature(t Token) string {
	sig := t.extra.Scheme + "://"
	if t.extra.Authority != nil {
		sig += t.extra.Authority.Signature()
	}
	if t.extra.Path != nil {
		sig += t.extra.Path.Signature()
	}
	if t.extra.Query != nil {
		sig += "?" + *t.extra.Query
	}
	if t.extra.Fragment != nil {
		sig += "#" + *t.extra.Fragment
	}
	return sig
}

func authoritySignature(t Token) string {
	sig := ""
	if t.extra.HasUser {
		sig += t.extra.UserInfo + "@"
	}
	if t.extra.Host != nil {
		sig += hostSignature(*t.extra.Host)
	}
	if t.extra.HasPort {
		sig += ":port"
	}
	return sig
}

func hostSignature(t Token) string {
	switch t.Type {
	case TypeIPv4Address:
		return "ipv4"
	case TypeIPv6Address:
		return "ipv6"
	default:
		return "regularHostName"
	}
}

func hexDumpSignature(t Token) string {
	ascii := "F"
	if t.extra.HasASCII {
		ascii = "T"
	}
	return fmt.Sprintf("HexDump[dl:%d|ascii:%s]", t.extra.DispLen, ascii)
}

func kvSignature(t Token) string {
	sorted := make([]string, len(t.extra.KVKeys))
	copy(sorted, t.extra.KVKeys)
	sortStrings(sorted)
	unique := uniqueStrings(sorted)
	return fmt.Sprintf("KV%d=[%s][q:%s|s:%s%s|ps:%s]KV",
		len(unique),
		strings.Join(unique, ", "),
		t.extra.KVQuote,
		t.extra.KVSep,
		t.extra.KVSep,
		t.extra.KVPairSep,
	)
}

// PatternString returns the display pattern for this token.
func (t Token) PatternString() string {
	if t.IsWild {
		return "*"
	}
	return t.Value
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func uniqueStrings(sorted []string) []string {
	if len(sorted) == 0 {
		return sorted
	}
	result := []string{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[len(result)-1] {
			result = append(result, sorted[i])
		}
	}
	return result
}
