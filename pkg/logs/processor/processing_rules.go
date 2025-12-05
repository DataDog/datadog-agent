// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

var tokenRules = []*TokenBasedProcessingRule{
	{
		Name: "auto_redact_ssn",
		Type: RuleTypeToken,
		TokenPattern: []tokens.Token{
			tokens.D3, tokens.Dash, tokens.D2, tokens.Dash, tokens.D4,
		},
		Replacement: []byte("[SSN_REDACTED]"),
	},
	// SSN with numbers only: DDDDDDDDD
	{
		Name: "auto_redact_ssn_numbers_only",
		Type: RuleTypeToken,
		TokenPattern: []tokens.Token{
			tokens.D9,
		},
		Replacement: []byte("[SSN_REDACTED]"),
	},
}

func getTokenRules(config ProcessingRuleApplicatorConfig) []*TokenBasedProcessingRule {
	return tokenRules
}
