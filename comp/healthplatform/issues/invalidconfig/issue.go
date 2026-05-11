// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml problems (unparseable YAML or
// schema violations) through the Agent Health Platform. The periodic in-Fx
// check and the rescue path in pkg/config/lite share the same issue payload.
package invalidconfig

import (
	"strconv"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/pkg/config/lite"
)

// InvalidConfigIssue is a thin template; the issue content lives in
// pkg/config/lite so the rescue path produces an identical payload.
type InvalidConfigIssue struct{}

func NewInvalidConfigIssue() *InvalidConfigIssue { return &InvalidConfigIssue{} }

// BuildIssue delegates to the shared builder. Unknown error_kind values fall
// through to the schema shape since "the config has problems" is still useful.
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
