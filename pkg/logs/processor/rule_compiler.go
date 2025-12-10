// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// CompileRuleFromJSON converts a JSON-like map representation to a compiled ProcessingRule.
// This is useful for programmatically creating rules from configuration files or APIs.
//
// Expected JSON structure:
//
//	{
//	  "type": "mask_sequences" | "exclude_at_match" | "include_at_match",
//	  "name": "rule_name",
//	  "token_pattern": ["Token1", "Token2", ...],
//	  "prefilter_keywords": ["keyword1", "keyword2", ...],  // optional
//	  "length_constraints": [                                 // optional
//	    {"token_index": 0, "min_length": 1, "max_length": 3}
//	  ],
//	  "replace_placeholder": "[REDACTED]"                    // required for mask_sequences
//	}
func CompileRuleFromJSON(ruleJSON map[string]interface{}) (*config.ProcessingRule, error) {
	// Extract type (required)
	ruleType, ok := ruleJSON["type"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'type' field")
	}

	// Extract name (required)
	ruleName, ok := ruleJSON["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'name' field")
	}

	rule := &config.ProcessingRule{
		Type: ruleType,
		Name: ruleName,
	}

	// Token pattern (required)
	if tp, ok := ruleJSON["token_pattern"].([]string); ok {
		rule.TokenPatternStr = tp
	} else if tp, ok := ruleJSON["token_pattern"].([]interface{}); ok {
		// Handle []interface{} from JSON unmarshaling
		rule.TokenPatternStr = make([]string, len(tp))
		for i, token := range tp {
			if tokenStr, ok := token.(string); ok {
				rule.TokenPatternStr[i] = tokenStr
			} else {
				return nil, fmt.Errorf("invalid token at index %d in token_pattern", i)
			}
		}
	} else {
		return nil, fmt.Errorf("missing or invalid 'token_pattern' field")
	}

	// Prefilter keywords (optional)
	if pk, ok := ruleJSON["prefilter_keywords"].([]string); ok {
		rule.PrefilterKeywords = pk
	} else if pk, ok := ruleJSON["prefilter_keywords"].([]interface{}); ok {
		// Handle []interface{} from JSON unmarshaling
		rule.PrefilterKeywords = make([]string, len(pk))
		for i, keyword := range pk {
			if keywordStr, ok := keyword.(string); ok {
				rule.PrefilterKeywords[i] = keywordStr
			} else {
				return nil, fmt.Errorf("invalid keyword at index %d in prefilter_keywords", i)
			}
		}
	}

	// Replace placeholder (required for mask_sequences)
	if rp, ok := ruleJSON["replace_placeholder"].(string); ok {
		rule.ReplacePlaceholder = rp
	} else if ruleType == "mask_sequences" {
		return nil, fmt.Errorf("missing 'replace_placeholder' for mask_sequences rule")
	}

	// Length constraints (optional)
	if lc, ok := ruleJSON["length_constraints"].([]map[string]interface{}); ok {
		rule.LengthConstraints = make([]config.LengthConstraint, len(lc))
		for i, constraint := range lc {
			tokenIndex, ok1 := constraint["token_index"].(int)
			minLength, ok2 := constraint["min_length"].(int)
			maxLength, ok3 := constraint["max_length"].(int)

			if !ok1 || !ok2 || !ok3 {
				return nil, fmt.Errorf("invalid length constraint at index %d", i)
			}

			rule.LengthConstraints[i] = config.LengthConstraint{
				TokenIndex: tokenIndex,
				MinLength:  minLength,
				MaxLength:  maxLength,
			}
		}
	} else if lc, ok := ruleJSON["length_constraints"].([]interface{}); ok {
		// Handle []interface{} from JSON unmarshaling
		rule.LengthConstraints = make([]config.LengthConstraint, len(lc))
		for i, constraintInterface := range lc {
			constraint, ok := constraintInterface.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid length constraint at index %d", i)
			}

			// Try both int and float64 (JSON numbers unmarshal to float64)
			tokenIndex := getIntFromInterface(constraint["token_index"])
			minLength := getIntFromInterface(constraint["min_length"])
			maxLength := getIntFromInterface(constraint["max_length"])

			if tokenIndex < 0 || minLength < 0 || maxLength < 0 {
				return nil, fmt.Errorf("invalid length constraint values at index %d", i)
			}

			rule.LengthConstraints[i] = config.LengthConstraint{
				TokenIndex: tokenIndex,
				MinLength:  minLength,
				MaxLength:  maxLength,
			}
		}
	}

	// Compile the rule (converts TokenPatternStr to TokenPattern, etc.)
	if err := config.CompileProcessingRules([]*config.ProcessingRule{rule}); err != nil {
		return nil, fmt.Errorf("failed to compile rule '%s': %w", ruleName, err)
	}

	return rule, nil
}

// getIntFromInterface extracts an int from interface{} that could be int, int64, or float64
func getIntFromInterface(val interface{}) int {
	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return -1
	}
}
