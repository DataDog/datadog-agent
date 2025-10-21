// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHybridDetector_SSN tests SSN detection using token patterns
func TestHybridDetector_SSN(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name          string
		input         string
		expected      string
		shouldMatch   bool
		expectedRules []string
	}{
		{
			name:          "valid_ssn",
			input:         "SSN is 123-45-6789",
			expected:      "SSN is [SSN_REDACTED]",
			shouldMatch:   true,
			expectedRules: []string{"auto_redact_ssn"},
		},
		{
			name:          "ssn_in_sentence",
			input:         "User with SSN 987-65-4321 created account",
			expected:      "User with SSN [SSN_REDACTED] created account",
			shouldMatch:   true,
			expectedRules: []string{"auto_redact_ssn"},
		},
		{
			name:          "multiple_ssns",
			input:         "Transfer from 111-22-3333 to 444-55-6666",
			expected:      "Transfer from [SSN_REDACTED] to [SSN_REDACTED]",
			shouldMatch:   true,
			expectedRules: []string{"auto_redact_ssn", "auto_redact_ssn"},
		},
		{
			name:          "not_ssn_date",
			input:         "Date: 2024-10-20",
			expected:      "Date: 2024-10-20",
			shouldMatch:   false,
			expectedRules: nil,
		},
		{
			name:          "not_ssn_but_valid_phone",
			input:         "Code: 555-456-7890", // DDD-DDD-DDDD phone pattern, valid area code
			expected:      "Code: [PHONE_REDACTED]",
			shouldMatch:   true,
			expectedRules: []string{"auto_redact_phone_dashed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rules := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result), "Redaction mismatch")
			if tt.shouldMatch {
				assert.NotEmpty(t, rules, "Expected rules to match")
				assert.Equal(t, len(tt.expectedRules), len(rules), "Rule count mismatch")
			} else {
				assert.Empty(t, rules, "Expected no rules to match")
			}
		})
	}
}

// TestHybridDetector_CreditCard tests credit card detection
func TestHybridDetector_CreditCard(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name        string
		input       string
		expected    string
		shouldMatch bool
	}{
		// Formatted credit cards (token-based)
		{
			name:        "cc_dashed_visa",
			input:       "Card: 4532-0151-1283-0366",
			expected:    "Card: [CC_REDACTED]",
			shouldMatch: true,
		},
		{
			name:        "cc_spaced_mastercard",
			input:       "Payment: 5425 2334 3010 9903",
			expected:    "Payment: [CC_REDACTED]",
			shouldMatch: true,
		},
		// Should NOT match - not valid credit card BIN
		{
			name:        "not_cc_wrong_bin",
			input:       "Order #1234567890123456",
			expected:    "Order #1234567890123456",
			shouldMatch: false,
		},
		{
			name:        "not_cc_tracking_number",
			input:       "Tracking: 9999999999999999",
			expected:    "Tracking: 9999999999999999",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rules := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result), "Redaction mismatch")
			if tt.shouldMatch {
				assert.NotEmpty(t, rules, "Expected CC rule to match")
			} else {
				assert.Empty(t, rules, "Expected no CC rule to match (false positive)")
			}
		})
	}
}

// TestHybridDetector_Phone tests phone number detection
func TestHybridDetector_Phone(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name        string
		input       string
		expected    string
		shouldMatch bool
	}{
		// Formatted phones (token-based with confirmation)
		{
			name:        "phone_parens",
			input:       "Call (555) 123-4567",
			expected:    "Call [PHONE_REDACTED]",
			shouldMatch: true,
		},
		{
			name:        "phone_dashed",
			input:       "Contact: 555-123-4567",
			expected:    "Contact: [PHONE_REDACTED]",
			shouldMatch: true,
		},
		{
			name:        "phone_dotted",
			input:       "Number: 555.123.4567",
			expected:    "Number: [PHONE_REDACTED]",
			shouldMatch: true,
		},
		// Unformatted phones (token + regex confirmation)
		{
			name:        "phone_unformatted",
			input:       "Call 5551234567 now",
			expected:    "Call [PHONE_REDACTED] now",
			shouldMatch: true,
		},
		// Should NOT match - invalid area codes
		{
			name:        "not_phone_area_code_0",
			input:       "Code: 055-123-4567", // Area code can't start with 0
			expected:    "Code: 055-123-4567",
			shouldMatch: false,
		},
		{
			name:        "not_phone_area_code_1",
			input:       "Code: 155-123-4567", // Area code can't start with 1
			expected:    "Code: 155-123-4567",
			shouldMatch: false,
		},
		{
			name:        "not_phone_timestamp",
			input:       "Time: 012-345-6789", // Looks like phone but starts with 0
			expected:    "Time: 012-345-6789",
			shouldMatch: false,
		},
		// Edge case: multiple phones
		{
			name:        "multiple_phones",
			input:       "Call (555) 123-4567 or 555-987-6543",
			expected:    "Call [PHONE_REDACTED] or [PHONE_REDACTED]",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rules := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result), "Redaction mismatch")
			if tt.shouldMatch {
				assert.NotEmpty(t, rules, "Expected phone rule to match")
			} else {
				assert.Empty(t, rules, "Expected no phone rule to match (false positive prevention)")
			}
		})
	}
}

// TestHybridDetector_Email tests email detection (regex-based)
func TestHybridDetector_Email(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple_email",
			input:    "Contact: user@example.com",
			expected: "Contact: [EMAIL_REDACTED]",
		},
		{
			name:     "complex_email",
			input:    "Email: john.doe+tag@subdomain.example.co.uk",
			expected: "Email: [EMAIL_REDACTED]",
		},
		{
			name:     "multiple_emails",
			input:    "From: a@b.com To: x@y.com",
			expected: "From: [EMAIL_REDACTED] To: [EMAIL_REDACTED]",
		},
		{
			name:     "email_with_numbers",
			input:    "User: user123@test456.com",
			expected: "User: [EMAIL_REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rules := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result))
			assert.Contains(t, rules, "auto_redact_email")
		})
	}
}

// TestHybridDetector_IPv4 tests IP address detection (regex-based)
func TestHybridDetector_IPv4(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ipv4_standard",
			input:    "Connection from 192.168.1.100",
			expected: "Connection from [IP_REDACTED]",
		},
		{
			name:     "ipv4_short",
			input:    "Server: 10.0.0.1",
			expected: "Server: [IP_REDACTED]",
		},
		{
			name:     "ipv4_public",
			input:    "DNS: 8.8.8.8",
			expected: "DNS: [IP_REDACTED]",
		},
		{
			name:     "multiple_ips",
			input:    "Route: 10.0.1.1 -> 172.16.0.1",
			expected: "Route: [IP_REDACTED] -> [IP_REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rules := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result))
			assert.Contains(t, rules, "auto_redact_ipv4")
		})
	}
}

// TestHybridDetector_MultiplePIITypes tests mixed PII in one message
func TestHybridDetector_MultiplePIITypes(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "email_and_ssn",
			input:    "User john@example.com with SSN 123-45-6789",
			expected: "User [EMAIL_REDACTED] with SSN [SSN_REDACTED]",
		},
		{
			name:     "phone_and_credit_card",
			input:    "Call (555) 123-4567 for card 4532-0151-1283-0366",
			expected: "Call [PHONE_REDACTED] for card [CC_REDACTED]",
		},
		{
			name:     "all_pii_types",
			input:    "User: john@test.com, SSN: 987-65-4321, Phone: 555-123-4567, Card: 4532 0151 1283 0366, IP: 10.0.1.1",
			expected: "User: [EMAIL_REDACTED], SSN: [SSN_REDACTED], Phone: [PHONE_REDACTED], Card: [CC_REDACTED], IP: [IP_REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rules := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result))
			assert.NotEmpty(t, rules, "Expected multiple rules to match")
		})
	}
}

// TestHybridDetector_EdgeCases tests boundary conditions
func TestHybridDetector_EdgeCases(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "no_pii",
			input:    "This is a normal log message with no PII",
			expected: "This is a normal log message with no PII",
		},
		{
			name:     "pii_at_start",
			input:    "123-45-6789 is the SSN",
			expected: "[SSN_REDACTED] is the SSN",
		},
		{
			name:     "pii_at_end",
			input:    "The SSN is 123-45-6789",
			expected: "The SSN is [SSN_REDACTED]",
		},
		{
			name:     "adjacent_pii",
			input:    "SSN:123-45-6789,Phone:555-123-4567",
			expected: "SSN:[SSN_REDACTED],Phone:[PHONE_REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

// TestHybridDetector_NoTokenizer tests that regex rules still work without tokenizer
func TestHybridDetector_NoTokenizer(t *testing.T) {
	detector := NewHybridPIIDetector()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "email_without_tokenizer",
			input:    "Contact: user@example.com",
			expected: "Contact: [EMAIL_REDACTED]",
		},
		{
			name:     "ip_without_tokenizer",
			input:    "Server: 192.168.1.1",
			expected: "Server: [IP_REDACTED]",
		},
		{
			name:     "ssn_without_tokenizer",
			input:    "SSN: 123-45-6789",
			expected: "SSN: 123-45-6789", // Won't match without tokenizer (token-based rule)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := detector.Redact([]byte(tt.input), nil) // nil tokenizer
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

// TestHybridDetector_FalsePositivePrevention specifically tests false positive scenarios
func TestHybridDetector_FalsePositivePrevention(t *testing.T) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	tests := []struct {
		name     string
		input    string
		expected string
		reason   string
	}{
		{
			name:     "version_number_not_phone",
			input:    "Version: 555.123.4567",
			expected: "Version: [PHONE_REDACTED]", // Regex confirmation allows this (starts with 5)
			reason:   "555 is a valid area code, so this matches phone pattern",
		},
		{
			name:     "version_number_looks_like_ip",
			input:    "Version: 1.2.3.4",
			expected: "Version: [IP_REDACTED]", // This IS a valid IP pattern, so it matches
			reason:   "Matches IPv4 pattern - this is a design tradeoff (redact IPs vs allow version numbers)",
		},
		{
			name:     "order_id_not_cc",
			input:    "Order #1234567890123456",
			expected: "Order #1234567890123456",
			reason:   "Not a valid credit card BIN (doesn't start with 3,4,5,6)",
		},
		{
			name:     "timestamp_not_phone",
			input:    "Time: 012-345-6789",
			expected: "Time: 012-345-6789",
			reason:   "Starts with 0, not a valid US area code",
		},
		{
			name:     "date_not_ssn",
			input:    "Date: 2024-10-20",
			expected: "Date: 2024-10-20",
			reason:   "Wrong pattern: DDDD-DD-DD vs DDD-DD-DDDD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := detector.Redact([]byte(tt.input), tokenizer)
			assert.Equal(t, tt.expected, string(result), tt.reason)
		})
	}
}

// TestHybridDetector_ThreadSafety tests concurrent usage with different tokenizers
func TestHybridDetector_ThreadSafety(t *testing.T) {
	detector := NewHybridPIIDetector()

	// Run multiple goroutines, each with its own tokenizer
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			tokenizer := automultilinedetection.NewTokenizer(10000)
			input := []byte("SSN: 123-45-6789, Email: test@example.com")
			expected := "SSN: [SSN_REDACTED], Email: [EMAIL_REDACTED]"

			for j := 0; j < 100; j++ {
				result, rules := detector.Redact(input, tokenizer)
				require.Equal(t, expected, string(result))
				require.NotEmpty(t, rules)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// ==================== BENCHMARKS ====================

// BenchmarkHybridDetector_SSN benchmarks SSN detection
func BenchmarkHybridDetector_SSN(b *testing.B) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)
	input := []byte("User with SSN 123-45-6789 created account")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = detector.Redact(input, tokenizer)
	}
}

// BenchmarkHybridDetector_Email benchmarks email detection
func BenchmarkHybridDetector_Email(b *testing.B) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)
	input := []byte("Contact: john.doe@example.com for info")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = detector.Redact(input, tokenizer)
	}
}

// BenchmarkHybridDetector_MultiplePII benchmarks mixed PII
func BenchmarkHybridDetector_MultiplePII(b *testing.B) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)
	input := []byte("User: john@test.com, SSN: 987-65-4321, Phone: 555-123-4567, Card: 4532015112830366, IP: 10.0.1.1")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = detector.Redact(input, tokenizer)
	}
}

// BenchmarkHybridDetector_NoPII benchmarks overhead when no PII present
func BenchmarkHybridDetector_NoPII(b *testing.B) {
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)
	input := []byte("2024-10-20 15:30:00 INFO Application started successfully on port 8080")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = detector.Redact(input, tokenizer)
	}
}

// BenchmarkTokenizationOverhead measures just tokenization cost
func BenchmarkTokenizationOverhead(b *testing.B) {
	tokenizer := automultilinedetection.NewTokenizer(10000)
	input := []byte("User with SSN 123-45-6789 created account")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = tokenizer.TokenizeBytes(input)
	}
}
