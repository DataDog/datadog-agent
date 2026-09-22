// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/dustin/go-humanize/english"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	contextKeyConfigPath = "config_path"
	contextKeyErrors     = "errors"
	contextKeyErrorCount = "error_count"
	contextKeyImpact     = "impact"
	contextKeyViolations = "violations"
	defaultCorrection    = "Fix each violation listed in the description."
)

// contextErrorKey returns the Context key for the i-th error line.
func contextErrorKey(i int) string {
	return "error." + strconv.Itoa(i)
}

// InvalidConfigIssue is the template for "invalid-config" issues.
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	count, _ := strconv.Atoi(ctx[contextKeyErrorCount])
	errorWord := english.PluralWord(count, "error", "errors")
	violationWord := english.PluralWord(count, "Schema Violation", "Schema Violations")
	path := ctx[contextKeyConfigPath]
	var desc, configStep string
	if path == "" {
		path = "(unknown path)"
		desc = fmt.Sprintf("Found %d %s in the Agent configuration", count, errorWord)
		configStep = "Check the settings listed below in your Agent configuration file or environment variables."
	} else {
		desc = fmt.Sprintf("Found %d configuration %s in %s", count, errorWord, path)
		configStep = fmt.Sprintf("Open %s in an editor.", path)
	}

	errLines := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if v := ctx[contextErrorKey(i)]; v != "" {
			errLines = append(errLines, v)
		}
	}

	errGroups := make(map[string][]string, len(errLines))
	for _, line := range errLines {
		// Schema errors have the form: at '<path>': <message>
		// Strip the "at '" prefix and trailing "'" to get a bare JSON path.
		before, msg, _ := strings.Cut(line, ": ")
		path := strings.TrimSuffix(strings.TrimPrefix(before, "at '"), "'")
		errGroups[path] = append(errGroups[path], msg)
	}
	errMap := make(map[string]any, len(errGroups))
	for path, msgs := range errGroups {
		slice := make([]any, len(msgs))
		for i, m := range msgs {
			slice[i] = m
		}
		errMap[path] = slice
	}

	// Add optional violation details before converting the map to protobuf.
	fields := map[string]any{
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     errMap,
		contextKeyImpact:     "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
	}
	description := strings.Join(errLines, "; ")
	correction := defaultCorrection
	var violations []any
	if err := json.Unmarshal([]byte(ctx[contextKeyViolations]), &violations); err == nil && len(violations) > 0 {
		fields[contextKeyViolations] = violations
		if details, fixes := formatViolations(ctx[contextKeyViolations]); details != "" {
			description, correction = details, fixes
		}
	}
	extra, _ := structpb.NewStruct(fields)
	if description != "" {
		desc += ": " + description
	} else {
		desc += "."
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("Datadog Agent Configuration Has %d %s in %s", count, violationWord, filepath.Base(path)),
		Description: desc,
		Category:    "configuration",
		Location:    "agent",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "config",
		Extra:       extra,
		Tags:        []string{"config", "schema"},
		Remediation: &healthplatform.Remediation{
			Summary: "Fix each schema violation in the configuration file, then restart the Datadog Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: configStep},
				{Order: 2, Text: correction},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}, nil
}

var typeLabels = map[string]string{
	"boolean": "true or false",
	"integer": "a whole number",
	"number":  "a number",
	"string":  "a string",
	"array":   "a YAML list",
	"object":  "a YAML mapping",
	"null":    "null",
}

// formatViolations explains each error and how to fix it.
// Unusable details leave the existing description and generic remediation intact.
func formatViolations(raw string) (string, string) {
	var violations []violationPayload
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
func formatViolation(violation violationPayload) (string, string) {
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
