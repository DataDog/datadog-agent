// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	workspaceNameRegex = regexp.MustCompile(`[^\p{L}\p{N}_:./-]+`)
	dedupeUnderscores  = regexp.MustCompile(`_+`)
)

// NormalizeWorkspaceName normalizes a workspace name to match Python's normalize_tags behavior:
// 1. Convert to all lowercase unicode string
// 2. Trim leading/trailing quotes
// 3. Convert bad characters to underscores
// 4. Dedupe contiguous underscores
// 5. Remove initial underscores/digits such that the string starts with an alpha char
// 6. Truncate to 200 characters
// 7. Strip trailing underscores
func NormalizeWorkspaceName(value string) string {
	if value == "" {
		return ""
	}
	normalized := strings.ToLower(value)
	normalized = strings.Trim(normalized, "\"'")
	normalized = workspaceNameRegex.ReplaceAllString(normalized, "_")
	normalized = dedupeUnderscores.ReplaceAllString(normalized, "_")

	if len(normalized) > 0 && (normalized[0] == '_' || normalized[0] == ':') {
		normalized = strings.TrimLeft(normalized, "_")
		if len(normalized) > 0 && unicode.IsDigit([]rune(normalized)[0]) {
			normalized = strings.TrimLeft(normalized, "_:0123456789")
		}
	}

	if len(normalized) > 200 {
		normalized = normalized[:200]
	}
	return strings.TrimRight(normalized, "_")
}
