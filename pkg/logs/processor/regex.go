// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"regexp"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// matchLiterals checks whether content contains any of the rule's
// pre-computed literal byte strings using bytes.Contains. Returns false
// if the rule has no literal contents.
func matchLiterals(rule *config.ProcessingRule, content []byte) bool {
	for _, lit := range rule.LiteralContents() {
		if bytes.Contains(content, lit) {
			return true
		}
	}
	return false
}

// isMatchingLiteralPrefix uses a potential literal prefix from the given regex
// to indicate if the content even has a chance of matching the regex.
func isMatchingLiteralPrefix(r *regexp.Regexp, content []byte) bool {
	prefix, _ := r.LiteralPrefix()
	if prefix == "" {
		return true
	}

	return bytes.Contains(content, []byte(prefix))
}

// replaceAllLazy performs the same replacement as re.ReplaceAll(src, repl) but
// avoids allocating a new buffer when there are no matches. When needExpand is
// true, submatch capture groups in repl (e.g. "$1") are expanded via
// re.Expand; when false, repl is used as a literal replacement.
//
// Returns (result, true) when at least one replacement was made, or
// (src, false) with zero allocations when src contains no matches.
func replaceAllLazy(re *regexp.Regexp, src, repl []byte, needExpand bool) ([]byte, bool) {
	if needExpand {
		matches := re.FindAllSubmatchIndex(src, -1)
		if matches == nil {
			return src, false
		}
		var buf []byte
		lastEnd := 0
		for _, m := range matches {
			buf = append(buf, src[lastEnd:m[0]]...)
			buf = re.Expand(buf, repl, src, m)
			lastEnd = m[1]
		}
		buf = append(buf, src[lastEnd:]...)
		return buf, true
	}

	matches := re.FindAllIndex(src, -1)
	if matches == nil {
		return src, false
	}

	var buf []byte
	lastEnd := 0
	for _, m := range matches {
		buf = append(buf, src[lastEnd:m[0]]...)
		buf = append(buf, repl...)
		lastEnd = m[1]
	}
	buf = append(buf, src[lastEnd:]...)
	return buf, true
}
