// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package store

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// ProtoToIssue converts a HealthIssueReport received over gRPC into a
// healthplatform.Issue ready for storage. No template lookup is performed.
func ProtoToIssue(in *pb.HealthIssueReport) *healthplatformpayload.Issue {
	issue := &healthplatformpayload.Issue{
		Id:          in.IssueId,
		Title:       in.Title,
		Description: in.Description,
		Severity:    in.Severity,
		Category:    in.Category,
		Source:      in.Source,
		Location:    in.Location,
		Tags:        in.Tags,
	}

	if in.RemediationSummary != "" || len(in.RemediationSteps) > 0 {
		steps := make([]*healthplatformpayload.RemediationStep, 0, len(in.RemediationSteps))
		for i, text := range in.RemediationSteps {
			steps = append(steps, &healthplatformpayload.RemediationStep{
				Order: int32(i + 1),
				Text:  text,
			})
		}
		issue.Remediation = &healthplatformpayload.Remediation{
			Summary: in.RemediationSummary,
			Steps:   steps,
		}
	}

	return issue
}
