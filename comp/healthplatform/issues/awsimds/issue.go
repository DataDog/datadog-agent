// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package awsimds

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

//go:embed fix-aws-imds-hop-limit.sh
var fixScript string

const imdsAddress = "169.254.169.254:80"

// AWSIMDSIssue provides the complete issue template for AWS IMDS hop limit problems
type AWSIMDSIssue struct{}

// NewAWSIMDSIssue creates a new AWS IMDS issue template
func NewAWSIMDSIssue() *AWSIMDSIssue {
	return &AWSIMDSIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation
func (t *AWSIMDSIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	imdsAddr := context["imds_address"]
	if imdsAddr == "" {
		imdsAddr = imdsAddress
	}

	issueExtra, err := structpb.NewStruct(map[string]any{
		"imds_address": imdsAddr,
		"impact":       "The agent cannot determine the EC2 instance hostname, which prevents proper host-level data correlation in Datadog",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       "AWS IMDSv2 Unreachable from Container (Hop Limit Too Low)",
		Description: "The Datadog Agent is running inside a container on AWS EC2 (commonly seen on ECS) but cannot reach the instance metadata service (IMDS) at 169.254.169.254. This is typically caused by the default IMDSv2 hop limit of 1: the metadata request needs to traverse an extra network hop from the container to the host, but the packet's TTL expires before it arrives. The endpoint can also be unreachable because it has been deliberately locked down by an IAM-role-injection tool such as Kube2IAM or kiam. As a result, the agent cannot resolve the EC2 hostname, which breaks host-level data correlation in Datadog.",
		Category:    "connectivity",
		Location:    "core-agent",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		DetectedAt:  "", // Filled by health platform
		Source:      "core",
		Extra:       issueExtra,
		Remediation: t.buildRemediation(),
		Tags:        []string{"aws", "ec2", "imds", "hop-limit", "container", "hostname"},
	}, nil
}

func (t *AWSIMDSIssue) buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Increase the EC2 IMDSv2 hop limit to 2, or configure the hostname explicitly in the agent",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "If access to 169.254.169.254 is being blocked by Kube2IAM or kiam (used to assign IAM roles to pods), update its configuration to allow the Agent to reach this endpoint."},
			{Order: 2, Text: "RECOMMENDED: Otherwise, increase the IMDSv2 hop limit to at least 2 on the EC2 instance (run on the host, not inside the container). Note this permits other containers on the host to reach IMDS as well, which may have security implications:"},
			{Order: 3, Text: "INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)"},
			{Order: 4, Text: "aws ec2 modify-instance-metadata-options --instance-id \"$INSTANCE_ID\" --http-put-response-hop-limit 2 --http-endpoint enabled"},
			{Order: 5, Text: "Restart the Datadog Agent container to pick up the correct EC2 hostname."},
			{Order: 6, Text: "ALTERNATIVE (EKS): use the hostname discovered by cloud-init instead of querying IMDS, by setting providers.eks.ec2.useHostnameFromFile to true."},
			{Order: 7, Text: "ALTERNATIVE: run the Agent in the host's UTS namespace so it sees the host's real hostname, by setting agents.useHostNetwork to true."},
			{Order: 8, Text: "ALTERNATIVE: set DD_HOSTNAME explicitly in the container to bypass IMDS entirely, e.g. via the Kubernetes Downward API:\n  env:\n    - name: DD_HOSTNAME\n      valueFrom:\n        fieldRef:\n          fieldPath: spec.nodeName"},
			{Order: 9, Text: "ALTERNATIVE: for Agent 7.42+, trust the in-container UTS hostname by setting DD_HOSTNAME_TRUST_UTS_NAMESPACE=true (only use if the container hostname is meaningful)."},
		},
		Script: &healthplatform.Script{
			Language:        "bash",
			LanguageVersion: "4.0+",
			Filename:        "fix-aws-imds-hop-limit.sh",
			RequiresRoot:    false,
			Content:         fixScript,
		},
	}
}
