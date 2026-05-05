// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissionprobe

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	issueName = "admission_controller_unreachable"
	category  = "availability"
	location  = "admission-controller"
	severity  = "high"
	source    = "cluster-agent"
)

// AdmissionProbeIssue builds a complete issue for admission webhook connectivity failures.
type AdmissionProbeIssue struct{}

// BuildIssue creates a complete issue with metadata and remediation for admission probe failures.
// Expected context keys:
//   - "issue": a human-readable description of the problem
//   - "remediation": a provider-specific hint for remediation
func (t *AdmissionProbeIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	issue := context["issue"]
	if issue == "" {
		issue = "Datadog admission controller is unreachable from the Kubernetes API server."
	}

	remediation := context["remediation"]
	if remediation == "" {
		remediation = "Ensure proper inbound network connectivity to the cluster agent's node on port 8000."
	}

	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: "Check the cluster agent logs for admission controller errors"},
		{Order: 2, Text: "Verify the cluster agent service is reachable from the Kubernetes API server on port 8000"},
		{Order: 3, Text: "Verify the MutatingWebhookConfiguration exists and has the correct service reference: kubectl get mutatingwebhookconfigurations"},
		{Order: 4, Text: remediation},
		{Order: 5, Text: "See docs: https://docs.datadoghq.com/containers/troubleshooting/admission-controller"},
	}

	extra, err := structpb.NewStruct(map[string]any{
		"issue":       issue,
		"remediation": remediation,
		"impact":      "Pod mutations (config injection, library injection, standard tags) may not be applied to new pods",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		Title:       "Admission Controller Unreachable",
		Description: issue,
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Remediation: &healthplatform.Remediation{
			Summary: "Verify network connectivity between the Kubernetes API server and the cluster agent admission webhook",
			Steps:   steps,
		},
		Tags: []string{"admission-controller", "connectivity", "cluster-agent"},
	}, nil
}
