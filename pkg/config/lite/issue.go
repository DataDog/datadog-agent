// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package lite provides shared building blocks for the Agent Health
// "invalid-config" issue.
package lite

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// IssueID is the stable Agent Health issue identifier for configuration-validation
const IssueID = "invalid-config"

// ErrorKind discriminates the variant of invalid-config issue
type ErrorKind string

const (
	// ErrorKindYAMLParse means yaml.Unmarshal rejected datadog.yaml outright
	ErrorKindYAMLParse ErrorKind = "yaml_parse"
	// ErrorKindSchemaValidation means the parsed map failed the embedded schema
	ErrorKindSchemaValidation ErrorKind = "schema_validation"
	// ErrorKindStartupFailure is a catch-all when the agent fails to start
	ErrorKindStartupFailure ErrorKind = "startup_failure"
)

// Context keys used in Issue.Extra and IssueReport.Context
const (
	ContextKeyErrorKind    = "error_kind"
	ContextKeyConfigPath   = "config_path"
	ContextKeyErrorMessage = "error_message"
	ContextKeyErrors       = "errors"
	ContextKeyErrorCount   = "error_count"
	ContextKeyImpact       = "impact"
)

// IssueInfo is the input to BuildInvalidConfigIssue.
type IssueInfo struct {
	Kind         ErrorKind
	ConfigPath   string
	ErrorMessage string // yaml_parse / startup_failure
	Errors       string // schema_validation, newline-joined
	ErrorCount   int    // schema_validation: total violation count
}

// BuildInvalidConfigIssue produces the healthplatform.Issue for an invalid datadog.yaml.
// Sets kind-based tags; producers add context-specific ones (env, host) on top.
func BuildInvalidConfigIssue(info IssueInfo) *healthplatform.Issue {
	var issue *healthplatform.Issue
	switch info.Kind {
	case ErrorKindYAMLParse:
		issue = yamlParseIssue(info)
	case ErrorKindStartupFailure:
		issue = startupFailureIssue(info)
	default:
		issue = schemaValidationIssue(info)
	}
	issue.Tags = info.Tags()
	return issue
}

// Tags returns the static tag list that pairs with this issue kind.
func (info IssueInfo) Tags() []string {
	switch info.Kind {
	case ErrorKindYAMLParse:
		return []string{"config", "yaml_parse"}
	case ErrorKindStartupFailure:
		return []string{"agent", "startup_failure"}
	default:
		return []string{"config", "schema"}
	}
}

// ToContext serialises IssueInfo into the IssueReport.Context
func (info IssueInfo) ToContext() map[string]string {
	return map[string]string{
		ContextKeyErrorKind:    string(info.Kind),
		ContextKeyConfigPath:   info.ConfigPath,
		ContextKeyErrorMessage: info.ErrorMessage,
		ContextKeyErrors:       info.Errors,
		ContextKeyErrorCount:   strconv.Itoa(info.ErrorCount),
	}
}

// IssueInfoFromContext is the inverse of IssueInfo.ToContext.
func IssueInfoFromContext(ctx map[string]string) IssueInfo {
	count, _ := strconv.Atoi(ctx[ContextKeyErrorCount])
	return IssueInfo{
		Kind:         ErrorKind(ctx[ContextKeyErrorKind]),
		ConfigPath:   ctx[ContextKeyConfigPath],
		ErrorMessage: ctx[ContextKeyErrorMessage],
		Errors:       ctx[ContextKeyErrors],
		ErrorCount:   count,
	}
}

func yamlParseIssue(info IssueInfo) *healthplatform.Issue {
	path := info.ConfigPath
	if path == "" {
		path = "(no datadog.yaml found)"
	}
	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       "Datadog Agent configuration is not valid YAML",
		Description: fmt.Sprintf("The Datadog Agent could not parse %s as YAML: %s", path, info.ErrorMessage),
		Category:    "config",
		Location:    "config",
		Severity:    "high",
		Source:      "config",
		Extra: asProtoStruct(map[string]any{
			ContextKeyErrorKind:    string(ErrorKindYAMLParse),
			ContextKeyConfigPath:   path,
			ContextKeyErrorMessage: info.ErrorMessage,
			ContextKeyImpact:       "The Datadog Agent cannot start until the configuration file is valid YAML",
		}),
		Remediation: &healthplatform.Remediation{
			Summary: "Fix the YAML syntax error in the configuration file, then restart the Datadog Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Locate the syntax error reported by the parser: " + info.ErrorMessage},
				{Order: 3, Text: "Fix the YAML (common causes: indentation, unquoted special characters, mismatched brackets or quotes)."},
				{Order: 4, Text: "Restart the Datadog Agent."},
				{Order: 5, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}
}

func schemaValidationIssue(info IssueInfo) *healthplatform.Issue {
	path := info.ConfigPath
	if path == "" {
		path = "(unknown path)"
	}
	suffix := ""
	if info.ErrorCount != 1 {
		suffix = "s"
	}
	desc := fmt.Sprintf("Found %d schema violation%s in %s", info.ErrorCount, suffix, path)
	if info.Errors != "" {
		desc += ": " + strings.ReplaceAll(info.Errors, "\n", "; ")
	} else {
		desc += "."
	}
	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       fmt.Sprintf("Datadog Agent configuration has %d schema violation%s", info.ErrorCount, suffix),
		Description: desc,
		Category:    "config",
		Location:    "config",
		Severity:    "medium",
		Source:      "config",
		Extra: asProtoStruct(map[string]any{
			ContextKeyErrorKind:  string(ErrorKindSchemaValidation),
			ContextKeyConfigPath: path,
			ContextKeyErrorCount: info.ErrorCount,
			ContextKeyErrors:     strings.ReplaceAll(info.Errors, "\n", " • "),
			ContextKeyImpact:     "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
		}),
		Remediation: &healthplatform.Remediation{
			Summary: "Fix each schema violation in the configuration file, then restart the Datadog Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Fix each violation listed in the description."},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}
}

func startupFailureIssue(info IssueInfo) *healthplatform.Issue {
	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       "Datadog Agent failed to start",
		Description: "Configuration is parseable but the agent could not complete startup: " + info.ErrorMessage,
		Category:    "config",
		Location:    "agent",
		Severity:    "high",
		Source:      "config",
		Extra: asProtoStruct(map[string]any{
			ContextKeyErrorKind:    string(ErrorKindStartupFailure),
			ContextKeyConfigPath:   info.ConfigPath,
			ContextKeyErrorMessage: info.ErrorMessage,
			ContextKeyImpact:       "The Datadog Agent process failed to start. No telemetry will be collected until the underlying problem is resolved.",
		}),
		Remediation: &healthplatform.Remediation{
			Summary: "Inspect the Datadog Agent logs for the underlying cause and address it before restarting.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Open the Datadog Agent log file"},
				{Order: 2, Text: "Look for the error message: " + info.ErrorMessage},
				{Order: 3, Text: "Resolve the underlying issue (port conflicts, missing files, permissions, etc.)."},
				{Order: 4, Text: "Restart the Datadog Agent."},
			},
		},
	}
}

// asStruct converts a map to a structpb.Struct
func asProtoStruct(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		return &structpb.Struct{}
	}
	return s
}
