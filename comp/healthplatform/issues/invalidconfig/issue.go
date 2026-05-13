// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/pkg/config/lite"
)

// InvalidConfigIssue uses the payload that lives in pkg/config/lite
type InvalidConfigIssue struct{}

// BuildIssue decodes the IssueReport.Context and builds the Issue.
func (InvalidConfigIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return lite.BuildInvalidConfigIssue(lite.IssueInfoFromContext(context)), nil
}
