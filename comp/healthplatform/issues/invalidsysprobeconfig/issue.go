// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidsysprobeconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/configschema"
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

// InvalidSysprobeConfigIssue is the template for "invalid-system-probe-config" issues.
type InvalidSysprobeConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidSysprobeConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	count, _ := strconv.Atoi(ctx[contextKeyErrorCount])
	errorWord := english.PluralWord(count, "error", "errors")
	path := ctx[contextKeyConfigPath]
	var title, desc string
	if path == "" {
		path = "(unknown path)"
		desc = fmt.Sprintf("Found %d %s in the system-probe configuration", count, errorWord)
		title = desc
	} else {
		title = fmt.Sprintf("Found %d configuration %s in %s", count, errorWord, filepath.Base(path))
		desc = fmt.Sprintf("Found %d configuration %s in %s or environment variables", count, errorWord, path)
	}

	errLines := make([]string, 0, count)
	for i := 0; i < count; i++ {
		if v := ctx[contextErrorKey(i)]; v != "" {
			errLines = append(errLines, v)
		}
	}

	errGroups := make(map[string][]any, len(errLines))
	for _, line := range errLines {
		// Schema errors have the form: at '<path>': <message>
		// Strip the "at '" prefix and trailing "'" to get a bare JSON path.
		before, msg, _ := strings.Cut(line, ": ")
		path := strings.TrimSuffix(strings.TrimPrefix(before, "at '"), "'")
		errGroups[path] = append(errGroups[path], msg)
	}
	errMap := make(map[string]any, len(errGroups))
	for path, msgs := range errGroups {
		errMap[path] = msgs
	}

	// Add optional violation details before converting the map to protobuf.
	fields := map[string]any{
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     errMap,
		contextKeyImpact:     "The Datadog system-probe may apply defaults for incorrectly-typed fields and may not behave as configured.",
	}
	description := strings.Join(errLines, "; ")
	correction := defaultCorrection
	var violations []any
	if err := json.Unmarshal([]byte(ctx[contextKeyViolations]), &violations); err == nil && len(violations) > 0 {
		fields[contextKeyViolations] = violations
		if details, fixes := configschema.FormatViolations(ctx[contextKeyViolations]); details != "" {
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
		Title:       title,
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
				{Order: 1, Text: "Check the settings listed below in your system-probe configuration file or environment variables."},
				{Order: 2, Text: correction},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}, nil
}
