// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Benchmark test messages with various PII patterns
var benchmarkMessages = []struct {
	name    string
	message string
}{
	{
		name:    "no_pii",
		message: "2024-10-20 15:30:00 INFO Application started successfully on port 8080",
	},
	{
		name:    "single_ssn",
		message: "2024-10-20 15:30:01 ERROR Failed to process SSN 123-45-6789 for user account",
	},
	{
		name:    "single_credit_card",
		message: "Payment processed for card 4532-0151-1283-0366 amount $150.00",
	},
	{
		name:    "single_phone",
		message: "Contact customer at (555) 123-4567 for verification",
	},
	{
		name:    "single_email",
		message: "User john.doe@example.com logged in successfully",
	},
	{
		name:    "single_ipv4",
		message: "Connection from 192.168.1.100 to server accepted",
	},
	{
		name:    "multiple_pii_mixed",
		message: "User john.doe@example.com with SSN 987-65-4321 called from (555) 123-4567, IP: 10.0.1.50, Card: 4532-0151-1283-0366",
	},
	{
		name:    "long_log_single_pii",
		message: "2024-10-20 15:30:45 WARN [RequestHandler] Processing request id=abc123-def456-ghi789 user=john_smith session=sess_98765432 duration=1234ms endpoint=/api/v1/users/profile status=200 bytes_sent=4096 user_agent=Mozilla/5.0 contact=(555) 123-4567 correlation_id=cor-12345",
	},
	{
		name:    "long_log_no_pii",
		message: "2024-10-20 15:30:45 INFO [DatabaseConnection] Query executed successfully query=SELECT * FROM users WHERE created_at > 2024-01-01 duration=45ms rows_affected=1234 connection_pool=primary cache_hit=true index_used=idx_created_at optimizer=cost_based partition=p2024",
	},
	{
		name:    "multiple_same_type",
		message: "Transferred from account 4532-0151-1283-0366 to account 5425-2334-3010-9903 amount $500",
	},
}

// BenchmarkPIIDetectionComparison compares regex, hybrid, and tokenization approaches
func BenchmarkPIIDetectionComparison(b *testing.B) {
	// Setup for hybrid and tokenization benchmarks
	detector := NewHybridPIIDetector()
	tokenizer := automultilinedetection.NewTokenizer(10000)

	for _, bm := range benchmarkMessages {
		b.Run(bm.name, func(b *testing.B) {
			b.Run("regex", func(b *testing.B) {
				// Current regex-based approach
				msg := &message.Message{}
				msg.SetContent([]byte(bm.message))
				rules := defaultPIIRedactionRules

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					content := msg.GetContent()
					for _, rule := range rules {
						if rule.Type == config.MaskSequences {
							if isMatchingLiteralPrefix(rule.Regex, content) {
								content = rule.Regex.ReplaceAll(content, rule.Placeholder)
							}
						}
					}
				}
			})

			b.Run("hybrid", func(b *testing.B) {
				// NEW hybrid approach: token-based pre-filtering + regex
				input := []byte(bm.message)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_, _ = detector.Redact(input, tokenizer)
				}
			})

			b.Run("tokenization_only", func(b *testing.B) {
				// Just tokenization overhead (baseline)
				input := []byte(bm.message)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_, _ = tokenizer.TokenizeBytes(input)
				}
			})
		})
	}
}

// BenchmarkRegexSingleRule benchmarks individual regex rules in isolation
func BenchmarkRegexSingleRule(b *testing.B) {
	b.Run("SSN", func(b *testing.B) {
		input := []byte("User SSN is 123-45-6789 in the system")
		rule := defaultPIIRedactionRules[2] // SSN rule

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			if isMatchingLiteralPrefix(rule.Regex, input) {
				_ = rule.Regex.ReplaceAll(input, rule.Placeholder)
			}
		}
	})

	b.Run("Email", func(b *testing.B) {
		input := []byte("Contact user@example.com for more information")
		rule := defaultPIIRedactionRules[0] // Email rule

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			if isMatchingLiteralPrefix(rule.Regex, input) {
				_ = rule.Regex.ReplaceAll(input, rule.Placeholder)
			}
		}
	})

	b.Run("CreditCard", func(b *testing.B) {
		input := []byte("Payment card 4532015112830366 was charged")
		rule := defaultPIIRedactionRules[1] // CC rule

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			if isMatchingLiteralPrefix(rule.Regex, input) {
				_ = rule.Regex.ReplaceAll(input, rule.Placeholder)
			}
		}
	})
}
