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

const contextKeyIMDSAddress = "imds_address"

// contextKeyHostnameConfigured is "true" when DD_HOSTNAME is set, which restores hostname but not other IMDS data.
const contextKeyHostnameConfigured = "hostname_configured"

// AWSIMDSIssue provides the complete issue template for AWS IMDS hop limit problems
type AWSIMDSIssue struct{}

// NewAWSIMDSIssue creates a new AWS IMDS issue template
func NewAWSIMDSIssue() *AWSIMDSIssue {
	return &AWSIMDSIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation
func (t *AWSIMDSIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	imdsAddr := context[contextKeyIMDSAddress]
	if imdsAddr == "" {
		imdsAddr = imdsAddress
	}

	// A configured hostname keeps hostname resolution working, but tags/credentials/metadata stay degraded.
	hostnameConfigured := context[contextKeyHostnameConfigured] == "true"
	severity := healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	impact := "The agent cannot retrieve EC2 instance metadata (hostname, host aliases, instance tags, IAM role credentials, and instance/network metadata), degrading host identification and cloud tagging across metrics, logs, and traces."
	if hostnameConfigured {
		severity = healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
		impact = "DD_HOSTNAME is set so the hostname is resolved, but the agent still cannot retrieve EC2 host aliases, instance tags, IAM role credentials, and instance/network metadata, degrading cloud tagging across metrics, logs, and traces."
	}

	issueExtra, err := structpb.NewStruct(map[string]any{
		contextKeyIMDSAddress: imdsAddr,
		"impact":              impact,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   IssueName,
		IssueType:   IssueType,
		Title:       "AWS IMDSv2 Unreachable from Container (Hop Limit Too Low)",
		Description: "The Datadog Agent is running inside a container on AWS EC2 (commonly seen on ECS) but cannot reach the instance metadata service (IMDS) at 169.254.169.254. This is typically caused by the default IMDSv2 hop limit of 1: the metadata request must traverse an extra network hop from the container to the host, but the packet's TTL expires before it arrives. The endpoint can also be locked down by an IAM-role-injection tool such as Kube2IAM or kiam. Because IMDS is unreachable, the agent cannot retrieve EC2 instance metadata — the private DNS hostname, host aliases, EC2 instance tags (ec2_collect_tags), IAM role credentials, and instance/network metadata — degrading host identification and cloud tagging across metrics, logs, and traces.",
		Category:    "connectivity",
		Location:    "core-agent",
		Severity:    severity,
		DetectedAt:  "", // Filled by health platform
		Source:      "core",
		Extra:       issueExtra,
		Remediation: t.buildRemediation(),
		Tags:        []string{"aws", "ec2", "imds", "hop-limit", "container", "hostname", "tags"},
	}, nil
}

func (t *AWSIMDSIssue) buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Increase the EC2 IMDSv2 hop limit to 2 to fully restore IMDS access; setting DD_HOSTNAME only restores the hostname, not host tags or instance metadata",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "If access to 169.254.169.254 is being blocked by Kube2IAM or kiam (used to assign IAM roles to pods), update its configuration to allow the Agent to reach this endpoint."},
			{Order: 2, Text: "RECOMMENDED: Otherwise, increase the IMDSv2 hop limit to at least 2 on the EC2 instance (run on the host, not inside the container). Note this permits other containers on the host to reach IMDS as well, which may have security implications:"},
			{Order: 3, Text: `if ! TOKEN=$(curl -sf -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600" --max-time 5) || [ -z "$TOKEN" ]; then
    echo "ERROR: Could not fetch IMDSv2 token. Run this script on the EC2 host, not inside a container." >&2
    exit 1
fi

if ! INSTANCE_ID=$(curl -sf "http://169.254.169.254/latest/meta-data/instance-id" -H "X-aws-ec2-metadata-token: $TOKEN" --max-time 5) || [ -z "$INSTANCE_ID" ]; then
    echo "ERROR: Could not fetch EC2 instance ID using the IMDSv2 token." >&2
    exit 1
fi

if ! REGION=$(curl -sf "http://169.254.169.254/latest/meta-data/placement/region" -H "X-aws-ec2-metadata-token: $TOKEN" --max-time 5) || [ -z "$REGION" ]; then
    echo "ERROR: Could not fetch EC2 region using the IMDSv2 token." >&2
    exit 1
fi`},
			{Order: 4, Text: "aws ec2 modify-instance-metadata-options --region \"$REGION\" --instance-id \"$INSTANCE_ID\" --http-put-response-hop-limit 2 --http-endpoint enabled"},
			{Order: 5, Text: "Restart the Datadog Agent container so it can reach IMDS and pick up the EC2 hostname, tags, and metadata."},
			{Order: 6, Text: "PARTIAL (hostname only): the following options restore the hostname but do NOT recover EC2 instance tags, IAM role credentials, or other IMDS metadata, which still require IMDS reachability."},
			{Order: 7, Text: "PARTIAL (EKS): use the hostname discovered by cloud-init instead of querying IMDS, by setting providers.eks.ec2.useHostnameFromFile to true."},
			{Order: 8, Text: "PARTIAL: run the Agent in the host's UTS namespace so it sees the host's real hostname, by setting agents.useHostNetwork to true."},
			{Order: 9, Text: "PARTIAL: set DD_HOSTNAME explicitly in the container to bypass IMDS for hostname resolution, e.g. via the Kubernetes Downward API:\n  env:\n    - name: DD_HOSTNAME\n      valueFrom:\n        fieldRef:\n          fieldPath: spec.nodeName"},
			{Order: 10, Text: "PARTIAL: for Agent 7.42+, trust the in-container UTS hostname by setting DD_HOSTNAME_TRUST_UTS_NAMESPACE=true (only use if the container hostname is meaningful)."},
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
