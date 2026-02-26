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
	TypeHttpMethod
	TypeHttpStatusCode
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
	case TypeHttpMethod:
		return "HttpMethod"
	case TypeHttpStatusCode:
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

type Token struct {
	Type  TokenType
	Value string

	// Word-specific
	HasDigits     bool
	NeverWildcard bool

	// Date-specific
	DateFormat string

	// Path-specific
	Segments []string
	Query    *string
	Fragment *string

	// URI-specific
	Scheme    string
	Authority *Token
	Path      *Token

	// Authority-specific
	Host     *Token
	Port     int
	HasPort  bool
	UserInfo string
	HasUser  bool

	// Email-specific
	LocalPart string
	Domain    string

	// HexDump-specific
	DispLen  int
	HasAscii bool

	// KV-specific
	KVKeys    []string
	KVSep     string
	KVPairSep string
	KVQuote   string

	// Merging
	IsWild bool
	Values []string
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

func SpecialCharToken(ch byte) Token {
	return Token{Type: TypeSpecialCharacter, Value: string(ch)}
}

func WhitespaceToken(count int) Token {
	return Token{Type: TypeWhitespace, Value: strings.Repeat(" ", count)}
}

func DateToken(format, rawText string) Token {
	return Token{Type: TypeDate, Value: rawText, DateFormat: format}
}

func LocalTimeToken(format, rawText string) Token {
	return Token{Type: TypeLocalTime, Value: rawText, DateFormat: format}
}

func IPv4Token(text string, a, b, c, d int) Token {
	return Token{Type: TypeIPv4Address, Value: text}
}

func PathToken(segments []string) Token {
	return Token{
		Type:     TypeAbsolutePath,
		Value:    "/" + strings.Join(segments, "/"),
		Segments: segments,
	}
}

func PathQueryFragmentToken(segments []string, query, fragment *string) Token {
	v := "/" + strings.Join(segments, "/")
	if query != nil {
		v += "?" + *query
	}
	if fragment != nil {
		v += "#" + *fragment
	}
	return Token{
		Type:     TypePathQueryFragment,
		Value:    v,
		Segments: segments,
		Query:    query,
		Fragment: fragment,
	}
}

func URIToken(scheme string, authority *Token, path *Token, query, fragment *string) Token {
	v := scheme + "://"
	if authority != nil {
		v += authority.Value
	}
	if path != nil {
		v += path.Value
	}
	if query != nil {
		v += "?" + *query
	}
	if fragment != nil {
		v += "#" + *fragment
	}
	return Token{
		Type:      TypeURI,
		Value:     v,
		Scheme:    scheme,
		Authority: authority,
		Path:      path,
		Query:     query,
		Fragment:  fragment,
	}
}

func AuthorityToken(host *Token, port int, hasPort bool, userInfo string, hasUser bool) Token {
	v := ""
	if hasUser {
		v += userInfo + "@"
	}
	if host != nil {
		v += host.Value
	}
	if hasPort {
		v += fmt.Sprintf(":%d", port)
	}
	return Token{
		Type:     TypeAuthority,
		Value:    v,
		Host:     host,
		Port:     port,
		HasPort:  hasPort,
		UserInfo: userInfo,
		HasUser:  hasUser,
	}
}

func EmailToken(localPart, domain string) Token {
	return Token{
		Type:      TypeEmailAddress,
		Value:     localPart + "@" + domain,
		LocalPart: localPart,
		Domain:    domain,
	}
}

func HttpMethodToken(method string) Token {
	return Token{Type: TypeHttpMethod, Value: method}
}

func HttpStatusCodeToken(code string) Token {
	return Token{Type: TypeHttpStatusCode, Value: code}
}

func SeverityToken(level string) Token {
	return Token{Type: TypeSeverity, Value: level}
}

func HexDumpToken(text string, dispLen int, hasAscii bool) Token {
	return Token{
		Type:     TypeHexDump,
		Value:    text,
		DispLen:  dispLen,
		HasAscii: hasAscii,
	}
}

func KVSequenceToken(keys []string, sep, pairSep, quote string) Token {
	return Token{
		Type:      TypeKVSequence,
		Value:     "",
		KVKeys:    keys,
		KVSep:     sep,
		KVPairSep: pairSep,
		KVQuote:   quote,
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
		return t.DateFormat
	case TypeLocalTime:
		return "local-time:" + t.DateFormat
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
	case TypeHttpMethod:
		return "HttpMethod{value=*}"
	case TypeHttpStatusCode:
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
	if t.Query != nil {
		sig += "?" + *t.Query
	}
	if t.Fragment != nil {
		sig += "#" + *t.Fragment
	}
	return sig
}

func uriSignature(t Token) string {
	sig := t.Scheme + "://"
	if t.Authority != nil {
		sig += t.Authority.Signature()
	}
	if t.Path != nil {
		sig += t.Path.Signature()
	}
	if t.Query != nil {
		sig += "?" + *t.Query
	}
	if t.Fragment != nil {
		sig += "#" + *t.Fragment
	}
	return sig
}

func authoritySignature(t Token) string {
	sig := ""
	if t.HasUser {
		sig += t.UserInfo + "@"
	}
	if t.Host != nil {
		sig += hostSignature(*t.Host)
	}
	if t.HasPort {
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
	if t.HasAscii {
		ascii = "T"
	}
	return fmt.Sprintf("HexDump[dl:%d|ascii:%s]", t.DispLen, ascii)
}

func kvSignature(t Token) string {
	sorted := make([]string, len(t.KVKeys))
	copy(sorted, t.KVKeys)
	sortStrings(sorted)
	unique := uniqueStrings(sorted)
	return fmt.Sprintf("KV%d=[%s][q:%s|s:%s%s|ps:%s]KV",
		len(unique),
		strings.Join(unique, ", "),
		t.KVQuote,
		t.KVSep,
		t.KVSep,
		t.KVPairSep,
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
