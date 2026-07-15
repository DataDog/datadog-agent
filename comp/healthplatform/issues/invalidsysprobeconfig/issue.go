// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidsysprobeconfig

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	contextKeyConfigPath = "config_path"
	contextKeyErrors     = "errors"
	contextKeyErrorCount = "error_count"
	contextKeyImpact     = "impact"
)

// contextErrorKey returns the Context key for the i-th error line.
func contextErrorKey(i int) string {
	return "error." + strconv.Itoa(i)
}

// InvalidSysprobeConfigIssue is the template for "invalid-system-probe-config" issues.
type InvalidSysprobeConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidSysprobeConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
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

	// Group messages by JSON path, as []any so structpb can consume them directly.
	errMap := make(map[string]any, len(errLines))
	for _, line := range errLines {
		// Schema errors have the form: at '<path>': <message>
		// Strip the "at '" prefix and trailing "'" to get a bare JSON path.
		before, msg, _ := strings.Cut(line, ": ")
		errPath := strings.TrimSuffix(strings.TrimPrefix(before, "at '"), "'")
		msgs, _ := errMap[errPath].([]any)
		errMap[errPath] = append(msgs, msg)
	}

	extra, _ := structpb.NewStruct(map[string]any{
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     errMap,
		contextKeyImpact:     "The Datadog system-probe may apply defaults for incorrectly-typed fields and may not behave as configured.",
	})

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       fmt.Sprintf("Datadog System-Probe Configuration Has %d Schema Violation%s in %s", count, suffix, filepath.Base(path)),
		Description: desc,
		Category:    "configuration",
		Location:    "system-probe",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "config",
		Extra:       extra,
		Tags:        []string{"config", "schema", "system-probe"},
		Remediation: &healthplatform.Remediation{
			Summary: "Fix each schema violation in the system-probe configuration, then restart the Datadog Agent.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Fix each violation listed in the description."},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}, nil
}
