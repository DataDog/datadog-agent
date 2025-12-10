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
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Benchmark test cases with realistic token rules
var benchmarkTokenRules = []struct {
	name     string
	ruleJSON map[string]interface{}
}{
	{
		name: "SSN",
		ruleJSON: map[string]interface{}{
			"type":                "mask_sequences",
			"name":                "mask_ssn",
			"token_pattern":       []string{"D3", "Dash", "D2", "Dash", "D4"},
			"prefilter_keywords":  []string{"-"},
			"replace_placeholder": "[SSN_REDACTED]",
		},
	},
	{
		name: "Email",
		ruleJSON: map[string]interface{}{
			"type":               "mask_sequences",
			"name":               "mask_email",
			"token_pattern":      []string{"CAny", "At", "CAny", "Period", "CAny"},
			"prefilter_keywords": []string{"@"},
			"length_constraints": []map[string]interface{}{
				{"token_index": 0, "min_length": 1, "max_length": 64},
				{"token_index": 2, "min_length": 1, "max_length": 253},
				{"token_index": 4, "min_length": 2, "max_length": 63},
			},
			"replace_placeholder": "[EMAIL_REDACTED]",
		},
	},
	{
		name: "IPv4",
		ruleJSON: map[string]interface{}{
			"type":               "mask_sequences",
			"name":               "mask_ipv4",
			"token_pattern":      []string{"DAny", "Period", "DAny", "Period", "DAny", "Period", "DAny"},
			"prefilter_keywords": []string{"."},
			"length_constraints": []map[string]interface{}{
				{"token_index": 0, "min_length": 1, "max_length": 3},
				{"token_index": 2, "min_length": 1, "max_length": 3},
				{"token_index": 4, "min_length": 1, "max_length": 3},
				{"token_index": 6, "min_length": 1, "max_length": 3},
			},
			"replace_placeholder": "[IP_REDACTED]",
		},
	},
	{
		name: "CreditCard",
		ruleJSON: map[string]interface{}{
			"type":                "mask_sequences",
			"name":                "mask_credit_card",
			"token_pattern":       []string{"D4", "Dash", "D4", "Dash", "D4", "Dash", "D4"},
			"prefilter_keywords":  []string{"-"},
			"replace_placeholder": "[CREDIT_CARD_REDACTED]",
		},
	},
	{
		name: "APIKey",
		ruleJSON: map[string]interface{}{
			"type":               "mask_sequences",
			"name":               "mask_api_key",
			"token_pattern":      []string{"C3", "Underscore", "C3", "Equal", "CAny"},
			"prefilter_keywords": []string{"api_key="},
			"length_constraints": []map[string]interface{}{
				{"token_index": 4, "min_length": 26, "max_length": 64},
			},
			"replace_placeholder": "api_key=**************************",
		},
	},
	{
		name: "ExcludeHealthcheck",
		ruleJSON: map[string]interface{}{
			"type":               "exclude_at_match",
			"name":               "exclude_healthcheck",
			"token_pattern":      []string{"Fslash", "C6"},
			"prefilter_keywords": []string{"/health"},
		},
	},
	{
		name: "ExcludeIptables",
		ruleJSON: map[string]interface{}{
			"type":               "exclude_at_match",
			"name":               "exclude_iptables",
			"token_pattern":      []string{"C7", "Space", "C8", "Space", "C5"},
			"prefilter_keywords": []string{"iptables"},
		},
	},
	{
		name: "IncludeError",
		ruleJSON: map[string]interface{}{
			"type":               "include_at_match",
			"name":               "include_error",
			"token_pattern":      []string{"C5"},
			"prefilter_keywords": []string{"ERROR"},
		},
	},
	// Real-world production rules
	{
		name: "MaskAPIKeys",
		ruleJSON: map[string]interface{}{
			"type":               "mask_sequences",
			"name":               "mask_api_keys",
			"token_pattern":      []string{"C3", "Underscore", "C3", "Equal", "CAny"},
			"prefilter_keywords": []string{"api_key="},
			"length_constraints": []map[string]interface{}{
				{"token_index": 4, "min_length": 26, "max_length": 26},
			},
			"replace_placeholder": "api_key=**************************",
		},
	},
	{
		name: "MaskReservedWordHostname",
		ruleJSON: map[string]interface{}{
			"type":                "mask_sequences",
			"name":                "mask_reserved_word_hostname",
			"token_pattern":       []string{"C8"},
			"prefilter_keywords":  []string{"hostname"},
			"replace_placeholder": "mhostname",
		},
	},
	{
		name: "ExcludeTranscriptInsightsWarning",
		ruleJSON: map[string]interface{}{
			"type":               "exclude_at_match",
			"name":               "exclude_transcript_insights_warning_multiline",
			"token_pattern":      []string{"C7", "Colon", "C4", "Colon", "C10", "Space", "C8", "Space", "C6", "Space", "C2"},
			"prefilter_keywords": []string{"WARNING:root:"},
		},
	},
	{
		name: "IncludeSSHD",
		ruleJSON: map[string]interface{}{
			"type":               "include_at_match",
			"name":               "include_sshd",
			"token_pattern":      []string{"C4"},
			"prefilter_keywords": []string{"sshd"},
		},
	},
	{
		name: "IncludeSnoopy",
		ruleJSON: map[string]interface{}{
			"type":               "include_at_match",
			"name":               "include_snoopy",
			"token_pattern":      []string{"C6"},
			"prefilter_keywords": []string{"snoopy"},
		},
	},
	{
		name: "K8sMLPing",
		ruleJSON: map[string]interface{}{
			"type":               "exclude_at_match",
			"name":               "k8s_filter2_ping",
			"token_pattern":      []string{"Fslash", "C2", "Fslash", "C4"},
			"prefilter_keywords": []string{"/ml/ping"},
		},
	},
	{
		name: "K8sMLPredict",
		ruleJSON: map[string]interface{}{
			"type":               "exclude_at_match",
			"name":               "k8s_filter2_predict",
			"token_pattern":      []string{"Fslash", "C2", "Fslash", "C7"},
			"prefilter_keywords": []string{"/ml/predict"},
		},
	},
	{
		name: "Rails20XOK",
		ruleJSON: map[string]interface{}{
			"type":               "exclude_at_match",
			"name":               "rails_20X_ok",
			"token_pattern":      []string{"C9", "Space", "D3", "Space", "CAny", "Space", "C2", "Space"},
			"prefilter_keywords": []string{"Completed"},
			"length_constraints": []map[string]interface{}{
				{"token_index": 2, "min_length": 3, "max_length": 3}, // Digit must be exactly 3 chars (200-202)
			},
		},
	},
}

// Benchmark log messages for different scenarios
var benchmarkLogs = map[string]string{
	// Matching logs
	"ssn_match":         "User SSN: 123-45-6789 was verified",
	"email_match":       "Contact user@example.com for details",
	"ipv4_match":        "Request from 192.168.1.100 received",
	"credit_card_match": "Payment via 1234-5678-9012-3456 processed",
	"api_key_match":     "Authorization: api_key=abcdefghijklmnopqrstuvwxyz1234567890",
	"healthcheck_match": "GET /health HTTP/1.1",
	"iptables_match":    "Syncing iptables rules completed",
	"error_match":       "ERROR in processing",

	// Non-matching logs (prefilter hit, pattern miss)
	"ssn_prefilter_hit":     "User ID: 123456789 - no dashes",
	"email_prefilter_hit":   "Email format @ invalid",
	"ipv4_prefilter_hit":    "Version 1.2.3.4.5.6.7 found",
	"api_key_prefilter_hit": "api_key=short",

	// Non-matching logs (prefilter miss - early exit)
	"no_prefilter_match_1": "This is a normal log message with no sensitive data",
	"no_prefilter_match_2": "Processing request ID 9876543210 completed successfully",
	"no_prefilter_match_3": "Application startup sequence initiated",
	"no_prefilter_match_4": "Cache warmup finished in 1234ms",

	// Real-world production patterns
	"api_key_real_match":        "api_key=abcdefghijklmnopqrstuvwxyz",
	"hostname_match":            "Server hostname changed",
	"warning_root_match":        "WARNING:root:Transcript insights failed on device",
	"sshd_match":                "sshd[1234]: Connection from 192.168.1.100",
	"snoopy_match":              "snoopy[5678]: User logged in successfully",
	"k8s_ml_ping_match":         "GET /ml/ping HTTP/1.1 200",
	"k8s_ml_predict_match":      "POST /ml/predict with payload",
	"rails_20x_match":           "Completed 200 OK in 45ms",
	"rails_201_match":           "Completed 201 Created in 120ms",
	"no_real_world_prefilter_1": "Standard application log without sensitive patterns",
	"no_real_world_prefilter_2": "Request processed with response code 200",
}

// BenchmarkProcessing_PrefilterHit_TokenMatch benchmarks logs that pass prefilter and match tokens
func BenchmarkProcessing_PrefilterHit_TokenMatch(b *testing.B) {
	testCases := []struct {
		name   string
		rule   string
		logKey string
	}{
		{"SSN", "SSN", "ssn_match"},
		{"Email", "Email", "email_match"},
		{"IPv4", "IPv4", "ipv4_match"},
		{"CreditCard", "CreditCard", "credit_card_match"},
		{"APIKey", "APIKey", "api_key_match"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup
			var ruleJSON map[string]interface{}
			for _, r := range benchmarkTokenRules {
				if r.name == tc.rule {
					ruleJSON = r.ruleJSON
					break
				}
			}
			rule, _ := CompileRuleFromJSON(ruleJSON)
			processor := setupBenchmarkProcessor([]*config.ProcessingRule{rule})
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_PrefilterHit_TokenMiss benchmarks logs that pass prefilter but fail token match
func BenchmarkProcessing_PrefilterHit_TokenMiss(b *testing.B) {
	testCases := []struct {
		name   string
		rule   string
		logKey string
	}{
		{"SSN", "SSN", "ssn_prefilter_hit"},
		{"Email", "Email", "email_prefilter_hit"},
		{"IPv4", "IPv4", "ipv4_prefilter_hit"},
		{"APIKey", "APIKey", "api_key_prefilter_hit"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup
			var ruleJSON map[string]interface{}
			for _, r := range benchmarkTokenRules {
				if r.name == tc.rule {
					ruleJSON = r.ruleJSON
					break
				}
			}
			rule, _ := CompileRuleFromJSON(ruleJSON)
			processor := setupBenchmarkProcessor([]*config.ProcessingRule{rule})
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_PrefilterMiss benchmarks logs that fail prefilter (early exit)
func BenchmarkProcessing_PrefilterMiss(b *testing.B) {
	testCases := []struct {
		name   string
		logKey string
	}{
		{"NormalLog1", "no_prefilter_match_1"},
		{"NormalLog2", "no_prefilter_match_2"},
		{"NormalLog3", "no_prefilter_match_3"},
		{"NormalLog4", "no_prefilter_match_4"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup with all mask rules (most common case)
			rules := make([]*config.ProcessingRule, 0)
			for _, r := range benchmarkTokenRules {
				if ruleType, ok := r.ruleJSON["type"].(string); ok && ruleType == "mask_sequences" {
					rule, _ := CompileRuleFromJSON(r.ruleJSON)
					rules = append(rules, rule)
				}
			}
			processor := setupBenchmarkProcessor(rules)
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_MultipleRules benchmarks processing with multiple rules (realistic scenario)
func BenchmarkProcessing_MultipleRules(b *testing.B) {
	// Compile all rules
	rules := make([]*config.ProcessingRule, 0, len(benchmarkTokenRules))
	for _, r := range benchmarkTokenRules {
		rule, _ := CompileRuleFromJSON(r.ruleJSON)
		rules = append(rules, rule)
	}
	processor := setupBenchmarkProcessor(rules)

	testCases := []struct {
		name   string
		logKey string
	}{
		{"SSN_Match", "ssn_match"},
		{"Email_Match", "email_match"},
		{"PrefilterMiss", "no_prefilter_match_1"},
		{"PrefilterHit_TokenMiss", "ssn_prefilter_hit"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			logContent := benchmarkLogs[tc.logKey]
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_ExcludeRules benchmarks exclude rules
func BenchmarkProcessing_ExcludeRules(b *testing.B) {
	testCases := []struct {
		name   string
		rule   string
		logKey string
	}{
		{"Healthcheck_Match", "ExcludeHealthcheck", "healthcheck_match"},
		{"Iptables_Match", "ExcludeIptables", "iptables_match"},
		{"NoMatch", "ExcludeHealthcheck", "no_prefilter_match_1"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup
			var ruleJSON map[string]interface{}
			for _, r := range benchmarkTokenRules {
				if r.name == tc.rule {
					ruleJSON = r.ruleJSON
					break
				}
			}
			rule, _ := CompileRuleFromJSON(ruleJSON)
			processor := setupBenchmarkProcessor([]*config.ProcessingRule{rule})
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_IncludeRules benchmarks include rules
func BenchmarkProcessing_IncludeRules(b *testing.B) {
	testCases := []struct {
		name   string
		logKey string
	}{
		{"Match", "error_match"},
		{"NoMatch", "no_prefilter_match_1"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup
			var ruleJSON map[string]interface{}
			for _, r := range benchmarkTokenRules {
				if r.name == "IncludeError" {
					ruleJSON = r.ruleJSON
					break
				}
			}
			rule, _ := CompileRuleFromJSON(ruleJSON)
			processor := setupBenchmarkProcessor([]*config.ProcessingRule{rule})
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_LengthConstraints benchmarks rules with length constraints
func BenchmarkProcessing_LengthConstraints(b *testing.B) {
	testCases := []struct {
		name   string
		rule   string
		logKey string
	}{
		{"IPv4_Match", "IPv4", "ipv4_match"},
		{"Email_Match", "Email", "email_match"},
		{"APIKey_Match", "APIKey", "api_key_match"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup
			var ruleJSON map[string]interface{}
			for _, r := range benchmarkTokenRules {
				if r.name == tc.rule {
					ruleJSON = r.ruleJSON
					break
				}
			}
			rule, _ := CompileRuleFromJSON(ruleJSON)
			processor := setupBenchmarkProcessor([]*config.ProcessingRule{rule})
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_RealWorldRules benchmarks real-world production rules
func BenchmarkProcessing_RealWorldRules(b *testing.B) {
	testCases := []struct {
		name   string
		rule   string
		logKey string
	}{
		// Mask rules
		{"MaskAPIKeys_Match", "MaskAPIKeys", "api_key_real_match"},
		{"MaskHostname_Match", "MaskReservedWordHostname", "hostname_match"},

		// Exclude rules
		{"ExcludeWarning_Match", "ExcludeTranscriptInsightsWarning", "warning_root_match"},
		{"K8sMLPing_Match", "K8sMLPing", "k8s_ml_ping_match"},
		{"K8sMLPredict_Match", "K8sMLPredict", "k8s_ml_predict_match"},
		{"Rails20X_Match", "Rails20XOK", "rails_20x_match"},
		{"Rails201_Match", "Rails20XOK", "rails_201_match"},

		// Include rules
		{"IncludeSSHD_Match", "IncludeSSHD", "sshd_match"},
		{"IncludeSnoopy_Match", "IncludeSnoopy", "snoopy_match"},

		// Prefilter misses
		{"RealWorld_PrefilterMiss1", "MaskAPIKeys", "no_real_world_prefilter_1"},
		{"RealWorld_PrefilterMiss2", "IncludeSSHD", "no_real_world_prefilter_2"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Setup
			var ruleJSON map[string]interface{}
			for _, r := range benchmarkTokenRules {
				if r.name == tc.rule {
					ruleJSON = r.ruleJSON
					break
				}
			}
			rule, _ := CompileRuleFromJSON(ruleJSON)
			processor := setupBenchmarkProcessor([]*config.ProcessingRule{rule})
			logContent := benchmarkLogs[tc.logKey]

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// BenchmarkProcessing_AllRealWorldRules benchmarks processing with all real-world rules (realistic production scenario)
func BenchmarkProcessing_AllRealWorldRules(b *testing.B) {
	// Compile all real-world rules
	realWorldRuleNames := []string{
		"MaskAPIKeys", "MaskReservedWordHostname", "ExcludeTranscriptInsightsWarning",
		"IncludeSSHD", "IncludeSnoopy", "K8sMLPing", "K8sMLPredict", "Rails20XOK",
	}

	rules := make([]*config.ProcessingRule, 0)
	for _, ruleName := range realWorldRuleNames {
		for _, r := range benchmarkTokenRules {
			if r.name == ruleName {
				rule, _ := CompileRuleFromJSON(r.ruleJSON)
				rules = append(rules, rule)
				break
			}
		}
	}
	processor := setupBenchmarkProcessor(rules)

	testCases := []struct {
		name   string
		logKey string
	}{
		{"APIKey_Match", "api_key_real_match"},
		{"Hostname_Match", "hostname_match"},
		{"WarningRoot_Match", "warning_root_match"},
		{"SSHD_Match", "sshd_match"},
		{"K8sMLPing_Match", "k8s_ml_ping_match"},
		{"Rails20X_Match", "rails_20x_match"},
		{"PrefilterMiss", "no_real_world_prefilter_1"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			logContent := benchmarkLogs[tc.logKey]
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := createBenchmarkMessage(logContent)
				processor.applyRedactingRules(msg)
			}
		})
	}
}

// Helper functions

// setupBenchmarkProcessor creates a processor with the given rules
func setupBenchmarkProcessor(rules []*config.ProcessingRule) *Processor {
	processor := &Processor{
		processingRules:       rules,
		tokenizerWithLiterals: automultilinedetection.NewTokenizerWithLiterals(1000),
	}
	processor.Start()
	return processor
}

// createBenchmarkMessage creates a message for benchmarking
func createBenchmarkMessage(content string) *message.Message {
	source := sources.NewLogSource("benchmark", &config.LogsConfig{})
	return message.NewMessageWithSource([]byte(content), message.StatusInfo, source, 0)
}
