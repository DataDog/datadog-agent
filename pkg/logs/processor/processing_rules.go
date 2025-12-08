// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

// tokenRules defines fast, token-based detection rules.
// These rules are applied before regex-based rules for performance.
var tokenRules = []*TokenBasedProcessingRule{
	// SSN with dashes: XXX-XX-XXXX
	{
		Name: "auto_redact_ssn",
		Type: RuleTypeToken,
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.D3),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D2),
			tokens.NewSimpleToken(tokens.Dash),
			tokens.NewSimpleToken(tokens.D4),
		},
		Replacement: []byte("[SSN_REDACTED]"),
		PrefilterKeywords: [][]byte{
			[]byte("-"), // Must contain dash to possibly match XXX-XX-XXXX
		},
	},
	// SSN with numbers only: DDDDDDDDD
	{
		Name: "auto_redact_ssn_numbers_only",
		Type: RuleTypeToken,
		TokenPattern: []tokens.Token{
			tokens.NewSimpleToken(tokens.D9),
		},
		Replacement: []byte("[SSN_REDACTED]"),
		// No prefilter needed for pure digit patterns - too common
	},
	// Example: app_key= or application_key= followed by 34 hex digits
	// This matches: (?:(app(?:lication)?_key)=[a-fA-F0-9]{34})
	// We create two rules for the two variations
	{
		Name: "auto_redact_app_key",
		Type: RuleTypeToken,
		TokenPattern: []tokens.Token{
			tokens.NewToken(tokens.C3, "app"), // Literal "app"
			tokens.NewToken(tokens.Underscore, "_"),
			tokens.NewToken(tokens.C3, "key"), // Literal "key"
			tokens.NewToken(tokens.Equal, "="),
			tokens.NewSimpleToken(tokens.C10), // First 10 chars of hex string
			tokens.NewSimpleToken(tokens.C10), // Next 10 chars
			tokens.NewSimpleToken(tokens.C10), // Next 10 chars
			tokens.NewSimpleToken(tokens.C4),  // Last 4 chars (total 34)
		},
		Replacement: []byte("[APP_KEY_REDACTED]"),
		PrefilterKeywords: [][]byte{
			[]byte("app_key="),
		},
	},
	{
		Name: "auto_redact_application_key",
		Type: RuleTypeToken,
		TokenPattern: []tokens.Token{
			tokens.NewToken(tokens.C10, "application"), // Literal "application" (actually 11 chars but C10 is max)
			tokens.NewToken(tokens.C1, "n"),            // Last char of "application"
			tokens.NewToken(tokens.Underscore, "_"),
			tokens.NewToken(tokens.C3, "key"), // Literal "key"
			tokens.NewToken(tokens.Equal, "="),
			tokens.NewSimpleToken(tokens.C10), // First 10 chars of hex string
			tokens.NewSimpleToken(tokens.C10), // Next 10 chars
			tokens.NewSimpleToken(tokens.C10), // Next 10 chars
			tokens.NewSimpleToken(tokens.C4),  // Last 4 chars (total 34)
		},
		Replacement: []byte("[APP_KEY_REDACTED]"),
		PrefilterKeywords: [][]byte{
			[]byte("application_key="),
		},
	},
}

func getTokenRules(config ProcessingRuleApplicatorConfig) []*TokenBasedProcessingRule {
	return tokenRules
}
