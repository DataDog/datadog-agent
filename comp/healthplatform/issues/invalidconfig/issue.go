// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml problems (unparseable YAML or
// schema violations) through the Agent Health Platform. The periodic in-Fx
// check and the rescue path in pkg/config/lite share the same issue payload.
package invalidconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/pkg/config/lite"
)

// InvalidConfigIssue is a stateless template that delegates to lite so the
// rescue path produces an identical payload.
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context bag and builds the Issue.
func (InvalidConfigIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return lite.BuildInvalidConfigIssue(lite.IssueInfoFromContext(context)), nil
}
