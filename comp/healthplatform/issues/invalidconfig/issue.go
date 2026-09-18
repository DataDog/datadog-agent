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
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	contextKeyConfigPath        = "config_path"
	contextKeyErrors            = "errors"
	contextKeyErrorCount        = "error_count"
	contextKeyImpact            = "impact"
	contextKeyViolationsVersion = "violations_version"
	contextKeyViolations        = "violations"
	defaultCorrection           = "Fix each violation listed in the description."
)

// contextErrorKey returns the Context key for the i-th error line.
func contextErrorKey(i int) string {
	return "error." + strconv.Itoa(i)
}

// InvalidConfigIssue is the template for "invalid-config" issues.
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	path := ctx[contextKeyConfigPath]
	if path == "" {
		path = "(unknown path)"
	}
	count, _ := strconv.Atoi(ctx[contextKeyErrorCount])

	errLines := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if v := ctx[contextErrorKey(i)]; v != "" {
			errLines = append(errLines, v)
		}
	}

	suffix := ""
	if count != 1 {
		suffix = "s"
	}
	desc := fmt.Sprintf("Found %d schema violation%s in %s", count, suffix, path)
	if len(errLines) > 0 {
		desc += ": " + strings.Join(errLines, "; ")
	} else {
		desc += "."
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

	extraFields := map[string]any{
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     errMap,
		contextKeyImpact:     "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
	}
	correction := defaultCorrection
	if ctx[contextKeyViolationsVersion] == "1" {
		var violations []any
		if err := json.Unmarshal([]byte(ctx[contextKeyViolations]), &violations); err == nil && len(violations) > 0 {
			extraFields[contextKeyViolationsVersion] = 1
			extraFields[contextKeyViolations] = violations
			correction = formatCorrections(ctx[contextKeyViolations])
		}
	}
	extra, _ := structpb.NewStruct(extraFields)

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("Datadog Agent Configuration Has %d Schema Violation%s in %s", count, suffix, filepath.Base(path)),
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
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
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

func formatCorrections(raw string) string {
	var violations []violationPayload
	if err := json.Unmarshal([]byte(raw), &violations); err != nil || len(violations) == 0 {
		return defaultCorrection
	}
	const limit = 10
	corrections := make([]string, 0, min(len(violations), limit))
	for _, violation := range violations[:min(len(violations), limit)] {
		correction := formatCorrection(violation)
		if correction == "" {
			return defaultCorrection
		}
		corrections = append(corrections, correction)
	}
	if len(violations) == 1 {
		return corrections[0]
	}
	result := "- " + strings.Join(corrections, "\n- ")
	if len(violations) > limit {
		remaining := len(violations) - limit
		wording := "violations are"
		if remaining == 1 {
			wording = "violation is"
		}
		result += fmt.Sprintf("\n\n%d more %s listed in the description.", remaining, wording)
	}
	return result
}

func formatCorrection(violation violationPayload) string {
	actual := typeLabels[violation.ActualType]
	if actual == "" || len(violation.ExpectedTypes) == 0 {
		return ""
	}
	expected := make([]string, 0, len(violation.ExpectedTypes))
	for _, kind := range violation.ExpectedTypes {
		label := typeLabels[kind]
		if label == "" {
			return ""
		}
		expected = append(expected, label)
	}
	want := strings.Join(expected, " or ")
	if len(expected) > 2 {
		want = strings.Join(expected[:len(expected)-1], ", ") + ", or " + expected[len(expected)-1]
	}
	path := violation.Path
	if path == "" {
		path = "/"
	}
	correction := fmt.Sprintf("%s received %s instead of %s. Replace it with %s.", inlineCode(path), actual, want, want)
	switch violation.DefaultStatus {
	case "known":
		value, err := json.Marshal(violation.DefaultValue)
		if err != nil || violation.DefaultValue == nil {
			return ""
		}
		correction += " The default value for this setting is " + inlineCode(string(value))
		if violation.DefaultValue == "" {
			correction += " (an empty string)"
		}
		correction += "."
	case "none":
		correction += " This setting has no default."
	case "unknown":
	default:
		return ""
	}
	return correction
}

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
