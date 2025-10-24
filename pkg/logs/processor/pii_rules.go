// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

// getSSNRules returns all SSN (Social Security Number) detection rules
func getSSNRules() []*PIIDetectionRule {
	return []*PIIDetectionRule{
		// SSN with dashes: DDD-DD-DDDD (unambiguous pattern, no confirmation needed)
		{
			Name: "auto_redact_ssn",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Dash, tokens.D2, tokens.Dash, tokens.D4,
			},
			Replacement: []byte("[SSN_REDACTED]"),
		},
		// SSN with numbers only: DDDDDDDDD
		{
			Name: "auto_redact_ssn_numbers_only",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D9,
			},
			Replacement: []byte("[SSN_REDACTED]"),
		},
		// SSN with spaces: DDD DD DDDD
		{
			Name: "auto_redact_ssn_spaces",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Space, tokens.D2, tokens.Space, tokens.D4,
			},
			Replacement: []byte("[SSN_REDACTED]"),
		},
	}
}

// getCreditCardRules returns all credit card detection rules
func getCreditCardRules() []*PIIDetectionRule {
	return []*PIIDetectionRule{
		// Credit Card with dashes: DDDD-DDDD-DDDD-DDDD (Visa, Mastercard, Discover)
		{
			Name: "auto_redact_credit_card_dashed",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D4, tokens.Dash, tokens.D4, tokens.Dash, tokens.D4, tokens.Dash, tokens.D4,
			},
			Replacement: []byte("[CC_REDACTED]"),
		},
		// Credit Card with spaces: DDDD DDDD DDDD DDDD (Visa, Mastercard, Discover)
		{
			Name: "auto_redact_credit_card_spaced",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D4, tokens.Space, tokens.D4, tokens.Space, tokens.D4, tokens.Space, tokens.D4,
			},
			Replacement: []byte("[CC_REDACTED]"),
		},
		// American Express with dashes: DDDD-DDDDDD-DDDDD
		{
			Name: "auto_redact_credit_card_amex_dashed",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D4, tokens.Dash, tokens.D6, tokens.Dash, tokens.D5,
			},
			Replacement: []byte("[CC_REDACTED]"),
		},
		// American Express with spaces: DDDD DDDDDD DDDDD
		{
			Name: "auto_redact_credit_card_amex_spaced",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D4, tokens.Space, tokens.D6, tokens.Space, tokens.D5,
			},
			Replacement: []byte("[CC_REDACTED]"),
		},
	}
}

// getPhoneRules returns all phone number detection rules
func getPhoneRules() []*PIIDetectionRule {
	return []*PIIDetectionRule{
		// Phone with parentheses: (DDD) DDD-DDDD
		// Regex confirms: area code starts with 2-9 (not 0 or 1, which would be invalid)
		{
			Name: "auto_redact_phone_parens",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.Parenopen, tokens.D3, tokens.Parenclose, tokens.Space, tokens.D3, tokens.Dash, tokens.D4,
			},
			RegexConfirmation: regexp.MustCompile(`^\([2-9][0-9]{2}\) [0-9]{3}-[0-9]{4}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
		// Phone with dashes: DDD-DDD-DDDD
		// Regex confirms: this is a phone (area code 2-9), not a timestamp (would start with 0-1)
		{
			Name: "auto_redact_phone_dashed",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Dash, tokens.D3, tokens.Dash, tokens.D4,
			},
			RegexConfirmation: regexp.MustCompile(`^[2-9][0-9]{2}-[0-9]{3}-[0-9]{4}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
		// Phone with dots: DDD.DDD.DDDD
		// Regex confirms: phone, not version number or partial IP
		{
			Name: "auto_redact_phone_dotted",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D3, tokens.Period, tokens.D3, tokens.Period, tokens.D4,
			},
			RegexConfirmation: regexp.MustCompile(`^[2-9][0-9]{2}\.[0-9]{3}\.[0-9]{4}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
		// Phone unformatted: D10 = 10 consecutive digits
		// Regex confirms: area code starts with 2-9 (not 0 or 1)
		{
			Name: "auto_redact_phone_unformatted",
			Type: PIIRuleTypeToken,
			TokenPattern: []tokens.Token{
				tokens.D10,
			},
			RegexConfirmation: regexp.MustCompile(`^[2-9][0-9]{9}$`),
			Replacement:       []byte("[PHONE_REDACTED]"),
		},
	}
}

// getEmailRules returns all email detection rules
func getEmailRules() []*PIIDetectionRule {
	return []*PIIDetectionRule{
		// Email: too variable for token matching (many TLD lengths, subdomain variations)
		{
			Name:        "auto_redact_email",
			Type:        PIIRuleTypeRegex,
			Regex:       regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
			Replacement: []byte("[EMAIL_REDACTED]"),
		},
	}
}

// getIPRules returns all IP address detection rules
func getIPRules() []*PIIDetectionRule {
	return []*PIIDetectionRule{
		// IPv4: variable digit lengths (D.D.D.D to DDD.DDD.DDD.DDD), hard for tokens
		{
			Name:        "auto_redact_ipv4",
			Type:        PIIRuleTypeRegex,
			Regex:       regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
			Replacement: []byte("[IP_REDACTED]"),
		},
	}
}
