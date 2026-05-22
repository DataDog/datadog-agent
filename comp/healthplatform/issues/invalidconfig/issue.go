// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package invalidconfig

import (
	"fmt"
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

// InvalidConfigIssue is the template for "invalid-config" issues.
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (InvalidConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	path := ctx[contextKeyConfigPath]
	if path == "" {
		path = "(unknown path)"
	}
	count, _ := strconv.Atoi(ctx[contextKeyErrorCount])
	errors := ctx[contextKeyErrors]

	suffix := ""
	if count != 1 {
		suffix = "s"
	}
	desc := fmt.Sprintf("Found %d schema violation%s in %s", count, suffix, path)
	if errors != "" {
		desc += ": " + strings.ReplaceAll(errors, "\n", "; ")
	} else {
		desc += "."
	}

	extra, _ := structpb.NewStruct(map[string]any{
		contextKeyConfigPath: path,
		contextKeyErrorCount: count,
		contextKeyErrors:     strings.ReplaceAll(errors, "\n", " • "),
		contextKeyImpact:     "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
	})

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "invalid_config",
		Title:       fmt.Sprintf("Datadog Agent configuration has %d schema violation%s", count, suffix),
		Description: desc,
		Category:    "config",
		Location:    "config",
		Severity:    "medium",
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
