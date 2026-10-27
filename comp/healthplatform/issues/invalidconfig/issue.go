// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/dustin/go-humanize/english"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	contextKeyConfigPath  = "config_path"
	contextKeyErrors      = "errors"
	contextKeyErrorCount  = "error_count"
	contextKeyImpact      = "impact"
	contextKeyViolations  = "violations"
	contextKeyViolation   = "violation"
	contextKeySettingPath = "setting_path"
)

// InvalidConfigIssue is the template for "invalid-config" issues.
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	issue := BuildIssue(ctx, "Agent")
	issue.IssueName = IssueName
	issue.IssueType = IssueType
	return issue, nil
}

// BuildIssue builds one configuration issue, shared by the Agent and system-probe checks.
func BuildIssue(ctx map[string]string, component string) *healthplatform.Issue {
	path := ctx[contextKeySettingPath]
	setting := inlineCode(path)
	if path == "" {
		setting = "the configuration"
	}
	description := "The value at " + setting + " does not match the configuration schema."
	correction := "Check the value and format of " + setting + "."
	title := "Invalid configuration: " + setting
	if path == "" {
		title = "Invalid " + component + " configuration"
		description = "The " + component + " configuration does not match the configuration schema."
	}
	fields := map[string]any{}
	var violation violationPayload
	if err := json.Unmarshal([]byte(ctx[contextKeyViolation]), &violation); err == nil {
		if details, fix := formatViolation(violation); details != "" {
			description, correction = details, fix
			title = "Incorrect type for " + setting
			var metadata any
			_ = json.Unmarshal([]byte(ctx[contextKeyViolation]), &metadata)
			fields[contextKeyViolations] = []any{metadata}
		}
	}
	configPath := ctx[contextKeyConfigPath]
	where := "your " + component + " configuration file"
	if configPath != "" {
		where = inlineCode(configPath)
	} else {
		configPath = "(unknown path)"
	}
	fields[contextKeyConfigPath] = configPath
	fields[contextKeyErrorCount] = 1
	// Fleet can show only the inline warning, so include the fix there too.
	fields[contextKeyErrors] = map[string]any{path: []any{description + " " + correction}}
	fields[contextKeyImpact] = "The Datadog " + component + " may apply defaults for incorrectly-typed fields and may not behave as configured."
	extra, _ := structpb.NewStruct(fields)
	return &healthplatform.Issue{
		Title:       title,
		Description: description,
		Category:    "configuration",
		Location:    strings.ToLower(component),
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "config",
		Extra:       extra,
		Tags:        []string{"config", "schema"},
		Remediation: &healthplatform.Remediation{
			Summary: "Correct this setting, then restart the Datadog Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check this setting in " + where + " or environment variables."},
				{Order: 2, Text: correction},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}
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
