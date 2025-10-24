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
// enabled vs disabled for quick performance comparison
func BenchmarkProcessorPIIModes(b *testing.B) {
	// Simplified test cases for quick benchmarking
	benchmarkMessages := []struct {
		name    string
		message []byte
	}{
		{
			name:    "no_pii",
			message: []byte("2024-10-20 15:30:00 INFO Application started successfully on port 8080"),
		},
		{
			name:    "single_pii",
			message: []byte("User john.doe@example.com with SSN 123-45-6789 logged in"),
		},
		{
			name:    "multiple_pii",
			message: []byte("User john.doe@example.com with SSN 987-65-4321 called from (555) 123-4567, IP: 10.0.1.50, Card: 4532-0151-1283-0366"),
		},
	}

	for _, bm := range benchmarkMessages {
		b.Run(bm.name, func(b *testing.B) {
			// Benchmark with PII redaction enabled
			b.Run("enabled", func(b *testing.B) {
				// Setup BEFORE timer
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactEnabled, true, model.SourceAgentRuntime)
				mockConfig.Set(configAutoRedactEmail, true, model.SourceAgentRuntime)
				mockConfig.Set(configAutoRedactCreditCard, true, model.SourceAgentRuntime)
				mockConfig.Set(configAutoRedactSSN, true, model.SourceAgentRuntime)
				mockConfig.Set(configAutoRedactPhone, true, model.SourceAgentRuntime)
				mockConfig.Set(configAutoRedactIP, true, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
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

			// Benchmark with PII redaction disabled (baseline)
			b.Run("disabled", func(b *testing.B) {
				// Setup BEFORE timer
				mockConfig := pkgconfigmock.New(b)
				mockConfig.Set(configAutoRedactEnabled, false, model.SourceAgentRuntime)

				processor := &Processor{
					config:       mockConfig,
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
