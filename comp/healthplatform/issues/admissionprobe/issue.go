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
	issueName = IssueName
	issueType = IssueType
	category  = "availability"
	location  = "admission-controller"
	severity  = healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	source    = "cluster-agent"
)

// Cause values for the "cause" context key. They select which remediation
// steps are the most useful, since the probe usually knows more about the
// root cause than "something is wrong" by the time it reports an issue.
const (
	// CauseSecretControllerFailed means the cluster agent's own certificate
	// controller reported an error on its last reconciliation attempt, for a
	// reason other than an RBAC denial.
	CauseSecretControllerFailed = "secret_controller_failed"
	// CauseSecretControllerForbidden means the certificate controller's last
	// reconciliation attempt failed because the Kubernetes API server denied
	// the request (RBAC).
	CauseSecretControllerForbidden = "secret_controller_forbidden"
	// CauseWebhookControllerFailed means the cluster agent's own webhook
	// registration controller reported an error on its last reconciliation
	// attempt, for a reason other than an RBAC denial.
	CauseWebhookControllerFailed = "webhook_controller_failed"
	// CauseWebhookControllerForbidden means the webhook registration
	// controller's last reconciliation attempt failed because the
	// Kubernetes API server denied the request (RBAC).
	CauseWebhookControllerForbidden = "webhook_controller_forbidden"
	// CauseWebhookMissing means the webhook configuration was confirmed absent
	// from the Kubernetes API (not merely unreachable).
	CauseWebhookMissing = "webhook_missing"
	// CauseIndeterminate means the probe could not determine whether the
	// webhook configuration exists (e.g. an RBAC error on the lookup).
	CauseIndeterminate = "indeterminate"
	// CauseProbeConfigForbidden means the probe itself could not create its
	// dry-run ConfigMap because the Kubernetes API server denied the request
	// (RBAC). The probe cannot determine whether the webhook is reachable in
	// this case, but the cause is still known and actionable.
	CauseProbeConfigForbidden = "probe_config_forbidden"
)

// AdmissionProbeIssue builds a complete issue for admission webhook connectivity failures.
type AdmissionProbeIssue struct{}

// BuildIssue creates a complete issue with metadata and remediation for admission probe failures.
// Expected context keys:
//   - "issue": a human-readable description of the problem
//   - "remediation": a provider-specific hint for remediation
//   - "cause": one of the Cause* constants, selecting more targeted remediation
//     steps than the generic "webhook missing or network failure" checklist
//   - "fix_hint": a specific, actionable diagnosis of the underlying
//     Kubernetes API error — the exact RBAC permission that was denied for
//     the *Forbidden causes, or a classification of the API error (timeout,
//     conflict, invalid, etc.) for the *Failed causes — so the remediation
//     states precisely what's wrong instead of asking the user to go find
//     information already known
func (t *AdmissionProbeIssue) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	issue := context["issue"]
	if issue == "" {
		issue = "Datadog admission controller is unreachable from the Kubernetes API server."
	}

	remediation := context["remediation"]
	if remediation == "" {
		remediation = "Ensure proper inbound network connectivity to the cluster agent's node on port 8000."
	}

	steps := remediationSteps(context["cause"], remediation, context["fix_hint"])

	extra, err := structpb.NewStruct(map[string]any{
		"impact": "Pod mutations (config injection, library injection, standard tags) may not be applied to new pods",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create extra: %v", err)
	}

	return &healthplatform.Issue{
		Id:          IssueID,
		IssueName:   issueName,
		IssueType:   issueType,
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

// remediationSteps returns the remediation steps for the given cause. When
// the probe knows the specific reason the webhook is unreachable, the steps
// point directly at that reason instead of asking users to check things the
// probe has already ruled out. fixHint, when set, is that specific diagnosis
// — the exact RBAC permission denied for the *Forbidden causes, or a
// classification of the underlying Kubernetes API error for the *Failed
// causes — and replaces a generic "go check the logs" step with the real
// answer.
func remediationSteps(cause, remediation, fixHint string) []*healthplatform.RemediationStep {
	switch cause {
	case CauseSecretControllerForbidden:
		grant := fixHint
		if grant == "" {
			grant = "Grant the cluster agent's Kubernetes service account get/create/update permissions on Secrets in its namespace"
		}
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: grant},
			{Order: 2, Text: "The secret controller will retry automatically once permissions are fixed; restart the cluster agent to force an immediate retry"},
			{Order: 3, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	case CauseSecretControllerFailed:
		diagnosis := fixHint
		if diagnosis == "" {
			diagnosis = "The specific secret controller error is included in the issue description above"
		}
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: diagnosis},
			{Order: 2, Text: "The secret controller will retry automatically once the underlying issue is fixed; restart the cluster agent to force an immediate retry"},
			{Order: 3, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	case CauseWebhookControllerForbidden:
		grant := fixHint
		if grant == "" {
			grant = "Grant the cluster agent's Kubernetes service account get/create/update permissions on MutatingWebhookConfigurations and ValidatingWebhookConfigurations"
		}
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: grant},
			{Order: 2, Text: "The webhook controller will retry automatically once permissions are fixed; restart the cluster agent to force an immediate retry"},
			{Order: 3, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	case CauseWebhookControllerFailed:
		diagnosis := fixHint
		if diagnosis == "" {
			diagnosis = "The specific webhook controller error is included in the issue description above"
		}
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: diagnosis},
			{Order: 2, Text: "The webhook controller will retry automatically once the underlying issue is fixed; restart the cluster agent to force an immediate retry"},
			{Order: 3, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	case CauseWebhookMissing:
		// The issue description already names the missing webhook configuration
		// and states that both controllers last reconciled without error, so the
		// object was most likely deleted after being created rather than never
		// created — there's nothing left to "go check", only to recreate it.
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: "The webhook configuration named in the issue description above was deleted after being created; nothing else in the cluster agent detected an error, so this is most likely an external deletion (e.g. by another controller, an admission policy, or a manual kubectl delete)"},
			{Order: 2, Text: "Restart the cluster agent to force the webhook controller to recreate the webhook configuration immediately, rather than waiting for its next reconcile"},
			{Order: 3, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	case CauseProbeConfigForbidden:
		grant := fixHint
		if grant == "" {
			grant = "Grant the cluster agent's Kubernetes service account \"create\" permission on ConfigMaps in its own namespace"
		}
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: grant},
			{Order: 2, Text: "This does not mean the admission webhook itself is broken — the probe cannot verify webhook connectivity until this permission is restored"},
			{Order: 3, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	case CauseIndeterminate:
		diagnosis := fixHint
		if diagnosis == "" {
			diagnosis = "The specific error encountered while looking up the webhook configuration is included in the issue description above"
		}
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: diagnosis},
			{Order: 2, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	default:
		// The webhook configuration is confirmed registered and the cluster
		// agent's own controllers report no errors, so the most likely
		// explanation is a network problem between the API server and the
		// cluster agent's admission webhook endpoint.
		return []*healthplatform.RemediationStep{
			{Order: 1, Text: "The webhook configuration is registered and the secret/webhook controllers report no error, so the API server is reaching the endpoint incorrectly or not at all; run 'datadog-cluster-agent status' and check the 'Webhook running' line in the admissionWebhook section for the ValidatingWebhookError/MutatingWebhookError/SecretError fields"},
			{Order: 2, Text: "Verify the cluster agent service is reachable from the Kubernetes API server on port 8000 (network policies, firewalls, or a misconfigured service/endpoint are the most common causes)"},
			{Order: 3, Text: remediation},
			{Order: 4, Text: "See docs: https://dtdg.co/4eZW0g4"},
		}
	}
}
