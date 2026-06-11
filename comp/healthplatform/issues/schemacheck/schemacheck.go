// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package schemacheck holds the schema-validation flow
package schemacheck

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/types/known/structpb"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Keys for the IssueReport context map, shared by Run and BuildIssue.
const (
	ContextKeyConfigPath = "config_path"
	ContextKeyErrors     = "errors"
	ContextKeyErrorCount = "error_count"
	ContextKeyImpact     = "impact"
)

// ConfigReader is the minimal config surface the schema check needs
type ConfigReader interface {
	AllSettingsWithoutDefaultOrSecrets() map[string]any
	ConfigFileUsed() string
}

// Validator validates a normalized config map and returns human-readable violations.
type Validator func(config any) ([]string, error)

// Check validates a config against its schema and builds the resulting Issue.
type Check struct {
	IssueID            string // dedup key: proto IssueName and IssueReport IssueID/IssueName
	Validator          Validator
	Subject            string
	ViolationNoun      string
	Location           string
	Tags               []string
	Impact             string
	RemediationSummary string
}

// Run validates the customer-set config and reports a single IssueReport on any violations.
func (c Check) Run(cfg ConfigReader) ([]runnerdef.IssueReport, error) {
	// AllSettingsWithoutDefaultOrSecrets returns only values the customer actually set
	raw := cfg.AllSettingsWithoutDefaultOrSecrets()
	if len(raw) == 0 {
		return nil, nil
	}
	normalized, err := normalizeForSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: normalize config: %w", c.IssueID, err)
	}
	errs, schemaErr := c.Validator(normalized)
	if schemaErr != nil {
		pkglog.Warnf("%s: schema validator unavailable; skipping check: %v", c.IssueID, schemaErr)
		return nil, schemaErr
	}
	if len(errs) == 0 {
		return nil, nil
	}
	// Source is left empty; the runner backfills it from the check's Source.
	return []runnerdef.IssueReport{
		{
			IssueID:   c.IssueID,
			IssueName: c.IssueID,
			Context: map[string]string{
				ContextKeyConfigPath: cfg.ConfigFileUsed(),
				ContextKeyErrorCount: strconv.Itoa(len(errs)),
				ContextKeyErrors:     strings.Join(errs, "\n"),
			},
		},
	}, nil
}

// normalizeForSchema coerces a Go-native config map into JSON-native types via
// a YAML round-trip. ScrubYaml strips any accidental secret-like values
func normalizeForSchema(in map[string]any) (map[string]any, error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}
	scrubbed, err := scrubber.ScrubYaml(b)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(scrubbed, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BuildIssue decodes the IssueReport.Context and builds the proto Issue.
func (c Check) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	path := ctx[ContextKeyConfigPath]
	if path == "" {
		path = "(unknown path)"
	}
	schemaErrors := ctx[ContextKeyErrors]

	count, err := strconv.Atoi(ctx[ContextKeyErrorCount])
	if err != nil && schemaErrors != "" {
		count = strings.Count(schemaErrors, "\n") + 1
	}

	suffix := ""
	if count != 1 {
		suffix = "s"
	}
	// One bullet-separated blob, reused for the prose description and the structured field.
	errorList := strings.ReplaceAll(schemaErrors, "\n", " • ")
	desc := fmt.Sprintf("Found %d %s violation%s in %s", count, c.ViolationNoun, suffix, path)
	if schemaErrors != "" {
		desc += ": " + errorList
	} else {
		desc += "."
	}

	extra, _ := structpb.NewStruct(map[string]any{
		ContextKeyConfigPath: path,
		ContextKeyErrorCount: count,
		ContextKeyErrors:     errorList,
		ContextKeyImpact:     c.Impact,
	})

	return &healthplatform.Issue{
		IssueName:   c.IssueID,
		Title:       fmt.Sprintf("%s has %d schema violation%s", c.Subject, count, suffix),
		Description: desc,
		Category:    "configuration",
		Location:    c.Location,
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Source:      "config",
		Extra:       extra,
		Tags:        c.Tags,
		Remediation: &healthplatform.Remediation{
			Summary: c.RemediationSummary,
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Open %s in an editor.", path)},
				{Order: 2, Text: "Fix each violation listed in the description."},
				{Order: 3, Text: "Restart the Datadog Agent."},
				{Order: 4, Text: "Run `datadog-agent diagnose` to confirm the configuration is now valid."},
			},
		},
	}, nil
}
