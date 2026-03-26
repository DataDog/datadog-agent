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
		Id:          IssueID,
		IssueName:   "aws_imds_hop_limit",
		Title:       "AWS IMDSv2 Unreachable from Container (Hop Limit Too Low)",
		Description: "The Datadog Agent is running inside a container on AWS EC2 but cannot reach the instance metadata service (IMDS) at 169.254.169.254. This is typically caused by the default IMDSv2 hop limit of 1: the metadata request needs to traverse an extra network hop from the container to the host, but the packet's TTL expires before it arrives. As a result, the agent cannot resolve the EC2 hostname, which breaks host-level data correlation in Datadog.",
		Category:    "connectivity",
		Location:    "core-agent",
		Severity:    "high",
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
			{Order: 1, Text: "RECOMMENDED: Increase the IMDSv2 hop limit to 2 on the EC2 instance (run on the host, not inside the container):"},
			{Order: 2, Text: "INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)"},
			{Order: 3, Text: "aws ec2 modify-instance-metadata-options --instance-id \"$INSTANCE_ID\" --http-put-response-hop-limit 2 --http-endpoint enabled"},
			{Order: 4, Text: "Restart the Datadog Agent container to pick up the correct EC2 hostname."},
			{Order: 5, Text: "ALTERNATIVE: Set DD_HOSTNAME explicitly in the container to bypass IMDS entirely:"},
			{Order: 6, Text: "For Kubernetes, inject the node name via the Downward API:\n  env:\n    - name: DD_HOSTNAME\n      valueFrom:\n        fieldRef:\n          fieldPath: spec.nodeName"},
			{Order: 7, Text: "ALTERNATIVE: For Agent 7.42+, trust the in-container UTS hostname by setting DD_HOSTNAME_TRUST_UTS_NAMESPACE=true (only use if the container hostname is meaningful)."},
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
