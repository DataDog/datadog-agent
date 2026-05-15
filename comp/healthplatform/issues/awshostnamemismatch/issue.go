// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package awshostnamemismatch

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// AWSHostnameMismatchIssue provides the complete issue template for AWS hostname mismatch issues.
type AWSHostnameMismatchIssue struct{}

// NewAWSHostnameMismatchIssue creates a new AWS hostname mismatch issue template.
func NewAWSHostnameMismatchIssue() *AWSHostnameMismatchIssue {
	return &AWSHostnameMismatchIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps.
func (t *AWSHostnameMismatchIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	configuredHostname := context["configuredHostname"]
	ec2InstanceID := context["ec2InstanceId"]

	description := fmt.Sprintf(
		"The agent's configured hostname '%s' does not match or contain the EC2 instance ID '%s'. Metrics may be attributed to the wrong host.",
		configuredHostname,
		ec2InstanceID,
	)

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "aws_hostname_mismatch",
		Title:       "Agent Hostname Doesn't Match EC2 Instance ID",
		Description: description,
		Category:    "configuration",
		Severity:    "warning",
		Source:      "core-agent",
		Tags:        []string{"aws", "ec2", "hostname", "configuration"},
		Remediation: &healthplatform.Remediation{
			Summary: "Align the agent's configured hostname with the EC2 instance ID to ensure correct metric attribution.",
			Steps: []*healthplatform.RemediationStep{
				{
					Order: 1,
					Text:  "Remove the manual 'hostname' setting from datadog.yaml to let the agent auto-detect the EC2 instance ID as the hostname.",
				},
				{
					Order: 2,
					Text:  fmt.Sprintf("Or set 'hostname: %s' explicitly in datadog.yaml to match the EC2 instance ID.", ec2InstanceID),
				},
				{
					Order: 3,
					Text:  "On Windows, check the 'ec2_use_windows_prefix_detection' setting in datadog.yaml, which may affect how the hostname is resolved.",
				},
				{
					Order: 4,
					Text:  "Verify that the IAM role attached to the EC2 instance has the 'ec2:DescribeInstances' permission if using IMDSv2, or that IMDSv2 is not blocking metadata access.",
				},
			},
		},
	}, nil
}
