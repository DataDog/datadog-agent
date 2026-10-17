// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissionprobe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	healthplatform "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/stretchr/testify/require"
)

func TestBuildIssue_BasicFields(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":       "webhook not reachable",
		"remediation": "check firewall rules",
	})

	require.NoError(t, err)
	assert.Equal(t, IssueID, issue.Id)
	assert.Equal(t, issueName, issue.IssueName)
	assert.Equal(t, issueType, issue.IssueType)
	assert.Equal(t, "Admission Controller Unreachable", issue.Title)
	assert.Contains(t, issue.Description, "webhook not reachable")
	assert.Equal(t, "availability", issue.Category)
	assert.Equal(t, "admission-controller", issue.Location)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH, issue.Severity)
	assert.Equal(t, "cluster-agent", issue.Source)
	assert.Contains(t, issue.Tags, "admission-controller")
	assert.Contains(t, issue.Tags, "connectivity")
	assert.Contains(t, issue.Tags, "cluster-agent")
}

func TestBuildIssue_Remediation(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":       "timeout",
		"remediation": "check network",
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.NotEmpty(t, issue.Remediation.Summary)
	assert.Len(t, issue.Remediation.Steps, 4)

	lastStep := issue.Remediation.Steps[len(issue.Remediation.Steps)-1]
	assert.Contains(t, lastStep.Text, "dtdg.co")

	assert.Equal(t, "check network", issue.Remediation.Steps[2].Text)
}

func TestBuildIssue_Defaults(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{})

	require.NoError(t, err)
	assert.Contains(t, issue.Description, "unreachable from the Kubernetes API server")
	assert.Contains(t, issue.Remediation.Steps[1].Text, "port 8000")
}

func TestBuildIssue_Extra(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":       "connection refused",
		"remediation": "allow port 8000",
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Extra)
	assert.NotEmpty(t, issue.Extra.Fields["impact"].GetStringValue())
}

func TestBuildIssue_CauseSecretControllerForbidden(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":    "the cluster agent failed to create the TLS certificate Secret: forbidden",
		"cause":    CauseSecretControllerForbidden,
		"fix_hint": `Grant the cluster agent's Kubernetes service account "create" permission on secrets "datadog-webhook-certificate" in namespace "datadog"`,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	for _, step := range issue.Remediation.Steps {
		assert.NotContains(t, step.Text, "port 8000", "steps should not ask the user to check network connectivity when the secret controller already reported the root cause")
	}
	assert.Equal(t, `Grant the cluster agent's Kubernetes service account "create" permission on secrets "datadog-webhook-certificate" in namespace "datadog"`, issue.Remediation.Steps[0].Text)
}

func TestBuildIssue_CauseSecretControllerForbidden_NoRBACFixFallsBackToGenericGrant(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": "the cluster agent failed to create the TLS certificate Secret: forbidden",
		"cause": CauseSecretControllerForbidden,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.Contains(t, issue.Remediation.Steps[0].Text, "Secrets")
}

func TestBuildIssue_CauseSecretControllerFailed(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":    "the cluster agent failed to create the TLS certificate Secret: connection refused",
		"cause":    CauseSecretControllerFailed,
		"fix_hint": "The Kubernetes API server timed out handling the request; check network connectivity and API server load between the cluster agent and the control plane",
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	for _, step := range issue.Remediation.Steps {
		assert.NotContains(t, step.Text, "port 8000", "steps should not ask the user to check network connectivity when the secret controller already reported the root cause")
		assert.NotContains(t, step.Text, "RBAC", "steps should not guess RBAC as the cause when the error is not a Forbidden error")
	}
	assert.Equal(t, "The Kubernetes API server timed out handling the request; check network connectivity and API server load between the cluster agent and the control plane", issue.Remediation.Steps[0].Text)
}

func TestBuildIssue_CauseSecretControllerFailed_NoFixHintPointsAtDescriptionInsteadOfLogs(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": "the cluster agent failed to create the TLS certificate Secret: connection refused",
		"cause": CauseSecretControllerFailed,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.NotContains(t, issue.Remediation.Steps[0].Text, "logs", "the error is already in the issue description, so the fallback shouldn't ask the user to go find it again in the logs")
}

func TestBuildIssue_CauseWebhookControllerForbidden(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":    "the cluster agent failed to register the webhook configuration: forbidden",
		"cause":    CauseWebhookControllerForbidden,
		"fix_hint": `Grant the cluster agent's Kubernetes service account "create" permission on the cluster-scoped mutatingwebhookconfigurations "datadog-webhook"`,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	for _, step := range issue.Remediation.Steps {
		assert.NotContains(t, step.Text, "port 8000", "steps should not ask the user to check network connectivity when the webhook controller already reported the root cause")
	}
	assert.Equal(t, `Grant the cluster agent's Kubernetes service account "create" permission on the cluster-scoped mutatingwebhookconfigurations "datadog-webhook"`, issue.Remediation.Steps[0].Text)
}

func TestBuildIssue_CauseWebhookControllerForbidden_NoRBACFixFallsBackToGenericGrant(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": "the cluster agent failed to register the webhook configuration: forbidden",
		"cause": CauseWebhookControllerForbidden,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.Contains(t, issue.Remediation.Steps[0].Text, "WebhookConfigurations")
}

func TestBuildIssue_CauseWebhookControllerFailed(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":    "the cluster agent failed to register the webhook configuration: connection refused",
		"cause":    CauseWebhookControllerFailed,
		"fix_hint": "The Kubernetes API server timed out handling the request; check network connectivity and API server load between the cluster agent and the control plane",
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	for _, step := range issue.Remediation.Steps {
		assert.NotContains(t, step.Text, "port 8000", "steps should not ask the user to check network connectivity when the webhook controller already reported the root cause")
		assert.NotContains(t, step.Text, "RBAC", "steps should not guess RBAC as the cause when the error is not a Forbidden error")
	}
	assert.Equal(t, "The Kubernetes API server timed out handling the request; check network connectivity and API server load between the cluster agent and the control plane", issue.Remediation.Steps[0].Text)
}

func TestBuildIssue_CauseWebhookControllerFailed_NoFixHintPointsAtDescriptionInsteadOfLogs(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": "the cluster agent failed to register the webhook configuration: connection refused",
		"cause": CauseWebhookControllerFailed,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.NotContains(t, issue.Remediation.Steps[0].Text, "logs", "the error is already in the issue description, so the fallback shouldn't ask the user to go find it again in the logs")
}

func TestBuildIssue_CauseWebhookMissing(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": `The admission webhook configuration "datadog-webhook" does not exist in the cluster. Both the secret controller and the webhook controller last reconciled successfully, so the object was most likely deleted after being created rather than never created.`,
		"cause": CauseWebhookMissing,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	for _, step := range issue.Remediation.Steps {
		assert.NotContains(t, step.Text, "port 8000", "steps should not ask the user to check network connectivity when the webhook is confirmed missing")
		assert.NotContains(t, step.Text, "kubectl get", "the probe already confirmed the webhook is absent; steps shouldn't ask the user to re-verify that")
	}
	assert.Contains(t, issue.Remediation.Steps[0].Text, "deleted after being created")
}

func TestBuildIssue_CauseIndeterminate(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":    `Could not determine whether the admission webhook configuration "datadog-webhook" exists: checking mutating webhook configuration: "system:serviceaccount:datadog:datadog-cluster-agent" is missing "get" permission on mutatingwebhookconfigurations "datadog-webhook": forbidden.`,
		"cause":    CauseIndeterminate,
		"fix_hint": `Grant service account "system:serviceaccount:datadog:datadog-cluster-agent" "get" permission on the cluster-scoped mutatingwebhookconfigurations "datadog-webhook"`,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	for _, step := range issue.Remediation.Steps {
		assert.NotContains(t, step.Text, "port 8000", "steps should not claim a network diagnosis when existence could not be determined")
	}
	assert.Equal(t, `Grant service account "system:serviceaccount:datadog:datadog-cluster-agent" "get" permission on the cluster-scoped mutatingwebhookconfigurations "datadog-webhook"`, issue.Remediation.Steps[0].Text)
}

func TestBuildIssue_CauseIndeterminate_NoFixHintPointsAtDescriptionInsteadOfLogs(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": "could not determine whether the webhook configuration exists",
		"cause": CauseIndeterminate,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.NotContains(t, issue.Remediation.Steps[0].Text, "logs", "the error is already in the issue description, so the fallback shouldn't ask the user to go find it again in the logs")
}

func TestBuildIssue_CauseProbeConfigForbidden(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue":    "the cluster agent cannot verify admission webhook connectivity: forbidden",
		"cause":    CauseProbeConfigForbidden,
		"fix_hint": `Grant service account "system:serviceaccount:datadog:datadog-cluster-agent" "create" permission on configmaps "" in namespace "datadog"`,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.Equal(t, `Grant service account "system:serviceaccount:datadog:datadog-cluster-agent" "create" permission on configmaps "" in namespace "datadog"`, issue.Remediation.Steps[0].Text)
	assert.Contains(t, issue.Remediation.Steps[1].Text, "does not mean the admission webhook itself is broken")
}

func TestBuildIssue_CauseProbeConfigForbidden_NoRBACFixFallsBackToGenericGrant(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"issue": "the cluster agent cannot verify admission webhook connectivity: forbidden",
		"cause": CauseProbeConfigForbidden,
	})

	require.NoError(t, err)
	require.NotNil(t, issue.Remediation)
	assert.Contains(t, issue.Remediation.Steps[0].Text, "ConfigMaps")
}

func TestBuildIssue_UnknownCauseFallsBackToDefaultSteps(t *testing.T) {
	template := &AdmissionProbeIssue{}
	issue, err := template.BuildIssue(map[string]string{
		"cause": "not_a_real_cause",
	})

	require.NoError(t, err)
	assert.Len(t, issue.Remediation.Steps, 4)
}

func TestNewModule(t *testing.T) {
	m := NewModule(issues.ModuleDeps{})
	assert.Equal(t, IssueName, m.IssueName())
	issue, err := m.BuildIssue(map[string]string{})
	require.NoError(t, err)
	assert.NotNil(t, issue)
	assert.Nil(t, m.BuiltInPeriodicHealthCheck())
	assert.Nil(t, m.BuiltInStartupHealthCheck())
}
