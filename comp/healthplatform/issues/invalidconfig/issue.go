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
)

// contextErrorKey returns the Context key for the i-th error line.
func contextErrorKey(i int) string {
	return "error." + strconv.Itoa(i)
}

// InvalidConfigIssue is the template for "invalid-config" issues.
type InvalidConfigIssue struct{}

type decodedViolationPayload struct {
	Path          *string   `json:"path"`
	Rule          *string   `json:"rule"`
	ActualType    *string   `json:"actual_type"`
	ExpectedTypes *[]string `json:"expected_types"`
	Required      *bool     `json:"required"`
	DefaultStatus *string   `json:"default_status"`
	DefaultValue  *string   `json:"default_value"`
}

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
	if ctx[contextKeyViolationsVersion] == "1" {
		if violations, ok := decodeViolationPayloads(ctx[contextKeyViolations]); ok {
			extraFields[contextKeyViolationsVersion] = 1
			extraFields[contextKeyViolations] = violations
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
				{Order: 2, Text: "Fix each violation listed in the description."},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}, nil
}

func decodeViolationPayloads(raw string) ([]any, bool) {
	var decoded []decodedViolationPayload
	if json.Unmarshal([]byte(raw), &decoded) != nil || len(decoded) == 0 {
		return nil, false
	}

	violations := make([]any, 0, len(decoded))
	for _, violation := range decoded {
		if !validDecodedViolation(violation) {
			return nil, false
		}
		expectedTypes := make([]any, len(*violation.ExpectedTypes))
		for index, expectedType := range *violation.ExpectedTypes {
			expectedTypes[index] = expectedType
		}
		payload := map[string]any{
			"path":           *violation.Path,
			"rule":           *violation.Rule,
			"actual_type":    *violation.ActualType,
			"expected_types": expectedTypes,
			"required":       *violation.Required,
			"default_status": *violation.DefaultStatus,
		}
		if violation.DefaultValue != nil {
			payload["default_value"] = *violation.DefaultValue
		}
		violations = append(violations, payload)
	}
	return violations, true
}

func validDecodedViolation(violation decodedViolationPayload) bool {
	if violation.Path == nil || violation.Rule == nil || violation.ActualType == nil || violation.ExpectedTypes == nil || violation.Required == nil || violation.DefaultStatus == nil {
		return false
	}
	if *violation.Rule != "type" || !supportedActualType(*violation.ActualType) || !supportedExpectedTypes(*violation.ExpectedTypes) {
		return false
	}
	switch *violation.DefaultStatus {
	case "known":
		return violation.DefaultValue != nil && json.Valid([]byte(*violation.DefaultValue))
	case "none", "unknown":
		return violation.DefaultValue == nil
	default:
		return false
	}
}
