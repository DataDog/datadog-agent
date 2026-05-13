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
		Extra: mustStruct(map[string]any{
			ContextKeyErrorKind:    string(ErrorKindYAMLParse),
			ContextKeyConfigPath:   path,
			ContextKeyErrorMessage: info.ErrorMessage,
			ContextKeyImpact:       "The Datadog Agent cannot load its configuration and is running with defaults only. Telemetry will not be sent.",
		}),
		Remediation: &healthplatform.Remediation{
			Summary: "Open the configuration file and fix the YAML syntax error, then restart the agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Look at the location reported by the parser: " + info.ErrorMessage},
				{Order: 3, Text: "Fix the YAML syntax (check indentation, quoting, brackets)."},
				{Order: 4, Text: "Validate with: datadog-agent experimental check-config -c " + path},
				{Order: 5, Text: "Restart the agent: sudo systemctl restart datadog-agent (or your platform's equivalent)."},
			},
		},
	}
}

func schemaValidationIssue(info IssueInfo) *healthplatform.Issue {
	path := info.ConfigPath
	if path == "" {
		path = "(unknown path)"
	}
	errList := splitErrors(info.Errors)
	n, s := info.ErrorCount, pluralS(info.ErrorCount)
	desc := fmt.Sprintf("Found %d schema violation%s in %s.", n, s, path)
	if len(errList) > 0 {
		desc += " " + strings.Join(errList, "; ")
	}
	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       fmt.Sprintf("Datadog Agent configuration has %d schema violation%s", n, s),
		Description: desc,
		Category:    "config",
		Location:    "config",
		Severity:    "medium",
		Source:      "config",
		Extra: mustStruct(map[string]any{
			ContextKeyErrorKind:  string(ErrorKindSchemaValidation),
			ContextKeyConfigPath: path,
			ContextKeyErrorCount: info.ErrorCount,
			ContextKeyImpact:     "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
		}),
		Remediation: &healthplatform.Remediation{
			Summary: "Fix each schema violation in the configuration file and restart the agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Fix each violation listed in the description."},
				{Order: 3, Text: "Validate after fixing: datadog-agent experimental check-config -c " + path},
				{Order: 4, Text: "Restart the agent."},
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
		Extra: mustStruct(map[string]any{
			ContextKeyErrorKind:    string(ErrorKindStartupFailure),
			ContextKeyConfigPath:   info.ConfigPath,
			ContextKeyErrorMessage: info.ErrorMessage,
			ContextKeyImpact:       "The Datadog Agent process failed to start. No telemetry will be collected until the underlying problem is resolved.",
		}),
		Remediation: &healthplatform.Remediation{
			Summary: "Inspect the agent logs for the underlying cause and address it before restarting.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: "Check the agent log file (default /var/log/datadog/agent.log)."},
				{Order: 2, Text: "Look for the error message: " + info.ErrorMessage},
				{Order: 3, Text: "Resolve the underlying issue (port conflicts, missing files, permissions, etc.)."},
				{Order: 4, Text: "Restart the agent."},
			},
		},
	}
}

// pluralS returns "" for n == 1 and "s" otherwise
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// splitErrors turns the newline-joined Errors blob back into a slice. Empty input yields nil.
func splitErrors(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, "\n")
}

// mustStruct converts a map to a structpb.Struct. Inputs are always strings/
// ints/bools so this never fails in practice; an empty struct is returned on
// the unreachable error path rather than panicking.
func mustStruct(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		return &structpb.Struct{}
	}
	return s
}
