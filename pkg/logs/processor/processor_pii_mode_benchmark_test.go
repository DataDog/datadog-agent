// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// BenchmarkProcessorPIIModes benchmarks the processor's PII redaction
// in both regex and hybrid modes. This tests the actual integrated
// processor behavior using a direct approach to avoid config overhead.
func BenchmarkProcessorPIIModes(b *testing.B) {
	// Test messages with various PII patterns
	benchmarkMessages := []struct {
		name    string
		message []byte
	}{
		{
			name:    "no_pii",
			message: []byte("2024-10-20 15:30:00 INFO Application started successfully on port 8080"),
		},
		{
			name:    "single_ssn",
			message: []byte("2024-10-20 15:30:01 ERROR Failed to process SSN 123-45-6789 for user account"),
		},
		{
			name:    "single_credit_card",
			message: []byte("Payment processed for card 4532-0151-1283-0366 amount $150.00"),
		},
		{
			name:    "single_phone",
			message: []byte("Contact customer at (555) 123-4567 for verification"),
		},
		{
			name:    "single_email",
			message: []byte("User john.doe@example.com logged in successfully"),
		},
		{
			name:    "single_ipv4",
			message: []byte("Connection from 192.168.1.100 to server accepted"),
		},
		{
			name:    "multiple_pii_mixed",
			message: []byte("User john.doe@example.com with SSN 987-65-4321 called from (555) 123-4567, IP: 10.0.1.50, Card: 4532-0151-1283-0366"),
		},
		{
			name:    "long_log_single_pii",
			message: []byte("2024-10-20 15:30:45 WARN [RequestHandler] Processing request id=abc123-def456-ghi789 user=john_smith session=sess_98765432 duration=1234ms endpoint=/api/v1/users/profile status=200 bytes_sent=4096 user_agent=Mozilla/5.0 contact=(555) 123-4567 correlation_id=cor-12345"),
		},
		{
			name:    "long_log_no_pii",
			message: []byte("2024-10-20 15:30:45 INFO [DatabaseConnection] Query executed successfully query=SELECT * FROM users WHERE created_at > 2024-01-01 duration=45ms rows_affected=1234 connection_pool=primary cache_hit=true index_used=idx_created_at optimizer=cost_based partition=p2024"),
		},
		{
			name:    "multiple_same_type",
			message: []byte("Transferred from account 4532-0151-1283-0366 to account 5425-2334-3010-9903 amount $500"),
		},
	}

	for _, bm := range benchmarkMessages {
		b.Run(bm.name, func(b *testing.B) {
			// Benchmark regex mode
			b.Run("regex", func(b *testing.B) {
				// Setup BEFORE timer
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactPII, true, model.SourceAgentRuntime)
				mockConfig.Set(configPIIRedactionMode, PIIRedactionModeRegex, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
					piiDetector:  NewHybridPIIDetector(),
					piiTokenizer: newTokenizer(),
				}

				source := sources.NewLogSource("", &config.LogsConfig{})
				msg := newMessage(nil, source, "")

				// Reset timer AFTER all setup is complete
				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					msg.SetContent(bm.message)
					processor.applyRedactingRules(msg)
				}
			})

			// Benchmark hybrid mode
			b.Run("hybrid", func(b *testing.B) {
				// Setup BEFORE timer
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactPII, true, model.SourceAgentRuntime)
				mockConfig.Set(configPIIRedactionMode, PIIRedactionModeHybrid, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
					piiDetector:  NewHybridPIIDetector(),
					piiTokenizer: newTokenizer(),
				}

				source := sources.NewLogSource("", &config.LogsConfig{})
				msg := newMessage(nil, source, "")

				// Reset timer AFTER all setup is complete
				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					msg.SetContent(bm.message)
					processor.applyRedactingRules(msg)
				}
			})

			// Benchmark with PII disabled (baseline)
			b.Run("disabled", func(b *testing.B) {
				// Setup BEFORE timer
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactPII, false, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
					piiDetector:  NewHybridPIIDetector(),
					piiTokenizer: newTokenizer(),
				}

				source := sources.NewLogSource("", &config.LogsConfig{})
				msg := newMessage(nil, source, "")

				// Reset timer AFTER all setup is complete
				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					msg.SetContent(bm.message)
					processor.applyRedactingRules(msg)
				}
			})
		})
	}
}

// BenchmarkProcessorPIIModesFullPipeline benchmarks the full processor pipeline
// including message rendering and encoding (closer to real-world usage)
func BenchmarkProcessorPIIModesFullMessage(b *testing.B) {
	testMessages := []struct {
		name    string
		message []byte
	}{
		{
			name:    "no_pii",
			message: []byte("2024-10-20 15:30:00 INFO Application started successfully"),
		},
		{
			name:    "multiple_pii",
			message: []byte("User john@test.com SSN 123-45-6789 from IP 10.0.0.1 card 4532-0151-1283-0366"),
		},
	}

	for _, tm := range testMessages {
		b.Run(tm.name, func(b *testing.B) {
			b.Run("regex", func(b *testing.B) {
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactPII, true, model.SourceAgentRuntime)
				mockConfig.Set(configPIIRedactionMode, PIIRedactionModeRegex, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
					piiDetector:  NewHybridPIIDetector(),
					piiTokenizer: newTokenizer(),
				}

				source := sources.NewLogSource("", &config.LogsConfig{})

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					msg := newMessage(tm.message, source, "")
					processor.applyRedactingRules(msg)
				}
			})

			b.Run("hybrid", func(b *testing.B) {
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactPII, true, model.SourceAgentRuntime)
				mockConfig.Set(configPIIRedactionMode, PIIRedactionModeHybrid, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
					piiDetector:  NewHybridPIIDetector(),
					piiTokenizer: newTokenizer(),
				}

				source := sources.NewLogSource("", &config.LogsConfig{})

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					msg := newMessage(tm.message, source, "")
					processor.applyRedactingRules(msg)
				}
			})

			b.Run("disabled", func(b *testing.B) {
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactPII, false, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
					piiDetector:  NewHybridPIIDetector(),
					piiTokenizer: newTokenizer(),
				}

				source := sources.NewLogSource("", &config.LogsConfig{})

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					msg := newMessage(tm.message, source, "")
					processor.applyRedactingRules(msg)
				}
			})
		})
	}
}
