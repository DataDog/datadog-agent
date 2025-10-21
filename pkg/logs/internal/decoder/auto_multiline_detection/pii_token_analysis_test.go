// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"fmt"
	"testing"
)

// TestAnalyzePIITokenPatterns analyzes what PII data looks like when tokenized
// This helps us understand if token-based detection could be viable
func TestAnalyzePIITokenPatterns(t *testing.T) {
	tokenizer := NewTokenizer(1000)

	testCases := []struct {
		name     string
		examples []string
	}{
		{
			name: "emails",
			examples: []string{
				"user@example.com",
				"john.doe@company.co.uk",
				"test_user+tag@subdomain.example.com",
				"a@b.co",
				"very.long.email.address@some-company-name.com",
			},
		},
		{
			name: "credit_cards",
			examples: []string{
				"4532015112830366",    // Visa
				"5425233430109903",    // Mastercard
				"374245455400126",     // Amex
				"6011000991300009",    // Discover
				"4532-0151-1283-0366", // Visa with dashes
				"4532 0151 1283 0366", // Visa with spaces
			},
		},
		{
			name: "ssn",
			examples: []string{
				"123-45-6789",
				"987-65-4321",
				"111-22-3333",
			},
		},
		{
			name: "phone_numbers",
			examples: []string{
				"555-123-4567",
				"(555) 123-4567",
				"+1-555-123-4567",
				"555.123.4567",
				"1 555 123 4567",
				"5551234567",
			},
		},
		{
			name: "ipv4",
			examples: []string{
				"192.168.1.1",
				"10.0.0.1",
				"172.16.254.1",
				"8.8.8.8",
				"255.255.255.255",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			fmt.Printf("\n=== %s ===\n", tc.name)
			for _, example := range tc.examples {
				toks, _ := tokenizer.tokenize([]byte(example))
				fmt.Printf("%-40s -> %s\n", example, tokensToString(toks))
			}
		})
	}
}
