// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package backendconnectivity

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// BackendConnectivityIssue provides the complete issue template (metadata + remediation).
type BackendConnectivityIssue struct{}

// NewBackendConnectivityIssue creates a new backend connectivity issue template.
func NewBackendConnectivityIssue() *BackendConnectivityIssue {
	return &BackendConnectivityIssue{}
}

// BuildIssue creates a complete issue with metadata and remediation steps.
func (t *BackendConnectivityIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	endpoint := context["endpoint"]
	if endpoint == "" {
		endpoint = "unknown endpoint"
	}

	errMsg := context["error"]
	if errMsg == "" {
		errMsg = "unknown error"
	}

	issueExtra, err := structpb.NewStruct(map[string]any{
		"endpoint": endpoint,
		"error":    errMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create issue extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   "backend_connectivity_failure",
		Title:       "Agent Cannot Reach Datadog Backend",
		Description: fmt.Sprintf("The agent failed to connect to the Datadog intake endpoint %s: %s. This typically indicates a firewall rule blocking outbound traffic, a proxy misconfiguration, an incorrect site setting, or a DNS resolution failure.", endpoint, errMsg),
		Category:    "network",
		Location:    "forwarder",
		Severity:    "high",
		DetectedAt:  "", // Will be filled by health platform
		Source:      "forwarder",
		Extra:       issueExtra,
		Remediation: buildRemediation(),
		Tags:        []string{"network", "connectivity", "forwarder"},
	}, nil
}

// buildRemediation creates remediation steps for backend connectivity failures.
func buildRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Restore network connectivity between the agent and Datadog intake endpoints",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "Check firewall rules: ensure outbound TCP port 443 is allowed to *.datadoghq.com (or your configured site)."},
			{Order: 2, Text: "Verify proxy configuration: if using a proxy, check that 'proxy.http' and 'proxy.https' are correctly set in datadog.yaml."},
			{Order: 3, Text: "Check the 'site' configuration key in datadog.yaml (e.g. datadoghq.com, datadoghq.eu, us3.datadoghq.com)."},
			{Order: 4, Text: "Verify DNS resolution: run 'nslookup app.<site>' to confirm the intake hostname resolves correctly."},
			{Order: 5, Text: "If operating in a restricted network environment, consider whether Datadog PrivateLink is required for your setup."},
		},
	}
}
