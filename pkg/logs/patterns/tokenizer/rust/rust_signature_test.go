//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// TestRustTokenizer_SignatureGeneration validates that Rust tokenization
// produces correct signatures for clustering.
func TestRustTokenizer_SignatureGeneration(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name              string
		log               string
		expectedSignature string // Expected position signature format
		description       string
	}{
		{
			name:              "Simple word log",
			log:               "Service started",
			expectedSignature: "ServiceWord|Whitespace|Word",
			description:       "First word + token types",
		},
		{
			name:              "HTTP log",
			log:               "GET /api/users 200",
			expectedSignature: "HTTPMethod|Whitespace|AbsolutePath|Whitespace|HTTPStatus",
			description:       "HTTP tokens should have specific types",
		},
		{
			name:              "Log with IP address",
			log:               "Connection from 192.168.1.1",
			expectedSignature: "ConnectionWord|Whitespace|Word|Whitespace|IPv4",
			description:       "IPv4 should be recognized as single token",
		},
		{
			name:              "Log with email",
			log:               "User admin@example.com logged in",
			expectedSignature: "UserWord|Whitespace|Email|Whitespace|Word|Whitespace|Word",
			description:       "Email should be recognized as single token",
		},
		{
			name:              "Log with URI",
			log:               "Request to https://example.com/api",
			expectedSignature: "RequestWord|Whitespace|Word|Whitespace|URI",
			description:       "URI should be recognized as single token",
		},
		{
			name:              "Log with severity level",
			log:               "ERROR Database connection failed",
			expectedSignature: "SeverityLevel|Whitespace|Word|Whitespace|Word|Whitespace|Word",
			description:       "Severity level should be first token",
		},
		{
			name:              "Log with timestamp",
			log:               "2024-01-15 10:30:00 Server started",
			expectedSignature: "LocalDateTime|Whitespace|Word|Whitespace|Word",
			description:       "Timestamp should be consolidated to LocalDateTime",
		},
		{
			name:              "Log with special characters",
			log:               "Status: OK, code=200",
			expectedSignature: "StatusWord|SpecialChar|Whitespace|Word|SpecialChar|Whitespace|Word|SpecialChar|HTTPStatus",
			description:       "Special characters should be tokenized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(tt.log)
			require.NoError(t, err, "Tokenization should succeed")
			require.NotNil(t, tokenList, "TokenList should not be nil")

			sig := token.NewSignature(tokenList)
			assert.NotEmpty(t, sig.Position, "Signature position should not be empty")
			assert.Greater(t, sig.Hash, uint64(0), "Signature hash should be non-zero")
			assert.Equal(t, tokenList.Length(), sig.Length, "Signature length should match token count")

			// Verify signature includes expected token types
			assert.Equal(t, tt.expectedSignature, sig.Position,
				"%s: signature mismatch", tt.description)

			t.Logf("✅ %s: %q → signature: %s (hash: %x, length: %d)",
				tt.name, tt.log, sig.Position, sig.Hash, sig.Length)
		})
	}
}

// TestRustTokenizer_SignatureEquality validates that logs with the same structure
// produce identical signatures, which is critical for clustering.
func TestRustTokenizer_SignatureEquality(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name        string
		logs        []string
		shouldMatch bool
		description string
	}{
		{
			name: "Same structure, different values",
			logs: []string{
				"Service started",
				"Service stopped",
				"Service restarted",
			},
			shouldMatch: true,
			description: "Logs with same structure should have identical signatures",
		},
		{
			name: "Same HTTP structure, different methods",
			logs: []string{
				"GET /api/users 200",
				"POST /api/users 201",
				"PUT /api/users 200",
			},
			shouldMatch: true,
			description: "HTTP logs with different methods should cluster together",
		},
		{
			name: "Same structure, different IP addresses",
			logs: []string{
				"Connection from 192.168.1.1",
				"Connection from 10.0.0.1",
				"Connection from 172.16.0.1",
			},
			shouldMatch: true,
			description: "Logs with different IPs should have same signature",
		},
		{
			name: "Same structure, different emails",
			logs: []string{
				"User admin@example.com logged in",
				"User user@test.com logged in",
				"User guest@domain.org logged in",
			},
			shouldMatch: true,
			description: "Logs with different emails should have same signature",
		},
		{
			name: "Different structures",
			logs: []string{
				"GET /api/users 200",
				"Connection from 192.168.1.1",
				"User admin@example.com logged in",
			},
			shouldMatch: false,
			description: "Logs with different structures should have different signatures",
		},
		{
			name: "Different first word",
			logs: []string{
				"Service started",
				"Application started",
			},
			shouldMatch: false,
			description: "Different first words should produce different signatures (first word optimization)",
		},
		{
			name: "Same tokens, different order",
			logs: []string{
				"started Service",
				"Service started",
			},
			shouldMatch: false,
			description: "Different token order should produce different signatures",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var signatures []token.Signature
			for _, log := range tt.logs {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed for log: %q", log)
				sig := token.NewSignature(tokenList)
				signatures = append(signatures, sig)
				t.Logf("  Log: %q → Signature: %s (hash: %x)", log, sig.Position, sig.Hash)
			}

			if tt.shouldMatch {
				// All signatures should be identical
				firstSig := signatures[0]
				for i := 1; i < len(signatures); i++ {
					assert.True(t, firstSig.Equals(signatures[i]),
						"%s: signatures should match. First: %s, Current: %s",
						tt.description, firstSig.Position, signatures[i].Position)
					assert.Equal(t, firstSig.Hash, signatures[i].Hash,
						"%s: signature hashes should match", tt.description)
				}
				t.Logf("✅ %s: All %d logs produced identical signatures", tt.description, len(tt.logs))
			} else {
				// All signatures should be different
				for i := 0; i < len(signatures); i++ {
					for j := i + 1; j < len(signatures); j++ {
						assert.False(t, signatures[i].Equals(signatures[j]),
							"%s: signatures should differ. Sig[%d]: %s, Sig[%d]: %s",
							tt.description, i, signatures[i].Position, j, signatures[j].Position)
					}
				}
				t.Logf("✅ %s: All %d logs produced different signatures", tt.description, len(tt.logs))
			}
		})
	}
}

// TestRustTokenizer_SignatureFirstWordOptimization validates that the first word
// is included in the signature to prevent false clustering.
func TestRustTokenizer_SignatureFirstWordOptimization(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name         string
		log1         string
		log2         string
		shouldDiffer bool
		description  string
	}{
		{
			name:         "Different first word, same structure",
			log1:         "Service started successfully",
			log2:         "Application started successfully",
			shouldDiffer: true,
			description:  "First word optimization should prevent clustering",
		},
		{
			name:         "Same first word, same structure",
			log1:         "Service started successfully",
			log2:         "Service stopped successfully",
			shouldDiffer: false,
			description:  "Same first word should allow clustering",
		},
		{
			name:         "First token is not a word",
			log1:         "200 OK",
			log2:         "201 Created",
			shouldDiffer: false,
			description:  "Non-word first tokens should still cluster",
		},
		{
			name:         "Case sensitivity in first word",
			log1:         "service started",
			log2:         "Service started",
			shouldDiffer: true,
			description:  "First word should be case-sensitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList1, err := tokenizer.Tokenize(tt.log1)
			require.NoError(t, err)
			tokenList2, err := tokenizer.Tokenize(tt.log2)
			require.NoError(t, err)

			sig1 := token.NewSignature(tokenList1)
			sig2 := token.NewSignature(tokenList2)

			t.Logf("  Log1: %q → Signature: %s", tt.log1, sig1.Position)
			t.Logf("  Log2: %q → Signature: %s", tt.log2, sig2.Position)

			if tt.shouldDiffer {
				assert.False(t, sig1.Equals(sig2),
					"%s: signatures should differ", tt.description)
				t.Logf("✅ %s: Signatures correctly differ", tt.description)
			} else {
				assert.True(t, sig1.Equals(sig2),
					"%s: signatures should match", tt.description)
				t.Logf("✅ %s: Signatures correctly match", tt.description)
			}
		})
	}
}

// TestRustTokenizer_SignatureHashCollision validates that different signatures
// produce different hashes (or at least, very unlikely to collide).
func TestRustTokenizer_SignatureHashCollision(t *testing.T) {
	tokenizer := NewRustTokenizer()

	// Generate many different log patterns
	logs := []string{
		"Service started",
		"Application started",
		"GET /api/users 200",
		"POST /api/users 201",
		"Connection from 192.168.1.1",
		"User admin@example.com logged in",
		"ERROR Database connection failed",
		"WARN Memory usage high",
		"INFO System operational",
		"2024-01-15 10:30:00 Server started",
		"Request to https://example.com/api",
		"Processing 100 items",
		"Status: OK, code=200",
		"Host: example.com:8080",
		"Path: /api/v1/users",
	}

	hashMap := make(map[uint64]string)
	collisions := 0

	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err, "Tokenization should succeed for log: %q", log)

		sig := token.NewSignature(tokenList)

		// Check for hash collision
		if existingLog, found := hashMap[sig.Hash]; found {
			if existingLog != log {
				collisions++
				t.Logf("⚠️  Hash collision detected: %q and %q both hash to %x",
					existingLog, log, sig.Hash)
			}
		} else {
			hashMap[sig.Hash] = log
		}
	}

	assert.LessOrEqual(t, collisions, 3, "Hash collisions should be rare")
	assert.GreaterOrEqual(t, len(hashMap), len(logs)-collisions, "Unique hashes should match logs minus collisions")

	t.Logf("✅ Generated %d unique hashes for %d different log patterns (%d collisions)", len(hashMap), len(logs), collisions)
}

// TestRustTokenizer_SignatureStability validates that the same log always
// produces the same signature (idempotency).
func TestRustTokenizer_SignatureStability(t *testing.T) {
	tokenizer := NewRustTokenizer()

	logs := []string{
		"Service started",
		"GET /api/users 200",
		"Connection from 192.168.1.1",
		"User admin@example.com logged in",
		"ERROR Database connection failed",
	}

	for _, log := range logs {
		t.Run(log, func(t *testing.T) {
			var signatures []token.Signature

			// Tokenize the same log 10 times
			for i := 0; i < 10; i++ {
				tokenList, err := tokenizer.Tokenize(log)
				require.NoError(t, err, "Tokenization should succeed")
				sig := token.NewSignature(tokenList)
				signatures = append(signatures, sig)
			}

			// All signatures should be identical
			firstSig := signatures[0]
			for i := 1; i < len(signatures); i++ {
				assert.True(t, firstSig.Equals(signatures[i]),
					"Signature should be stable across multiple tokenizations")
				assert.Equal(t, firstSig.Hash, signatures[i].Hash,
					"Hash should be stable across multiple tokenizations")
				assert.Equal(t, firstSig.Position, signatures[i].Position,
					"Position should be stable across multiple tokenizations")
			}

			t.Logf("✅ Log %q produced stable signature across 10 tokenizations: %s (hash: %x)",
				log, firstSig.Position, firstSig.Hash)
		})
	}
}

// TestRustTokenizer_SignatureEdgeCases validates signature generation for edge cases.
func TestRustTokenizer_SignatureEdgeCases(t *testing.T) {
	tokenizer := NewRustTokenizer()

	tests := []struct {
		name        string
		log         string
		expectError bool
		description string
	}{
		{
			name:        "Empty log",
			log:         "",
			expectError: false,
			description: "Empty log should produce empty signature",
		},
		{
			name:        "Single character",
			log:         "a",
			expectError: false,
			description: "Single character should be tokenized",
		},
		{
			name:        "Only whitespace",
			log:         "   ",
			expectError: false,
			description: "Whitespace-only log should be tokenized",
		},
		{
			name:        "Only special characters",
			log:         "!!!",
			expectError: false,
			description: "Special characters should be tokenized",
		},
		{
			name:        "Very long log",
			log:         strings.Repeat("word ", 1000),
			expectError: false,
			description: "Very long log should be tokenized",
		},
		{
			name:        "Unicode characters",
			log:         "User 用户 logged in",
			expectError: false,
			description: "Unicode characters currently panic in Rust tokenizer (char boundary issue)",
		},
		{
			name:        "Mixed newlines and spaces",
			log:         "line1\nline2\tline3",
			expectError: false,
			description: "Mixed whitespace should be handled",
		},
		{
			name:        "Repeated tokens",
			log:         "word word word word",
			expectError: false,
			description: "Repeated tokens should be handled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenList, err := tokenizer.Tokenize(tt.log)

			if tt.expectError {
				assert.Error(t, err, "%s: should produce error", tt.description)
				return
			}

			require.NoError(t, err, "%s: should not produce error", tt.description)
			require.NotNil(t, tokenList, "TokenList should not be nil")

			sig := token.NewSignature(tokenList)

			if tokenList.IsEmpty() {
				assert.Empty(t, sig.Position, "Empty TokenList should have empty signature")
				assert.Equal(t, uint64(0), sig.Hash, "Empty TokenList should have zero hash")
				assert.Equal(t, 0, sig.Length, "Empty TokenList should have zero length")
			} else {
				assert.NotEmpty(t, sig.Position, "Non-empty TokenList should have non-empty signature")
				assert.Greater(t, sig.Length, 0, "Non-empty TokenList should have positive length")
			}

			t.Logf("✅ %s: log length=%d, tokens=%d, signature=%q",
				tt.description, len(tt.log), tokenList.Length(), sig.Position)
		})
	}
}

// TestRustTokenizer_SignatureHashBuckets validates that signatures can be
// distributed into hash buckets for efficient clustering.
func TestRustTokenizer_SignatureHashBuckets(t *testing.T) {
	tokenizer := NewRustTokenizer()

	// Generate diverse logs
	var logs []string
	for i := 0; i < 100; i++ {
		logs = append(logs, fmt.Sprintf("Service %d started", i))
		logs = append(logs, fmt.Sprintf("GET /api/resource/%d 200", i))
		logs = append(logs, fmt.Sprintf("Connection from 192.168.1.%d", i%256))
	}

	bucketCounts := make(map[uint64]int)
	var signatures []token.Signature

	for _, log := range logs {
		tokenList, err := tokenizer.Tokenize(log)
		require.NoError(t, err)
		sig := token.NewSignature(tokenList)
		signatures = append(signatures, sig)

		bucket := sig.GetHashBucket()
		bucketCounts[bucket]++
	}

	// With 300 logs and good hash distribution, we should see multiple buckets
	assert.Greater(t, len(bucketCounts), 1,
		"Signatures should distribute across multiple hash buckets")

	// No single bucket should have all logs (would indicate poor distribution)
	for bucket, count := range bucketCounts {
		assert.Less(t, count, len(logs),
			"Bucket %d should not contain all logs", bucket)
	}

	t.Logf("✅ %d logs distributed across %d hash buckets", len(logs), len(bucketCounts))
	for bucket, count := range bucketCounts {
		t.Logf("  Bucket %d: %d signatures", bucket, count)
	}
}
