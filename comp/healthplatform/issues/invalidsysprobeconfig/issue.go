// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidsysprobeconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"
)

// InvalidSysprobeConfigIssue is the template for "invalid-system-probe-config" issues.
type InvalidSysprobeConfigIssue struct{}

// BuildIssue uses the same explanation and fix as core configuration issues.
func (InvalidSysprobeConfigIssue) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	issue := invalidconfig.BuildIssue(ctx, "system-probe")
	issue.IssueName = IssueName
	issue.IssueType = IssueType
	issue.Tags = append(issue.Tags, "system-probe")
	return issue, nil
}
