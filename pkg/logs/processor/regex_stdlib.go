// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !re2_cgo

package processor

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// re2MatchContent checks whether content matches the rule's pattern.
// Uses bytes.Contains for literal patterns, stdlib regex otherwise.
func re2MatchContent(rule *config.ProcessingRule, content []byte) bool {
	if rule.HasLiteralContents() {
		return matchLiterals(rule, content)
	}
	return rule.Regex.Match(content)
}

// re2MaskReplace performs a global regex replacement on content using
// the rule's compiled pattern and placeholder. It uses the literal-prefix
// guard as a fast reject, then falls back to replaceAllLazy with Go's
// stdlib regexp engine.
func re2MaskReplace(rule *config.ProcessingRule, content []byte) ([]byte, bool) {
	if !isMatchingLiteralPrefix(rule.Regex, content) {
		return content, false
	}
	return replaceAllLazy(rule.Regex, content, rule.Placeholder, rule.PlaceholderHasExpansion())
}
