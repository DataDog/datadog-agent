// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package clusteragentunreachable

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName = IssueName
	issueType = IssueType
)

const (
	contextKeyError        = "error"
	contextKeyFailingSince = "failing_since"
	contextKeyHostname     = "hostname"
)

const (
	docsURL = "https://docs.datadoghq.com/containers/troubleshooting/cluster-agent/"
	impact  = "Features that rely on the Cluster Agent, such as cluster checks, endpoints checks, and cluster-level Kubernetes tags (kube_service, namespace labels and annotations), are degraded or unavailable on this agent."
)

// ClusterAgentUnreachableIssue builds the issue for an agent that cannot reach the Cluster Agent.
type ClusterAgentUnreachableIssue struct{}

// BuildIssue creates the issue from the check context.
func (ClusterAgentUnreachableIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	errMsg := context[contextKeyError]
	if errMsg == "" {
		errMsg = "unknown error"
	}
	failingSince := context[contextKeyFailingSince]
	if failingSince == "" {
		failingSince = "unknown"
	}
	hostname := context[contextKeyHostname]
	if hostname == "" {
		hostname = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		contextKeyError:        errMsg,
		contextKeyFailingSince: failingSince,
		contextKeyHostname:     hostname,
		"impact":               impact,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		IssueName:   issueName,
		IssueType:   issueType,
		Title:       fmt.Sprintf("Agent on '%s' cannot reach the Cluster Agent", hostname),
		Description: fmt.Sprintf("The agent on %s has been unable to connect to the Datadog Cluster Agent since %s. %s Last error: %s", hostname, failingSince, impact, errMsg),
		Category:    "connectivity",
		Location:    "cluster-agent-client",
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH,
		Source:      "agent",
		Extra:       extra,
		Remediation: buildRemediation(errMsg),
		Tags:        []string{"cluster-agent", "connectivity"},
	}, nil
}

func buildRemediation(errMsg string) *healthplatform.Remediation {
	texts := []string{
		"Run `agent status` in this agent's container and check the Datadog Cluster Agent section for the connection error",
		"Verify the Cluster Agent pods are running and ready (`kubectl get pods -A | grep cluster-agent`) and check their logs for errors",
	}
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "x509") || strings.Contains(lower, "certificate") || strings.Contains(lower, "tls") {
		texts = append(texts, "Verify the TLS settings under `cluster_agent` match on this agent and the Cluster Agent")
	}
	texts = append(texts,
		"Verify the Cluster Agent service resolves from this node and that its port (5005 by default) is not blocked by a NetworkPolicy or firewall; the service is set by `DD_CLUSTER_AGENT_URL` or `DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME`",
		"See docs: "+docsURL,
	)

	steps := make([]*healthplatform.RemediationStep, 0, len(texts))
	for i, text := range texts {
		steps = append(steps, &healthplatform.RemediationStep{Order: int32(i + 1), Text: text})
	}
	return &healthplatform.Remediation{
		Summary: "Restore network connectivity between this agent and the Datadog Cluster Agent",
		Steps:   steps,
	}
}
