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
	"strconv"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/pkg/config/lite"
)

// InvalidConfigIssue is the template implementation. The actual issue content
// (titles, descriptions, remediation steps) lives in pkg/config/lite so the
// rescue path produces an identical payload.
type InvalidConfigIssue struct{}

// NewInvalidConfigIssue creates a new invalid-config issue template.
func NewInvalidConfigIssue() *InvalidConfigIssue { return &InvalidConfigIssue{} }

// BuildIssue decodes the IssueReport.Context bag and delegates to the shared
// builder. Unknown error_kind values fall through to the schema shape since
// "the config has problems" is still useful information.
func (t *InvalidConfigIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	count, _ := strconv.Atoi(context[lite.ContextKeyErrorCount])
	info := lite.IssueInfo{
		Kind:         lite.ErrorKind(context[lite.ContextKeyErrorKind]),
		ConfigPath:   context[lite.ContextKeyConfigPath],
		ErrorMessage: context[lite.ContextKeyErrorMessage],
		Errors:       context[lite.ContextKeyErrors],
		ErrorCount:   count,
		Truncated:    context[lite.ContextKeyTruncated] == "true",
	}
	return lite.BuildInvalidConfigIssue(info), nil
}
