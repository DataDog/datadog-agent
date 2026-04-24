// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package k8srbac

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// K8sRBACIssue provides the issue template for Kubernetes RBAC 403 forbidden errors
type K8sRBACIssue struct{}

// NewK8sRBACIssue creates a new Kubernetes RBAC issue template
func NewK8sRBACIssue() *K8sRBACIssue {
	return &K8sRBACIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps
func (t *K8sRBACIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	endpoint := context["endpoint"]
	if endpoint == "" {
		endpoint = "unknown"
	}

	resource := context["resource"]
	if resource == "" {
		resource = "unknown"
	}

	verb := context["verb"]
	if verb == "" {
		verb = "unknown"
	}

	extra, err := structpb.NewStruct(map[string]any{
		"endpoint": endpoint,
		"resource": resource,
		"verb":     verb,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "k8s_rbac_forbidden",
		Title:       "Agent Lacks Kubernetes RBAC Permissions",
		Description: fmt.Sprintf("The Datadog agent received a 403 Forbidden response when accessing the Kubernetes API or kubelet endpoint %s. The agent's ServiceAccount is missing RBAC permissions for %s on %s.", endpoint, verb, resource),
		Category:    "permissions",
		Location:    "kubelet",
		Severity:    "high",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "kubernetes",
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Grant the agent's ServiceAccount the missing RBAC permissions via ClusterRole",
			Steps: []*healthplatform.RemediationStep{
				{Order: 1, Text: fmt.Sprintf("Check the agent's ClusterRole: kubectl describe clusterrole datadog-agent")},
				{Order: 2, Text: fmt.Sprintf("Verify the ClusterRoleBinding: kubectl describe clusterrolebinding datadog-agent")},
				{Order: 3, Text: fmt.Sprintf("Add the missing permission to the ClusterRole for %s on %s", verb, resource)},
				{Order: 4, Text: "Apply the official Datadog Helm chart or Operator which includes the correct RBAC"},
				{Order: 5, Text: "Check the ServiceAccount binding: kubectl get clusterrolebinding | grep datadog"},
			},
		},
		Tags: []string{"kubernetes", "rbac", "permissions", "kubelet"},
	}, nil
}
