// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig provides an issue module that reports datadog.yaml
// problems (unparseable YAML or schema violations) through the Agent Health
// Platform. Detection runs as a periodic built-in check; the rescue path in
// pkg/config/lite reports the same issue when normal agent startup fails.
package invalidconfig

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"

	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// Context keys produced by Check() and consumed by BuildIssue(). The keys are
// also stamped into the Issue.Extra struct so the backend / UI sees them.
const (
	contextKeyErrorKind    = "error_kind"
	contextKeyConfigPath   = "config_path"
	contextKeyErrorMessage = "error_message"
	contextKeyErrors       = "errors"
	contextKeyErrorCount   = "error_count"
	contextKeyTruncated    = "truncated"
)

// errorKind values mirror pkg/config/lite/rescue.go exactly so a backend
// dashboard filtering on @issue.error_kind matches both code paths.
const (
	errorKindYAMLParse        = "yaml_parse"
	errorKindSchemaValidation = "schema_validation"
)

// maxErrorsInPayload bounds how many individual schema-validation errors we
// embed in the Extra struct. A wildly broken config could generate hundreds;
// keeping the first N preserves UI readability while error_count exposes the
// true total.
const maxErrorsInPayload = 20

// InvalidConfigIssue is the template implementation. It produces two flavours
// of issue dispatched on context["error_kind"].
type InvalidConfigIssue struct{}

// NewInvalidConfigIssue creates a new invalid-config issue template.
func NewInvalidConfigIssue() *InvalidConfigIssue { return &InvalidConfigIssue{} }

// BuildIssue dispatches on the error_kind context value to produce either a
// YAML-parse-failure issue (high severity) or a schema-validation issue
// (medium severity). Unknown error_kind values fall through to the schema
// shape since "the config has problems" is still useful information.
func (t *InvalidConfigIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	kind := context[contextKeyErrorKind]
	path := context[contextKeyConfigPath]
	if path == "" {
		path = "(unknown path)"
	}

	switch kind {
	case errorKindYAMLParse:
		return buildYAMLParseIssue(path, context[contextKeyErrorMessage])
	default:
		return buildSchemaValidationIssue(path, context)
	}
}

func buildYAMLParseIssue(path, parseMsg string) (*healthplatform.Issue, error) {
	extra, err := structpb.NewStruct(map[string]any{
		contextKeyErrorKind:    errorKindYAMLParse,
		contextKeyConfigPath:   path,
		contextKeyErrorMessage: parseMsg,
		"impact":               "The Datadog Agent could not load this configuration file. It is running with defaults only — no telemetry will reach Datadog until this is fixed.",
	})
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: build extra struct: %w", err)
	}

	return &healthplatform.Issue{
		Id:          healthplatformdef.InvalidConfigIssueID,
		IssueName:   "invalid_config",
		Title:       "Datadog Agent configuration is not valid YAML",
		Description: fmt.Sprintf("The Datadog Agent could not parse %s as YAML: %s", path, truncate(parseMsg, 400)),
		Category:    "config",
		Location:    "config",
		Severity:    "high",
		Source:      "config",
		Extra:       extra,
		Tags:        []string{"config", "yaml_parse"},
		Remediation: &healthplatform.Remediation{
			Summary: "Open the configuration file and fix the YAML syntax error, then restart the agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Look at the location reported by the parser: " + truncate(parseMsg, 200)},
				{Order: 3, Text: "Fix the YAML syntax (check indentation, quoting, brackets)."},
				{Order: 4, Text: fmt.Sprintf("Validate with: datadog-agent experimental check-config -c %s", path)},
				{Order: 5, Text: "Restart the agent: sudo systemctl restart datadog-agent (or your platform's equivalent)."},
			},
		},
	}, nil
}

func buildSchemaValidationIssue(path string, context map[string]string) (*healthplatform.Issue, error) {
	errorsStr := context[contextKeyErrors]
	count := context[contextKeyErrorCount]
	if count == "" {
		count = "?"
	}

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyErrorKind:  errorKindSchemaValidation,
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     errorsStr,
		contextKeyTruncated:  context[contextKeyTruncated] == "true",
		"impact":             "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
	})
	if err != nil {
		return nil, fmt.Errorf("invalidconfig: build extra struct: %w", err)
	}

	firstLine := strings.SplitN(errorsStr, "\n", 2)[0]
	desc := fmt.Sprintf("Found %s schema violation(s) in %s.", count, path)
	if firstLine != "" {
		desc += " First: " + truncate(firstLine, 240)
	}

	return &healthplatform.Issue{
		Id:          healthplatformdef.InvalidConfigIssueID,
		IssueName:   "invalid_config",
		Title:       fmt.Sprintf("Datadog Agent configuration has %s schema violation(s)", count),
		Description: desc,
		Category:    "config",
		Location:    "config",
		Severity:    "medium",
		Source:      "config",
		Extra:       extra,
		Tags:        []string{"config", "schema"},
		Remediation: &healthplatform.Remediation{
			Summary: "Fix each schema violation in the configuration file and restart the agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Review the listed violations (see Extra.errors)."},
				{Order: 3, Text: fmt.Sprintf("Validate after fixing: datadog-agent experimental check-config -c %s", path)},
				{Order: 4, Text: "Restart the agent."},
			},
		},
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
