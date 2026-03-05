// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func tok(tokens []Token) []TokenType {
	types := make([]TokenType, len(tokens))
	for i, t := range tokens {
		types[i] = t.Type
	}
	return types
}

func assertTokenTypes(t *testing.T, msg string, expected []TokenType) {
	t.Helper()
	tokens := NewTokenizer().Tokenize(msg)
	actual := tok(tokens)
	if len(actual) != len(expected) {
		t.Errorf("Tokenize(%q): got %d tokens, want %d\n  got:  %v\n  want: %v",
			msg, len(actual), len(expected), actual, expected)
		return
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Errorf("Tokenize(%q): token[%d] type = %v, want %v\n  full: %v",
				msg, i, actual[i], expected[i], actual)
			return
		}
	}
}

func assertTokenValues(t *testing.T, msg string, expectedTypes []TokenType, expectedValues []string) {
	t.Helper()
	tokens := NewTokenizer().Tokenize(msg)
	if len(tokens) != len(expectedTypes) {
		t.Errorf("Tokenize(%q): got %d tokens, want %d", msg, len(tokens), len(expectedTypes))
		for i, tok := range tokens {
			t.Logf("  token[%d]: type=%v value=%q", i, tok.Type, tok.Value)
		}
		return
	}
	for i := range expectedTypes {
		if tokens[i].Type != expectedTypes[i] {
			t.Errorf("Tokenize(%q): token[%d] type = %v, want %v (value=%q)",
				msg, i, tokens[i].Type, expectedTypes[i], tokens[i].Value)
		}
		if expectedValues != nil && i < len(expectedValues) && expectedValues[i] != "" {
			if tokens[i].Value != expectedValues[i] {
				t.Errorf("Tokenize(%q): token[%d] value = %q, want %q",
					msg, i, tokens[i].Value, expectedValues[i])
			}
		}
	}
}

func dumpTokens(t *testing.T, msg string) {
	t.Helper()
	tokens := NewTokenizer().Tokenize(msg)
	t.Logf("Tokenize(%q) = %d tokens:", msg, len(tokens))
	for i, tok := range tokens {
		t.Logf("  [%d] %v %q", i, tok.Type, tok.Value)
	}
}

// --- Empty ---

func TestTokenizeEmpty(t *testing.T) {
	tokens := NewTokenizer().Tokenize("")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty string, got %d", len(tokens))
	}
}

// --- Absolute Path ---

func TestTokenizeAbsolutePath(t *testing.T) {
	assertTokenTypes(t, "/a", []TokenType{TypePathQueryFragment})
	assertTokenTypes(t, "/abc/def/ghi", []TokenType{TypePathQueryFragment})
	assertTokenTypes(t, "/", []TokenType{TypeSpecialCharacter})
	assertTokenTypes(t, "//abc/def", []TokenType{
		TypeSpecialCharacter, TypePathQueryFragment,
	})
	assertTokenTypes(t, "?", []TokenType{TypeSpecialCharacter})
}

func TestTokenizePathWithQuery(t *testing.T) {
	tokens := NewTokenizer().Tokenize("/reports/search_by_client?")
	if len(tokens) != 1 || tokens[0].Type != TypePathQueryFragment {
		t.Errorf("expected PathQueryFragment, got %v", tok(tokens))
	}
	if tokens[0].Query == nil {
		t.Error("expected non-nil query")
	}

	tokens = NewTokenizer().Tokenize("/reports/search_by_client?query")
	if tokens[0].Query == nil || *tokens[0].Query != "query" {
		t.Errorf("expected query='query', got %v", tokens[0].Query)
	}

	tokens = NewTokenizer().Tokenize("/reports/search_by_client#fragment")
	if tokens[0].Fragment == nil || *tokens[0].Fragment != "fragment" {
		t.Errorf("expected fragment='fragment', got %v", tokens[0].Fragment)
	}

	tokens = NewTokenizer().Tokenize("/reports/search_by_client?query#fragment")
	if tokens[0].Query == nil || *tokens[0].Query != "query" {
		t.Error("expected query='query'")
	}
	if tokens[0].Fragment == nil || *tokens[0].Fragment != "fragment" {
		t.Error("expected fragment='fragment'")
	}
}

func TestTokenizePathQuerySpace(t *testing.T) {
	tokens := NewTokenizer().Tokenize("/api/v1? /api/v1?param1=value1")
	if len(tokens) != 3 {
		dumpTokens(t, "/api/v1? /api/v1?param1=value1")
		t.Fatalf("expected 3 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TypePathQueryFragment {
		t.Errorf("token[0] expected PathQueryFragment, got %v", tokens[0].Type)
	}
	if tokens[1].Type != TypeWhitespace {
		t.Errorf("token[1] expected Whitespace, got %v", tokens[1].Type)
	}
	if tokens[2].Type != TypePathQueryFragment {
		t.Errorf("token[2] expected PathQueryFragment, got %v", tokens[2].Type)
	}
	if tokens[2].Query == nil || *tokens[2].Query != "param1=value1" {
		t.Errorf("token[2] expected query 'param1=value1'")
	}
}

// --- Date ---

func TestTokenizeDateISO(t *testing.T) {
	assertTokenTypes(t, "2018-03-04", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T13:34:29", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T13:34:29+00:00", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T13:34:29Z", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-10-29T13:39:52.93+0000", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T13:34:29.776", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T13:34:29.776Z", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-10-29T13:36:39.223+0000", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-01-01 12:23:34", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-10-23 14:08:26.829374", []TokenType{TypeDate})
	assertTokenTypes(t, "2018/01/01 12:23:34", []TokenType{TypeDate})
	assertTokenTypes(t, "2018/01/01", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T02:01:00.124543", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T02:01:00.124543-05:30", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T02:01:00.124543Z", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T02:01:00.124543765", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T02:01:00.124543765+13:00", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-03-04T02:01:00.124543765Z", []TokenType{TypeDate})
	assertTokenTypes(t, "2018-02-19", []TokenType{TypeDate})
}

func TestTokenizeDateCLF(t *testing.T) {
	assertTokenTypes(t, "09/Jun/2018", []TokenType{TypeDate})
	assertTokenTypes(t, "09/Jun/2018:06:26:25", []TokenType{TypeDate})
	assertTokenTypes(t, "09/Jun/2018:06:26:25.276", []TokenType{TypeDate})
}

func TestTokenizeTime(t *testing.T) {
	assertTokenTypes(t, "12:56:45", []TokenType{TypeLocalTime})
	assertTokenTypes(t, "12:56:45.878", []TokenType{TypeLocalTime})
}

func TestTokenizeInvalidDateTimes(t *testing.T) {
	// 49:40:37 -> invalid time (hour > 23)
	tokens := NewTokenizer().Tokenize("49:40:37")
	for _, tok := range tokens {
		if tok.Type == TypeLocalTime || tok.Type == TypeDate {
			t.Errorf("49:40:37 should not be a date/time, got %v", tok.Type)
		}
	}

	// Invalid month (00)
	tokens = NewTokenizer().Tokenize("2018-00-01")
	if len(tokens) == 1 && tokens[0].Type == TypeDate {
		t.Error("2018-00-01 should not be a valid date")
	}

	// Invalid day (00)
	tokens = NewTokenizer().Tokenize("2018-01-00")
	if len(tokens) == 1 && tokens[0].Type == TypeDate {
		t.Error("2018-01-00 should not be a valid date")
	}

	// April 31st doesn't exist
	tokens = NewTokenizer().Tokenize("2018-04-31")
	if len(tokens) == 1 && tokens[0].Type == TypeDate {
		t.Error("2018-04-31 should not be a valid date (April has 30 days)")
	}

	// 2000 is a leap year
	assertTokenTypes(t, "2000-02-29", []TokenType{TypeDate})

	// 2100 is not a leap year
	tokens = NewTokenizer().Tokenize("2100-02-29")
	if len(tokens) == 1 && tokens[0].Type == TypeDate {
		t.Error("2100-02-29 should not be a valid date (2100 is not a leap year)")
	}
}

// --- Email ---

func TestTokenizeEmail(t *testing.T) {
	assertTokenTypes(t, "a@b", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "simple@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "user.name+team@domain.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "very.common@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "disposable.style.email.with+symbol@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "other.email-with-hyphen@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "fully-qualified-domain@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "user.name+tag+sorting@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "x@example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "example-indeed@strange-example.com", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "admin@mailserver1", []TokenType{TypeEmailAddress})
	assertTokenTypes(t, "example@s.example", []TokenType{TypeEmailAddress})
}

// --- HTTP Method ---

func TestTokenizeHTTPMethod(t *testing.T) {
	assertTokenTypes(t, "GET", []TokenType{TypeHttpMethod})
	assertTokenTypes(t, "POST", []TokenType{TypeHttpMethod})
	assertTokenTypes(t, "PUT", []TokenType{TypeHttpMethod})
	assertTokenTypes(t, "DELETE", []TokenType{TypeHttpMethod})
	assertTokenTypes(t, "PATCH", []TokenType{TypeHttpMethod})
	assertTokenTypes(t, "HEAD", []TokenType{TypeHttpMethod})
	assertTokenTypes(t, "OPTIONS", []TokenType{TypeHttpMethod})

	// DELETED is NOT an HTTP method
	tokens := NewTokenizer().Tokenize("DELETED")
	if tokens[0].Type == TypeHttpMethod {
		t.Error("DELETED should not be an HTTP method")
	}

	// "GET /url" -> HttpMethod + Whitespace + Path
	assertTokenTypes(t, "\"GET /url\"", []TokenType{
		TypeSpecialCharacter, TypeHttpMethod, TypeWhitespace, TypePathQueryFragment, TypeSpecialCharacter,
	})
}

// --- HTTP Status Code ---

func TestTokenizeHTTPStatusCode(t *testing.T) {
	assertTokenTypes(t, "200", []TokenType{TypeHttpStatusCode})
	assertTokenTypes(t, "404", []TokenType{TypeHttpStatusCode})
	assertTokenTypes(t, "500", []TokenType{TypeHttpStatusCode})
	assertTokenTypes(t, "201", []TokenType{TypeHttpStatusCode})
	assertTokenTypes(t, "302", []TokenType{TypeHttpStatusCode})

	// 200OK -> HttpStatusCode + Word
	tokens := NewTokenizer().Tokenize("200OK")
	if len(tokens) != 2 || tokens[0].Type != TypeHttpStatusCode || tokens[1].Type != TypeWord {
		t.Errorf("expected HttpStatusCode + Word for '200OK', got %v", tok(tokens))
	}

	// 234 is NOT a valid HTTP status code -> NumericValue
	tokens = NewTokenizer().Tokenize("234")
	if tokens[0].Type == TypeHttpStatusCode {
		t.Error("234 should be NumericValue, not HttpStatusCode")
	}
}

// --- Numeric Value ---

func TestTokenizeNumericValue(t *testing.T) {
	assertTokenTypes(t, "0001", []TokenType{TypeNumericValue})
	assertTokenTypes(t, "1.123", []TokenType{TypeNumericValue})
	assertTokenTypes(t, "-1.123", []TokenType{TypeNumericValue})

	// 1. -> NumericValue (trailing dot)
	tokens := NewTokenizer().Tokenize("1.")
	if len(tokens) != 1 || tokens[0].Type != TypeNumericValue {
		t.Errorf("expected NumericValue for '1.', got %v", tok(tokens))
	}
}

// --- Severity ---

func TestTokenizeSeverity(t *testing.T) {
	assertTokenTypes(t, "INFO", []TokenType{TypeSeverity})
	assertTokenTypes(t, "WARN", []TokenType{TypeSeverity})
	assertTokenTypes(t, "ERROR", []TokenType{TypeSeverity})
	assertTokenTypes(t, "DEBUG", []TokenType{TypeSeverity})
	assertTokenTypes(t, "FATAL", []TokenType{TypeSeverity})

	// [INFO] -> [ + INFO + ]
	assertTokenTypes(t, "[INFO]", []TokenType{
		TypeSpecialCharacter, TypeSeverity, TypeSpecialCharacter,
	})

	// XX-INFO123 is a word (INFO is embedded in a longer word)
	tokens := NewTokenizer().Tokenize("XX-INFO123")
	if tokens[0].Type == TypeSeverity {
		t.Error("XX-INFO123 should be a word, not severity")
	}
}

// --- Word ---

func TestTokenizeWord(t *testing.T) {
	assertTokenTypes(t, "abc", []TokenType{TypeWord})
	assertTokenTypes(t, "ZcHxgPh0RuCNyfc3kRUEfQ", []TokenType{TypeWord})
	assertTokenTypes(t, "zDl1t05WQeawZiOAXgf_Hw", []TokenType{TypeWord})
	assertTokenTypes(t, "UuWYblc-HMEEmTbaxsPWvA", []TokenType{TypeWord})

	// Verify words with digits are flagged
	tokens := NewTokenizer().Tokenize("abc22")
	if !tokens[0].HasDigits {
		t.Error("'abc22' should have HasDigits=true")
	}

	tokens = NewTokenizer().Tokenize("abc")
	if tokens[0].HasDigits {
		t.Error("'abc' should have HasDigits=false")
	}
}

// --- Whitespace ---

func TestTokenizeWhitespace(t *testing.T) {
	tokens := NewTokenizer().Tokenize("  ;")
	if len(tokens) != 2 || tokens[0].Type != TypeWhitespace || tokens[1].Type != TypeSpecialCharacter {
		t.Errorf("expected [Whitespace, SpecialChar], got %v", tok(tokens))
	}

	// Trailing whitespace is trimmed
	tokens = NewTokenizer().Tokenize(";   ")
	if len(tokens) != 1 || tokens[0].Type != TypeSpecialCharacter {
		t.Errorf("trailing whitespace should be trimmed, got %v", tok(tokens))
	}
}

// --- URI ---

func TestTokenizeURI(t *testing.T) {
	assertTokenTypes(t, "https://app.datadoghq.com", []TokenType{TypeURI})

	tokens := NewTokenizer().Tokenize("http://localhost:8083/v2/applications/tracestatsaggregator/pipelines")
	if len(tokens) != 1 || tokens[0].Type != TypeURI {
		t.Errorf("expected single URI token, got %v", tok(tokens))
	}

	assertTokenTypes(t, "https://192.168.1.1?foo", []TokenType{TypeURI})
	assertTokenTypes(t, "https://192.168.1.1:1234#bar", []TokenType{TypeURI})
	assertTokenTypes(t, "https://user@192.168.1.1:1234?foo#bar", []TokenType{TypeURI})

	// URI with path + fragment
	assertTokenTypes(t, "http://localhost:8083/v2/applications/tracestatsaggregator/pipelines#bar",
		[]TokenType{TypeURI})

	// URI with path + query
	assertTokenTypes(t, "http://localhost:8083/v2/applications?limit=30",
		[]TokenType{TypeURI})

	// URI with empty query
	assertTokenTypes(t, "http://localhost:8083/v2/applications?",
		[]TokenType{TypeURI})
}

func TestTokenizeURIEdgeCases(t *testing.T) {
	// "https://api.gopro.com:" -> URI + SpecialCharacter(':')
	tokens := NewTokenizer().Tokenize("https://api.gopro.com:")
	if len(tokens) < 2 {
		dumpTokens(t, "https://api.gopro.com:")
	}
}

// --- IPv4 ---

func TestTokenizeIPv4(t *testing.T) {
	tokens := NewTokenizer().Tokenize("123.234.123.234")
	if len(tokens) != 1 || tokens[0].Type != TypeAuthority {
		t.Errorf("expected Authority for standalone IP, got %v", tok(tokens))
	}

	// IP in URI
	tokens = NewTokenizer().Tokenize("http://123.234.123.234/abc")
	if len(tokens) != 1 || tokens[0].Type != TypeURI {
		t.Errorf("expected URI for http://ip/path, got %v", tok(tokens))
	}

	// /IP -> SpecialChar + Authority
	tokens = NewTokenizer().Tokenize("/123.234.123.234")
	if len(tokens) < 2 {
		dumpTokens(t, "/123.234.123.234")
		t.Fatal("expected at least 2 tokens")
	}

	// IP with port
	tokens = NewTokenizer().Tokenize("172.20.162.38:40032")
	if len(tokens) != 1 || tokens[0].Type != TypeAuthority {
		t.Errorf("expected Authority for IP:port, got %v", tok(tokens))
	}
	if !tokens[0].HasPort || tokens[0].Port != 40032 {
		t.Errorf("expected port 40032, got port=%d hasPort=%v", tokens[0].Port, tokens[0].HasPort)
	}

	// @IP
	tokens = NewTokenizer().Tokenize("@123.234.123.234")
	if len(tokens) < 2 || tokens[1].Type != TypeAuthority {
		dumpTokens(t, "@123.234.123.234")
		t.Error("expected SpecialChar(@) + Authority")
	}
}

func TestTokenizeIPv4InPath(t *testing.T) {
	tokens := NewTokenizer().Tokenize("/path/123.234.123.234/with/ip")
	if len(tokens) != 1 || tokens[0].Type != TypePathQueryFragment {
		t.Errorf("IP inside path should be part of path, got %v", tok(tokens))
	}
}

func TestTokenizeIPv4FromSlash(t *testing.T) {
	tokens := NewTokenizer().Tokenize("connection from /172.20.162.38:40032 is closed")
	found := false
	for _, tok := range tokens {
		if tok.Type == TypeAuthority && tok.HasPort {
			found = true
			break
		}
	}
	if !found {
		dumpTokens(t, "connection from /172.20.162.38:40032 is closed")
		t.Error("expected to find Authority with port in the token list")
	}
}

// --- HexDump ---

func TestTokenizeHexDump(t *testing.T) {
	tokenizer := NewTokenizer()
	tokenizer.ParseHexDump = true

	tokens := tokenizer.Tokenize("0010: DB 8A")
	if len(tokens) < 1 || tokens[0].Type != TypeHexDump {
		dumpTokens(t, "0010: DB 8A")
		t.Fatal("expected HexDump token")
	}
	if tokens[0].DispLen != 4 {
		t.Errorf("expected DispLen=4, got %d", tokens[0].DispLen)
	}
	if tokens[0].HasAscii {
		t.Error("expected HasAscii=false")
	}

	tokens = tokenizer.Tokenize("00000000: 02 f3 82 00")
	if tokens[0].Type != TypeHexDump || tokens[0].DispLen != 8 {
		t.Errorf("expected HexDump with DispLen=8")
	}

	tokens = tokenizer.Tokenize("0000: AA AA AA")
	if tokens[0].Type != TypeHexDump || tokens[0].DispLen != 4 {
		t.Errorf("expected HexDump with DispLen=4")
	}

	tokens = tokenizer.Tokenize("1C40: 82 0B A2")
	if tokens[0].Type != TypeHexDump || tokens[0].DispLen != 4 {
		t.Errorf("expected HexDump '1C40: 82 0B A2' with DispLen=4, got type=%v dl=%d", tokens[0].Type, tokens[0].DispLen)
	}
}

func TestTokenizeHexDumpWithAscii(t *testing.T) {
	tokenizer := NewTokenizer()
	tokenizer.ParseHexDump = true

	tokens := tokenizer.Tokenize("00000000: 01 02 03 04 05 Ascii")
	if len(tokens) < 1 || tokens[0].Type != TypeHexDump {
		dumpTokens(t, "00000000: 01 02 03 04 05 Ascii")
		t.Fatal("expected HexDump token")
	}
	if !tokens[0].HasAscii {
		t.Error("expected HasAscii=true")
	}

	tokens = tokenizer.Tokenize("0010: DB 8A 8E 01 00 .....")
	if tokens[0].Type != TypeHexDump {
		dumpTokens(t, "0010: DB 8A 8E 01 00 .....")
		t.Fatal("expected HexDump")
	}
	if !tokens[0].HasAscii {
		t.Error("expected HasAscii=true for '.....''")
	}
}

func TestTokenizeHexDumpDisabled(t *testing.T) {
	tokenizer := NewTokenizer()
	tokenizer.ParseHexDump = false

	tokens := tokenizer.Tokenize("0010: DB 8A")
	for _, tok := range tokens {
		if tok.Type == TypeHexDump {
			t.Error("HexDump should not be detected when disabled")
		}
	}
}

// --- Whole messages ---

func TestTokenizeWholeMessageKV(t *testing.T) {
	msg := `requestId={requestId:ZcHxgPh0RuCNyfc3kRUEfQ, nodeId:UuWYblc-HMEEmTbaxsPWvA} sourceType=null`
	tokens := NewTokenizer().Tokenize(msg)
	if len(tokens) == 0 {
		t.Fatal("expected non-empty token list")
	}
	if tokens[0].Type != TypeWord || tokens[0].Value != "requestId" {
		t.Errorf("first token should be Word('requestId'), got %v(%q)", tokens[0].Type, tokens[0].Value)
	}
}

func TestTokenizeWholeMessageWithDate(t *testing.T) {
	msg := "2018-06-01 16:12:59 INFO (monitor.go:119) - memory:      42kb"
	tokens := NewTokenizer().Tokenize(msg)
	if tokens[0].Type != TypeDate {
		t.Errorf("first token should be Date, got %v", tokens[0].Type)
	}
	// Find INFO severity
	foundSeverity := false
	for _, tok := range tokens {
		if tok.Type == TypeSeverity && tok.Value == "INFO" {
			foundSeverity = true
		}
	}
	if !foundSeverity {
		t.Error("expected to find Severity(INFO)")
	}
}

func TestTokenizeWholeMessageJSON(t *testing.T) {
	msg := `"http":{"status_code":"200","method":"GET"}`
	tokens := NewTokenizer().Tokenize(msg)
	// Should contain HttpStatusCode(200) and HttpMethod(GET)
	foundStatus := false
	foundMethod := false
	for _, tok := range tokens {
		if tok.Type == TypeHttpStatusCode && tok.Value == "200" {
			foundStatus = true
		}
		if tok.Type == TypeHttpMethod && tok.Value == "GET" {
			foundMethod = true
		}
	}
	if !foundStatus {
		t.Error("expected HttpStatusCode(200) in JSON message")
	}
	if !foundMethod {
		t.Error("expected HttpMethod(GET) in JSON message")
	}
}

func TestTokenizeIdsFirstWord(t *testing.T) {
	// From PatternClustererTest: it_should_merge_ids_even_when_they_are_the_first_word
	tokens1 := NewTokenizer().Tokenize("FATAL|2019-07-04T16:27:52,437|55767CC230D5|1.0|")
	tokens2 := NewTokenizer().Tokenize("FATAL|2019-07-04T16:27:54,880|E41F49A44911|1.0|")

	if len(tokens1) != len(tokens2) {
		t.Errorf("token count mismatch: %d vs %d", len(tokens1), len(tokens2))
	}
}

func TestTokenizeGCStats(t *testing.T) {
	msg := "8387 @95749.612s 0%: 0.021+89+0.12 ms clock, 0.34+2.6/356/810+1.9 ms cpu, 550->562->461 MB, 616 MB goal, 16 P"
	tokens := NewTokenizer().Tokenize(msg)
	if len(tokens) == 0 {
		t.Fatal("expected non-empty token list for GC stats")
	}
}

func TestGetSeverity(t *testing.T) {
	tok := NewTokenizer()
	assert.Equal(t, SEVERITY_INFO, GetSeverity(tok.Tokenize("INFO: 10.244.1.XXX:XXXX - * /*/* HTTP/1.1 200 OK")))
	assert.Equal(t, SEVERITY_ERROR, GetSeverity(tok.Tokenize("ERROR: yyyy/MM/dd HH:mm:ss - * : status=[500-502]")))
	assert.Equal(t, SEVERITY_ERROR, GetSeverity(tok.Tokenize("yyyy-MM-dd HH:mm:ss,SSS ERROR - Session creation failed: 500 {\"info\":\"failed to create session\"}")))
	assert.Equal(t, SEVERITY_INFO, GetSeverity(tok.Tokenize("INFO: session write failed: dial tcp 10.96.103.154:6379: connect: connection refused")))
	assert.Equal(t, SEVERITY_UNKNOWN, GetSeverity(tok.Tokenize("yyyy-MM-dd HH:mm:ss,SSS - Some log")))
	assert.Equal(t, SEVERITY_DEBUG, GetSeverity(tok.Tokenize("yyyy-MM-dd HH:mm:ss,SSS [DEBUG] - Some log")))
	assert.Equal(t, SEVERITY_INFO, GetSeverity(tok.Tokenize("[2026-02-23T17:15:20.385Z] Info : [us1-ddbuild-io] New rules in place")))
}
