// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/logs/tokens"
)

// matchesTokenPattern checks if message tokens match the pattern
func matchesTokenPattern(msgTokens []tokens.Token, patternTokens []tokens.Token) bool {
	if len(patternTokens) == 0 {
		return false
	}
	if len(msgTokens) < len(patternTokens) {
		return false
	}

	// Sliding window search
	for i := 0; i <= len(msgTokens)-len(patternTokens); i++ {
		match := true
		for j := 0; j < len(patternTokens); j++ {
			if !msgTokens[i+j].Equals(patternTokens[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// hasPrefilterKeywords checks if all required keywords are present in content.
// Enables early exit for rules that can't possibly match.
func hasPrefilterKeywords(content []byte, keywords [][]byte) bool {
	if len(keywords) == 0 {
		return true
	}
	
	for _, keyword := range keywords {
		if !bytes.Contains(content, keyword) {
			return false
		}
	}
	return true
}

