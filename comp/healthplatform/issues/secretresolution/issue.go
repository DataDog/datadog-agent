// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package secretresolution

import (
	"errors"
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// IssueName and IssueType identify the secret lookup failure contract.
	IssueName = "Secret Resolution Failure"
	IssueType = "secret_resolution_failure"
)

// SecretResolutionIssue builds the customer-facing explanation of a failed lookup.
type SecretResolutionIssue struct{}

var failureMessages = map[string]struct{ description, correction string }{
	"missing":          {"did not return a value for", "Check that this secret exists and that your backend returns an entry for the requested secret."},
	"empty":            {"returned an empty value for", "Check that this secret has a non-empty value, including after any trailing line breaks are removed."},
	"invalid_response": {"returned an unreadable response while looking up", "Check that the backend returns valid JSON in the [documented response format](https://docs.datadoghq.com/agent/configuration/secrets-management/)."},
	"backend_error":    {"could not resolve", "Check the backend configuration, its availability, and the Agent's access permissions."},
}

// BuildIssue uses fixed explanations, never backend errors or resolved values.
func (i *SecretResolutionIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	message := failureMessages[ctx["reason"]]
	if message.description == "" {
		return nil, errors.New("unknown secret resolution failure reason")
	}
	reference := "ENC[" + ctx["handle"] + "]"
	description := fmt.Sprintf("The secret backend %s %q, used by %q in %q.", message.description, reference, ctx["setting_path"], ctx["configuration"])
	impact := "This setting has no resolved secret value. Affected integration instances may not be running."
	if ctx["cached"] == "true" {
		impact = "The Agent kept the previously resolved value, but the latest lookup failed."
	}
	extra, err := structpb.NewStruct(map[string]any{
		"secret_reference": reference,
		"configuration":    ctx["configuration"], "setting_path": ctx["setting_path"],
		"reason": ctx["reason"], "has_cached_value": ctx["cached"] == "true", "impact": impact,
	})
	if err != nil {
		return nil, err
	}
	return &healthplatform.Issue{
		IssueName: IssueName, IssueType: IssueType,
		Title: fmt.Sprintf("Agent could not load secret %q for %s", ctx["handle"], ctx["configuration"]), Description: description + " " + impact,
		Category: "configuration", Location: "agent", Source: "secrets",
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
		Extra:    extra, Tags: []string{"secrets", "configuration"},
		Remediation: &healthplatform.Remediation{
			Summary: "Fix the secret lookup, then retry it.",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Check the secret reference %q in setting %q of %q.", reference, ctx["setting_path"], ctx["configuration"])},
				{Order: 2, Text: message.correction + " Run `datadog-agent secret` for local backend diagnostics."},
				{Order: 3, Text: "After fixing the backend, run `datadog-agent secret refresh`. Restart the Agent if the affected setting cannot be refreshed. The warning clears after a successful lookup."},
			},
		},
	}, nil
}
