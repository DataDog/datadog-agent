// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configschema

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/dustin/go-humanize/english"
)

const defaultCorrection = "Fix each violation listed in the description."

var typeLabels = map[string]string{
	"boolean": "true or false",
	"integer": "a whole number",
	"number":  "a number",
	"string":  "a string",
	"array":   "a YAML list",
	"object":  "a YAML mapping",
	"null":    "null",
}

// FormatViolations explains each error and how to fix it.
// Unusable details leave the existing description and generic remediation intact.
func FormatViolations(raw string) (string, string) {
	var violations []ViolationPayload
	if err := json.Unmarshal([]byte(raw), &violations); err != nil || len(violations) == 0 {
		return "", defaultCorrection
	}
	descriptions := make([]string, len(violations))
	corrections := make([]string, len(violations))
	for i, violation := range violations {
		descriptions[i], corrections[i] = formatViolation(violation)
		if descriptions[i] == "" {
			return "", defaultCorrection
		}
	}
	description := strings.Join(descriptions, " ")
	if len(violations) == 1 {
		return description, corrections[0]
	}
	return description, "- " + strings.Join(corrections, "\n- ")
}

// formatViolation separates what is wrong from how to fix it.
// Empty results tell the caller to keep the existing safe messages.
func formatViolation(violation ViolationPayload) (string, string) {
	actual := typeLabels[violation.ActualType]
	if actual == "" || len(violation.ExpectedTypes) == 0 {
		return "", ""
	}
	expected := make([]string, len(violation.ExpectedTypes))
	for i, kind := range violation.ExpectedTypes {
		expected[i] = typeLabels[kind]
		if expected[i] == "" {
			return "", ""
		}
	}
	want := english.OxfordWordSeries(expected, "or")
	if violation.Path == "" {
		violation.Path = "/"
	}
	path := inlineCode(violation.Path)
	description := fmt.Sprintf("%s expects %s, but received %s.", path, want, actual)
	correction := fmt.Sprintf("Set %s to %s.", path, want)
	switch violation.DefaultStatus {
	case "known":
		value, err := json.Marshal(violation.DefaultValue)
		if err != nil || violation.DefaultValue == nil {
			return "", ""
		}
		correction += " The default value for this setting is " + inlineCode(string(value))
		if violation.DefaultValue == "" {
			correction += " (an empty string)"
		}
		return description, correction + "."
	case "none":
		return description, correction + " This setting has no default."
	case "unknown":
		return description, correction
	default:
		return "", ""
	}
}

// inlineCode replaces control characters with spaces and wraps text as Markdown code.
// Its fence is longer than any backtick run so the text cannot close it.
func inlineCode(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	fenceLength := 1
	for _, ticks := range strings.FieldsFunc(text, func(r rune) bool { return r != '`' }) {
		fenceLength = max(fenceLength, len(ticks)+1)
	}
	fence := strings.Repeat("`", fenceLength)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return fence + text + fence
}
