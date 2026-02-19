//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rtokenizer provides tests for the Rust tokenizer integration.
//
// This file contains END-TO-END INTEGRATION TESTS that call the Rust tokenizer
// via FFI with real log strings and validate the complete pipeline:
//   Log string → Rust FFI → FlatBuffers → Go conversion → TokenList
//
// For UNIT TESTS of the conversion layer (no FFI), see token_conversion_test.go.

package rtokenizer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

func TestTokenizer_SimpleLog(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name            string
		log             string
		minTokens       int
		shouldHaveTypes []token.TokenType
		expectedValues  map[token.TokenType]string // expected normalized value for specific token type
	}{
		{
			name:            "Simple message",
			log:             "Hello World",
			minTokens:       3,
			shouldHaveTypes: []token.TokenType{token.TokenWord, token.TokenWhitespace, token.TokenWord},
			expectedValues: map[token.TokenType]string{
				token.TokenWord: "Hello", // first word preserved
			},
		},
		{
			name:            "Message with numbers",
			log:             "User 123 logged in",
			minTokens:       7,
			shouldHaveTypes: []token.TokenType{token.TokenWord, token.TokenWhitespace, token.TokenNumeric},
			expectedValues: map[token.TokenType]string{
				token.TokenNumeric: "123", // numbers preserved
				token.TokenWord:    "User",
			},
		},
		{
			name:            "Severity and message",
			log:             "ERROR Failed to connect",
			minTokens:       5,
			shouldHaveTypes: []token.TokenType{token.TokenSeverityLevel, token.TokenWhitespace, token.TokenWord},
			expectedValues: map[token.TokenType]string{
				// Raw values should be recovered via offsets
				token.TokenSeverityLevel: "ERROR",
				token.TokenWord:          "Failed", // first non-severity word
			},
		},
		{
			name:            "HTTP log",
			log:             "GET /api/users 200",
			minTokens:       5,
			shouldHaveTypes: []token.TokenType{token.TokenHTTPMethod, token.TokenWhitespace, token.TokenAbsolutePath},
			expectedValues: map[token.TokenType]string{
				// Raw values should be recovered via offsets
				token.TokenHTTPMethod:   "GET",
				token.TokenAbsolutePath: "/api/users", // paths preserved
				token.TokenHTTPStatus:   "200",        // status preserved
			},
		},
		{
			name:            "Log with IP",
			log:             "Request from 192.168.1.1",
			minTokens:       5,
			shouldHaveTypes: []token.TokenType{token.TokenWord, token.TokenWhitespace, token.TokenIPv4},
			expectedValues: map[token.TokenType]string{
				token.TokenIPv4: "192.168.1.1", // IPs preserved
				token.TokenWord: "Request",
			},
		},
		{
			name:            "Key-value pairs",
			log:             "status:ok level:info count:42",
			minTokens:       1,
			shouldHaveTypes: []token.TokenType{token.TokenKeyValueSequence},
			expectedValues: map[token.TokenType]string{
				// Raw values should be recovered via offsets
				token.TokenKeyValueSequence: "status:ok level:info count:42",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(tt.log)
			if err != nil {
				t.Fatalf("Tokenize failed: %v", err)
			}

			if tokenList.Length() < tt.minTokens {
				t.Errorf("Expected at least %d tokens, got %d", tt.minTokens, tokenList.Length())
			}

			// Check for expected token types
			for _, expectedType := range tt.shouldHaveTypes {
				found := false
				for _, tok := range tokenList.Tokens {
					if tok.Type == expectedType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find token type %v in result", expectedType)
				}
			}

			// Check for expected token values (these are normalized values)
			for expectedType, expectedValue := range tt.expectedValues {
				found := false
				for _, tok := range tokenList.Tokens {
					if tok.Type == expectedType && tok.Value == expectedValue {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find token with type %v and value %q", expectedType, expectedValue)
					// Print actual tokens for debugging
					t.Logf("Actual tokens:")
					for i, tok := range tokenList.Tokens {
						t.Logf("  [%d] Type: %v, Value: %q", i, tok.Type, tok.Value)
					}
				}
			}

			assertNonEmptyValues(t, tokenList)

			// Verify first word has NeverWildcard flag set
			if tokenList.Length() > 0 {
				hasFirstWord := false
				for _, tok := range tokenList.Tokens {
					if tok.Type == token.TokenWord && !tok.HasDigits {
						if !tok.NeverWildcard {
							t.Errorf("First word token should have NeverWildcard=true")
						}
						hasFirstWord = true
						break
					}
				}
				if !hasFirstWord && tt.name != "Key-value pairs" {
					// Key-value pairs might not have a regular word token
					t.Logf("Warning: No word token found in %q", tt.log)
				}
			}
		})
	}
}

func TestTokenizer_ComplexLogs(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name     string
		log      string
		validate func(*testing.T, *token.TokenList)
	}{
		{
			name: "Production log with email and IP",
			log:  "INFO User john.doe@example.com authenticated from 10.0.0.1",
			validate: func(t *testing.T, tl *token.TokenList) {
				if tl.Length() < 8 {
					t.Errorf("Expected at least 8 tokens, got %d", tl.Length())
				}

				// Should have severity, email, and IP tokens with correct normalized values
				hasSeverity := false
				hasEmail := false
				hasIP := false

				for _, tok := range tl.Tokens {
					switch tok.Type {
					case token.TokenSeverityLevel:
						hasSeverity = true
						// Raw values should be recovered via offsets
						if tok.Value != "INFO" {
							t.Errorf("Expected severity value %q, got %q", "INFO", tok.Value)
						}
					case token.TokenEmail:
						hasEmail = true
						// Emails are preserved during tokenization (wildcarded later during clustering)
						if tok.Value != "john.doe@example.com" {
							t.Errorf("Expected email value %q, got %q", "john.doe@example.com", tok.Value)
						}
					case token.TokenIPv4:
						hasIP = true
						// IPs are preserved
						if tok.Value != "10.0.0.1" {
							t.Errorf("Expected IP value %q, got %q", "10.0.0.1", tok.Value)
						}
					}
				}

				if !hasSeverity {
					t.Error("Expected to find severity token")
				}
				if !hasEmail {
					t.Error("Expected to find email token")
				}
				if !hasIP {
					t.Error("Expected to find IP token")
				}

				assertNonEmptyValues(t, tl)
			},
		},
		{
			name: "HTTP request log",
			log:  "172.16.0.1 - - [15/Jan/2024:10:30:45 +0000] \"GET /api/v1/users HTTP/1.1\" 200 1234",
			validate: func(t *testing.T, tl *token.TokenList) {
				if tl.Length() < 15 {
					t.Errorf("Expected at least 15 tokens, got %d", tl.Length())
				}

				hasIP := false
				hasHTTPMethod := false
				hasHTTPStatus := false
				pathTokens := []string{}

				for _, tok := range tl.Tokens {
					switch tok.Type {
					case token.TokenIPv4:
						hasIP = true
						// IPs are preserved
						if tok.Value != "172.16.0.1" {
							t.Errorf("Expected IP value %q, got %q", "172.16.0.1", tok.Value)
						}
					case token.TokenHTTPMethod:
						hasHTTPMethod = true
						// Raw values should be recovered via offsets
						if tok.Value != "GET" {
							t.Errorf("Expected HTTP method value %q, got %q", "GET", tok.Value)
						}
					case token.TokenHTTPStatus:
						hasHTTPStatus = true
						// HTTP status codes are preserved
						if tok.Value != "200" {
							t.Errorf("Expected HTTP status value %q, got %q", "200", tok.Value)
						}
					case token.TokenAbsolutePath:
						// Paths are preserved - collect all path tokens
						pathTokens = append(pathTokens, tok.Value)
					}
				}

				if !hasIP {
					t.Error("Expected to find IP token")
				}
				if !hasHTTPMethod {
					t.Error("Expected to find HTTP method token")
				}
				if !hasHTTPStatus {
					t.Error("Expected to find HTTP status token")
				}
				if len(pathTokens) == 0 {
					t.Error("Expected to find at least one path token")
				}

				// Verify all path tokens have values
				for i, pathValue := range pathTokens {
					if pathValue == "" {
						t.Errorf("Path token %d has empty value", i)
					}
				}

				assertNonEmptyValues(t, tl)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(tt.log)
			if err != nil {
				t.Fatalf("Tokenize failed: %v", err)
			}
			tt.validate(t, tokenList)
		})
	}
}

func TestTokenizer_EdgeCasesAndValidation(t *testing.T) {
	tokenizer := NewRustTokenizer()

	// Validate that diverse real-world logs produce valid tokens (non-empty values, correct types)
	testLogs := []string{
		"2024-01-15 10:30:00 INFO Server started on port 8080",
		"ERROR: Connection timeout after 30s",
		"User admin@example.com logged in from 192.168.1.100",
		"GET /api/v1/users?page=1&limit=10 HTTP/1.1 200",
		"Processing order_id:12345 status:completed amount:99.99",
		"WARN Memory usage: 85%",
		"[2024-01-15T10:30:45+00:00] DEBUG Request processing started",
	}

	for _, log := range testLogs {
		t.Run("Log: "+log[:min(40, len(log))], func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(log)
			if err != nil {
				t.Fatalf("Tokenize failed: %v", err)
			}

			if tokenList.Length() == 0 {
				t.Error("Expected at least one token")
				return
			}

			assertNonEmptyValues(t, tokenList)

			for i, tok := range tokenList.Tokens {
				// Verify value is valid for its type
				switch tok.Type {
				case token.TokenIPv4:
					// IPv4 addresses should be preserved or follow expected pattern
					if len(tok.Value) < 7 { // minimum: "0.0.0.0"
						t.Errorf("Token %d: IPv4 value too short: %q", i, tok.Value)
					}
				case token.TokenEmail:
					// Emails are preserved during tokenization (wildcarded later during clustering)
					if len(tok.Value) == 0 {
						t.Errorf("Token %d: Email value is empty", i)
					} else if tok.Value[0] == '*' {
						t.Errorf("Token %d: Email should not be wildcarded during tokenization, got %q", i, tok.Value)
					}
				case token.TokenNumeric:
					// Numeric values should contain digits
					hasDigit := false
					for _, c := range tok.Value {
						if c >= '0' && c <= '9' {
							hasDigit = true
							break
						}
					}
					if !hasDigit {
						t.Errorf("Token %d: Numeric token should contain digits: %q", i, tok.Value)
					}
				case token.TokenHTTPStatus:
					// HTTP status codes are 3 digits
					if len(tok.Value) != 3 {
						t.Errorf("Token %d: HTTP status should be 3 digits, got %q", i, tok.Value)
					}
				}
			}

			// Verify token values are consistent with token types
			for i, tok := range tokenList.Tokens {
				if tok.Type == token.TokenWhitespace {
					// Whitespace tokens should only contain whitespace
					for _, c := range tok.Value {
						if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
							t.Errorf("Token %d: Whitespace token contains non-whitespace: %q", i, tok.Value)
						}
					}
				}
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func assertNonEmptyValues(t *testing.T, tokenList *token.TokenList) {
	t.Helper()
	for i, tok := range tokenList.Tokens {
		if tok.Value == "" {
			t.Errorf("Token %d has empty value (type: %v)", i, tok.Type)
		}
	}
}

func TestTokenizer_EmptyString(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tokenList, err := tokenizer.Tokenize("")
	if err != nil {
		t.Fatalf("Tokenize failed: %v", err)
	}

	if tokenList.Length() != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", tokenList.Length())
	}
}

func TestTokenizer_WhitespaceOnly(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tokenList, err := tokenizer.Tokenize("   ")
	if err != nil {
		t.Fatalf("Tokenize failed: %v", err)
	}

	// The consolidator might remove pure whitespace, so just check it doesn't error
	// and returns a valid (possibly empty) token list
	if tokenList == nil {
		t.Error("Expected non-nil token list")
	}
}

func TestTokenizer_UnicodeInput(t *testing.T) {
	tokenizer := NewRustTokenizer()

	logs := []string{
		"用户 登录 成功",
		"User 用户 logged in",
	}

	for _, log := range logs {
		t.Run(log, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(log)
			if err != nil {
				t.Fatalf("Tokenize failed: %v", err)
			}
			if tokenList == nil {
				t.Fatal("Expected non-nil token list")
			}
		})
	}
}

func TestTokenizer_RealisticSamples(t *testing.T) {
	tokenizer := NewRustTokenizer()

	logs := []string{
		`127.0.0.1 - - [10/Feb/2026:12:34:56 +0000] "GET /api/v1/users HTTP/1.1" 200 123 "-" "Mozilla/5.0"`,
		`2026-02-10T12:34:56.789Z INFO service=payments request_id=abc123 duration_ms=42`,
		`Feb 10 12:34:56 host1 sshd[12345]: Failed password for invalid user root from 10.0.0.1 port 2222 ssh2`,
		`{"level":"error","msg":"db timeout","duration_ms":3000,"request_id":"r-1","ip":"192.168.1.1"}`,
		`kubelet[123]: I0210 12:34:56.789012 12345 pod_workers.go:191] "SyncPod" pod="default/nginx-12345"`,
		`GET https://example.com/api/v1/orders?limit=10&offset=20 504`,
		`WARN cache miss for key user:12345 in region us-east-1`,
		`2026-02-10 12:34:56 ERROR [auth] user=alice@example.com ip=2001:db8::1 status=401`,
		`User bob logged in from 192.168.1.2 with session=abc-def-123`,
		`/var/log/syslog rotated; size=102400 bytes`,
	}

	for _, log := range logs {
		t.Run(log[:min(40, len(log))], func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(log)
			if err != nil {
				t.Fatalf("Tokenize failed: %v", err)
			}
			if tokenList == nil {
				t.Fatal("Expected non-nil token list")
			}
			if tokenList.Length() == 0 {
				t.Fatalf("Expected tokens for log: %q", log)
			}
		})
	}
}

func TestTokenizer_AllTokenTypes(t *testing.T) {
	tokenizer := NewRustTokenizer()

	// Comprehensive test covering token types produced by Rust tokenizer
	//
	// Agent defines 23 token types total, Rust tokenizer actively produces 13-14 of them:
	//
	// ✅ TESTED (13 types):
	//   - Basic: TokenWord, TokenNumeric, TokenWhitespace, TokenSpecialChar
	//   - Network: TokenIPv4, TokenEmail, TokenURI, TokenAbsolutePath
	//   - HTTP: TokenHTTPMethod, TokenHTTPStatus
	//   - Log: TokenSeverityLevel, TokenLocalDateTime
	//   - Advanced: TokenKeyValueSequence
	//
	// ⚠️  MAPPED but NOT TESTED (1 type):
	//   - TokenCollapsedToken: Can be produced by Rust but not commonly seen in basic logs
	//
	// 🔧 FALLBACK (1 type):
	//   - TokenUnknown: Default fallback for unmapped/invalid tokens (not actively produced by Rust)
	//
	// ❌ NOT PRODUCED by Rust tokenizer (8 types):
	//   - TokenIPv6: Broken into Numeric/Word/SpecialChar components (not yet supported)
	//   - TokenAuthority: Only embedded within TokenURI, never standalone
	//   - TokenPathWithQueryAndFragment: Not consolidated, remains as AbsolutePath + components
	//   - TokenRegularName: Hostnames/domains tokenized as Word + SpecialChar components
	//   - TokenDate, TokenLocalDate, TokenLocalTime, TokenOffsetDateTime (4 types):
	//     All consolidated into TokenLocalDateTime (Rust's aggressive consolidation strategy)
	tests := []struct {
		name              string
		log               string
		expectedTokenType token.TokenType
		expectedValue     string                               // Expected exact value (or empty to just check non-empty)
		validateValue     func(t *testing.T, tok *token.Token) // Optional custom validation
	}{
		{
			name:              "Word token",
			log:               "Hello world",
			expectedTokenType: token.TokenWord,
			expectedValue:     "Hello",
		},
		{
			name:              "Numeric token",
			log:               "Count 12345",
			expectedTokenType: token.TokenNumeric,
			expectedValue:     "12345",
		},
		{
			name:              "IPv4 address",
			log:               "Server 192.168.1.100",
			expectedTokenType: token.TokenIPv4,
			expectedValue:     "192.168.1.100",
		},
		{
			name:              "Email address",
			log:               "Contact admin@example.com",
			expectedTokenType: token.TokenEmail,
			expectedValue:     "admin@example.com", // Preserved during tokenization
		},
		{
			name:              "HTTP method",
			log:               "POST /api/users",
			expectedTokenType: token.TokenHTTPMethod,
			expectedValue:     "POST",
		},
		{
			name:              "HTTP status",
			log:               "Response 404",
			expectedTokenType: token.TokenHTTPStatus,
			expectedValue:     "404",
		},
		{
			name:              "Severity level",
			log:               "ERROR connection failed",
			expectedTokenType: token.TokenSeverityLevel,
			expectedValue:     "ERROR",
		},
		{
			name:              "Absolute path",
			log:               "File /var/log/messages",
			expectedTokenType: token.TokenAbsolutePath,
			expectedValue:     "/var/log/messages",
		},
		{
			name:              "URI",
			log:               "Visit https://example.com/path",
			expectedTokenType: token.TokenURI,
			expectedValue:     "", // Just verify non-empty
			validateValue: func(t *testing.T, tok *token.Token) {
				if len(tok.Value) == 0 {
					t.Error("URI should have non-empty value")
				}
				if len(tok.Value) < 8 {
					t.Errorf("URI should be at least 8 chars, got %q", tok.Value)
					return
				}
				if tok.Value[:8] != "https://" {
					t.Errorf("URI should start with https://, got %q", tok.Value)
				}
			},
		},
		{
			name:              "Key-value sequence",
			log:               "status:ok level:info count:42",
			expectedTokenType: token.TokenKeyValueSequence,
			expectedValue:     "", // Just verify non-empty and raw format
			validateValue: func(t *testing.T, tok *token.Token) {
				if len(tok.Value) == 0 {
					t.Error("KV sequence should have non-empty value")
				}
				if tok.Value != "status:ok level:info count:42" {
					t.Errorf("KV sequence should preserve raw text, got %q", tok.Value)
				}
			},
		},
		{
			name:              "Special character",
			log:               "Error: connection",
			expectedTokenType: token.TokenSpecialChar,
			expectedValue:     ":",
		},
		{
			name:              "Whitespace",
			log:               "Hello   world", // Multiple spaces
			expectedTokenType: token.TokenWhitespace,
			expectedValue:     "", // Just verify it exists
			validateValue: func(t *testing.T, tok *token.Token) {
				if len(tok.Value) == 0 {
					t.Error("Whitespace should have non-empty value")
				}
			},
		},
		{
			name:              "Local date/time",
			log:               "Date: 2024-01-15",
			expectedTokenType: token.TokenLocalDateTime, // Rust produces LocalDateTime (more specific)
			expectedValue:     "2024-01-15",             // Raw value from offsets
		},
		{
			name:              "Local date (consolidated to LocalDateTime)",
			log:               "Date 2024-01-15",
			expectedTokenType: token.TokenLocalDateTime,
			expectedValue:     "2024-01-15",
		},
		{
			name:              "Local time (consolidated to LocalDateTime)",
			log:               "Time 14:30:00",
			expectedTokenType: token.TokenLocalDateTime,
			expectedValue:     "14:30:00",
		},
		{
			name:              "Offset date time (consolidated to LocalDateTime)",
			log:               "Event 2024-01-15T10:30:45+00:00",
			expectedTokenType: token.TokenLocalDateTime,
			expectedValue:     "2024-01-15T10:30:45+00:00",
		},
		// NOTE: IPv6 not supported by Rust tokenizer yet
		// It gets broken into: Numeric, SpecialChar, Word components

		// NOTE: Authority only appears within URI tokens, not standalone
		// Testing URI above already covers Authority

		// NOTE: PathWithQueryAndFragment not consolidated by Rust tokenizer
		// Query/fragment parts remain as separate tokens (AbsolutePath + SpecialChar + Word)

		// NOTE: RegularName not produced by Rust tokenizer
		// Hostnames/domains are tokenized as Word + SpecialChar components
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Tokenize
			tokenList, err := tokenizer.Tokenize(tt.log)
			if err != nil {
				t.Fatalf("Tokenization failed: %v", err)
			}

			// Find expected token type
			var foundToken *token.Token
			for i := range tokenList.Tokens {
				if tokenList.Tokens[i].Type == tt.expectedTokenType {
					foundToken = &tokenList.Tokens[i]
					break
				}
			}

			if foundToken == nil {
				t.Errorf("Expected token type %v not found in output", tt.expectedTokenType)
				t.Logf("Log: %q", tt.log)
				t.Logf("Tokens found:")
				for i, tok := range tokenList.Tokens {
					t.Logf("  [%d] Type=%v, Value=%q", i, tok.Type, tok.Value)
				}
				return
			}

			// Validate value if exact match is expected
			if tt.expectedValue != "" && foundToken.Value != tt.expectedValue {
				t.Errorf("Expected value %q, got %q", tt.expectedValue, foundToken.Value)
			}

			// Run custom validation if provided
			if tt.validateValue != nil {
				tt.validateValue(t, foundToken)
			}
		})
	}
}

func TestTokenizer_TokensNotProduced(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name          string
		log           string
		absentTypes   []token.TokenType
		presentTypes  []token.TokenType
		presentValues map[token.TokenType]string
	}{
		{
			name:        "IPv6 split into components",
			log:         "Client 2001:db8::1 connected",
			absentTypes: []token.TokenType{token.TokenIPv6},
			presentTypes: []token.TokenType{
				token.TokenNumeric,
				token.TokenSpecialChar,
				token.TokenWord,
			},
		},
		{
			name:        "Authority split into email + port",
			log:         "proxy user@host.example.com:8080",
			absentTypes: []token.TokenType{token.TokenAuthority},
			presentTypes: []token.TokenType{
				token.TokenEmail,
				token.TokenNumeric,
				token.TokenSpecialChar,
			},
			presentValues: map[token.TokenType]string{
				token.TokenEmail: "user@host.example.com",
			},
		},
		{
			name:        "Path with query/fragment split",
			log:         "GET /api/v1/users?page=1#top",
			absentTypes: []token.TokenType{token.TokenPathWithQueryAndFragment},
			presentTypes: []token.TokenType{
				token.TokenAbsolutePath,
				token.TokenSpecialChar,
				token.TokenWord,
				token.TokenNumeric,
			},
			presentValues: map[token.TokenType]string{
				token.TokenAbsolutePath: "/api/v1/users",
			},
		},
		{
			name:        "Regular name split into words/special chars",
			log:         "Host host.example.com resolved",
			absentTypes: []token.TokenType{token.TokenRegularName},
			presentTypes: []token.TokenType{
				token.TokenWord,
				token.TokenSpecialChar,
			},
			presentValues: map[token.TokenType]string{
				token.TokenWord: "host",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(tt.log)
			if err != nil {
				t.Fatalf("Tokenize failed: %v", err)
			}

			for _, absent := range tt.absentTypes {
				for _, tok := range tokenList.Tokens {
					if tok.Type == absent {
						t.Errorf("Did not expect token type %v", absent)
						break
					}
				}
			}

			for _, present := range tt.presentTypes {
				found := false
				for _, tok := range tokenList.Tokens {
					if tok.Type == present {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find token type %v", present)
				}
			}

			for expectedType, expectedValue := range tt.presentValues {
				found := false
				for _, tok := range tokenList.Tokens {
					if tok.Type == expectedType && tok.Value == expectedValue {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected to find token with type %v and value %q", expectedType, expectedValue)
				}
			}
		})
	}
}
