// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// impactText is shared by the description and extra.impact so the consequence is stated on
// every surface, including the ones that only render the description.
const impactText = "Until this is corrected the Agent falls back to the default for each setting, so it may not behave as configured."

const (
	contextKeyConfigPath = "config_path"
	contextKeyErrors     = "errors"
	contextKeyErrorCount = "error_count"
	contextKeyImpact     = "impact"
)

// The check writes each violation as three parallel keys rather than one string, so BuildIssue
// never has to parse a message back apart. The previous form split on ": ", which silently
// produced a garbage pointer for any message containing that sequence.
func contextErrorKey(i int) string   { return "error." + strconv.Itoa(i) }
func contextPointerKey(i int) string { return "error." + strconv.Itoa(i) + ".pointer" }
func contextFixKey(i int) string     { return "error." + strconv.Itoa(i) + ".fix" }

// InvalidConfigIssue is the template for "invalid-config" issues.
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	path := ctx[contextKeyConfigPath]
	if path == "" {
		path = "(unknown path)"
	}
	count, _ := strconv.Atoi(ctx[contextKeyErrorCount])

	type violation struct{ message, fix, pointer string }
	violations := make([]violation, 0, count)
	for i := 0; i < count; i++ {
		if m := ctx[contextErrorKey(i)]; m != "" {
			violations = append(violations, violation{
				message: m,
				fix:     ctx[contextFixKey(i)],
				pointer: ctx[contextPointerKey(i)],
			})
		}
	}

	suffix := ""
	if count != 1 {
		suffix = "s"
	}
	// The only field `agent diagnose` renders, so it carries the diagnosis and the consequence.
	var descBuilder strings.Builder
	fmt.Fprintf(&descBuilder, "Found %d problem%s in %s.", count, suffix, path)
	for _, v := range violations {
		descBuilder.WriteString(" " + v.message)
	}
	descBuilder.WriteString(" " + impactText)
	desc := descBuilder.String()

	// Path-keyed so a consumer can attach each one to the config line it came from; the
	// diagnosis and its correction sit under the same key.
	errGroups := make(map[string][]string, len(violations))
	order := make([]string, 0, len(violations))
	for _, v := range violations {
		key := v.pointer
		if key == "" {
			key = "/"
		}
		if _, seen := errGroups[key]; !seen {
			order = append(order, key)
		}
		errGroups[key] = append(errGroups[key], v.message)
		if v.fix != "" {
			errGroups[key] = append(errGroups[key], v.fix)
		}
	}
	errMap := make(map[string]any, len(errGroups))
	for _, key := range order {
		msgs := errGroups[key]
		slice := make([]any, len(msgs))
		for i, m := range msgs {
			slice[i] = m
		}
		errMap[key] = slice
	}

	// One step per violation. "Fix each violation listed in the description" was the only
	// remediation step in the package that told the user nothing they did not already know.
	// Capped because a bad 50-element list would otherwise produce 50 steps.
	const maxViolationSteps = 10
	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
	}
	for _, v := range violations {
		if len(steps) > maxViolationSteps {
			break
		}
		text := v.fix
		if text == "" {
			text = v.message
		}
		steps = append(steps, &healthplatform.RemediationStep{Order: int32(len(steps) + 1), Text: text})
	}
	if hidden := len(violations) - maxViolationSteps; hidden > 0 {
		plural := "s"
		if hidden == 1 {
			plural = ""
		}
		steps = append(steps, &healthplatform.RemediationStep{
			Order: int32(len(steps) + 1),
			Text:  fmt.Sprintf("Correct the remaining %d setting%s listed in the issue details.", hidden, plural),
		})
	}
	steps = append(steps,
		&healthplatform.RemediationStep{Order: int32(len(steps) + 1), Text: "Restart the Datadog Agent."},
		&healthplatform.RemediationStep{Order: int32(len(steps) + 2), Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
	)

	extra, _ := structpb.NewStruct(map[string]any{
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     errMap,
		contextKeyImpact:     impactText,
	})

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
			Summary: fmt.Sprintf("Correct %d setting%s in %s, then restart the Datadog Agent.",
				count, suffix, filepath.Base(path)),
			Steps: steps,
		},
	}, nil
}
