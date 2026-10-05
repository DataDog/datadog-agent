// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"regexp"
	"strings"
)

var (
	screamingSnakeCaseBoundaryAcronym = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	screamingSnakeCaseBoundaryWord    = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

// ToScreamingSnakeCase converts a camelCase or PascalCase string to
// SCREAMING_SNAKE_CASE, e.g. "targetURL" -> "TARGET_URL". Characters other than
// letters, digits, and underscores are passed through unchanged.
func ToScreamingSnakeCase(s string) string {
	s = screamingSnakeCaseBoundaryAcronym.ReplaceAllString(s, "${1}_${2}")
	s = screamingSnakeCaseBoundaryWord.ReplaceAllString(s, "${1}_${2}")
	return strings.ToUpper(s)
}
