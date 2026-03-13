// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build re2_cgo

package processor

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	re2 "github.com/DataDog/datadog-agent/pkg/logs/re2"
)

// re2MatchContent checks whether content matches the rule's pattern.
// Pure-literal patterns use bytes.Contains via matchLiterals.
// For regex patterns, it uses go-re2 when the DFA advantage outweighs
// CGo overhead: complex patterns (no literal prefix) always route to
// RE2, while simple patterns only use RE2 on large content.
func re2MatchContent(rule *config.ProcessingRule, content []byte) bool {
	if rule.HasLiteralContents() {
		return matchLiterals(rule, content)
	}

	compiled := rule.RE2Compiled()
	if compiled == nil {
		return rule.Regex.Match(content)
	}

	if rule.HasLiteralPrefix() && len(content) <= re2CGoThreshold {
		return rule.Regex.Match(content)
	}

	return re2.Match(compiled, content)
}

// re2CGoThreshold is the content-size below which simple (literal-prefix)
// patterns use stdlib instead of CGo. For short content the regex work is
// trivial, so the CGo crossing overhead outweighs RE2's DFA advantage.
// Complex patterns (no literal prefix) always use go-re2 because the DFA
// wins at every size.
const re2CGoThreshold = 4096

// re2MaskReplace performs a global regex replacement using go-re2.
// Uses FindAllIndex / FindAllSubmatchIndex to locate matches, then
// assembles the result in Go — no C-side string copies. Returns
// (content, false) with zero allocation on miss.
//
// The literal-prefix guard is applied internally as a fast reject.
// For simple patterns on short content, it falls back to stdlib to
// avoid CGo overhead.
func re2MaskReplace(rule *config.ProcessingRule, content []byte) ([]byte, bool) {
	if !isMatchingLiteralPrefix(rule.Regex, content) {
		return content, false
	}

	if rule.HasLiteralPrefix() && len(content) <= re2CGoThreshold {
		return replaceAllLazy(rule.Regex, content, rule.Placeholder, rule.PlaceholderHasExpansion())
	}

	compiled := rule.RE2Compiled()
	if compiled == nil {
		return replaceAllLazy(rule.Regex, content, rule.Placeholder, rule.PlaceholderHasExpansion())
	}

	if rule.PlaceholderHasExpansion() {
		return re2.ReplaceExpand(compiled, content, rule.Placeholder)
	}
	return re2.ReplaceLiteral(compiled, content, rule.Placeholder)
}
