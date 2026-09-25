// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package probe periodically tests admission webhook connectivity by sending
// dry-run ConfigMap creation requests through the Kubernetes API server.
package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/admissionprobe"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var errProbeNotReceived = errors.New("dry-run probe configmap was not annotated by the webhook")

// reconcileStatusProvider reports the outcome of the most recent reconciliation
// attempt of a controller. Implemented by the secret and webhook controllers.
type reconcileStatusProvider interface {
	LastReconcileError() error
}

// Probe periodically verifies that the admission webhook is reachable by
// creating dry-run ConfigMaps and checking if they are handled by the webhook.
type Probe struct {
	k8sClient      kubernetes.Interface
	isLeaderFunc   func() bool
	namespace      string
	interval       time.Duration
	gracePeriod    time.Duration
	logLimiter     *log.Limit
	diagnosticHint string

	webhookName       string
	mutationEnabled   bool
	validationEnabled bool

	secretController  reconcileStatusProvider
	webhookController reconcileStatusProvider

	healthPlatform healthplatformdef.Component

	stats Stats
}

// Stats holds probe execution statistics. All fields are protected by mu.
type Stats struct {
	mu                   sync.RWMutex
	TotalExecutions      int64
	SuccessCount         int64
	FailCount            int64
	LastExecutionTime    time.Time
	LastExecutionSuccess bool
	LastExecutionError   string
	LastSuccessTime      time.Time
	ConfigError          string
}

// StatsSnapshot is a point-in-time copy of Stats, safe to read without locks.
type StatsSnapshot struct {
	TotalExecutions      int64
	SuccessCount         int64
	FailCount            int64
	LastExecutionTime    time.Time
	LastExecutionSuccess bool
	LastExecutionError   string
	LastSuccessTime      time.Time
	ConfigError          string
}

const (
	defaultInterval = 60 * time.Second

	healthCheckID   = "admission-controller-connectivity"
	healthCheckName = "Admission Controller Connectivity"
	healthIssueID   = "admission-controller-connectivity-failure"
)

// New creates a new admission controller connectivity probe.
// The namespace parameter specifies where dry-run ConfigMaps are created; this
// should be the namespace the cluster agent is deployed in.
func New(k8sClient kubernetes.Interface, isLeaderFunc func() bool, namespace string, datadogConfig config.Component, healthPlatform healthplatformdef.Component, secretController, webhookController reconcileStatusProvider) *Probe {
	interval := time.Duration(datadogConfig.GetInt("admission_controller.probe.interval")) * time.Second
	if interval <= 0 {
		log.Warnf("admission_controller.probe.interval is invalid (%s), falling back to %s", interval, defaultInterval)
		interval = defaultInterval
	}

	return &Probe{
		k8sClient:         k8sClient,
		isLeaderFunc:      isLeaderFunc,
		namespace:         namespace,
		interval:          interval,
		gracePeriod:       time.Duration(datadogConfig.GetInt("admission_controller.probe.grace_period")) * time.Second,
		logLimiter:        log.NewLogLimit(1, 10*time.Minute),
		healthPlatform:    healthPlatform,
		webhookName:       datadogConfig.GetString("admission_controller.webhook_name"),
		mutationEnabled:   datadogConfig.GetBool("admission_controller.mutation.enabled"),
		validationEnabled: datadogConfig.GetBool("admission_controller.validation.enabled"),
		secretController:  secretController,
		webhookController: webhookController,
	}
}

// GetStatsSnapshot returns a point-in-time copy of the probe statistics.
func (p *Probe) GetStatsSnapshot() StatsSnapshot {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()
	return StatsSnapshot{
		TotalExecutions:      p.stats.TotalExecutions,
		SuccessCount:         p.stats.SuccessCount,
		FailCount:            p.stats.FailCount,
		LastExecutionTime:    p.stats.LastExecutionTime,
		LastExecutionSuccess: p.stats.LastExecutionSuccess,
		LastExecutionError:   p.stats.LastExecutionError,
		LastSuccessTime:      p.stats.LastSuccessTime,
		ConfigError:          p.stats.ConfigError,
	}
}

// IsLeader returns whether this instance is the current leader.
func (p *Probe) IsLeader() bool {
	return p.isLeaderFunc()
}

// GetStatsForStatus returns probe stats formatted for the agent status output.
func (p *Probe) GetStatsForStatus() map[string]interface{} {
	snap := p.GetStatsSnapshot()
	result := map[string]interface{}{}

	if snap.ConfigError != "" {
		result["ConfigError"] = snap.ConfigError
		return result
	}

	var successRate string
	if snap.TotalExecutions > 0 {
		successRate = fmt.Sprintf("%.1f%%", float64(snap.SuccessCount)/float64(snap.TotalExecutions)*100)
	} else {
		successRate = "N/A"
	}

	result["TotalExecutions"] = snap.TotalExecutions
	result["SuccessCount"] = snap.SuccessCount
	result["FailCount"] = snap.FailCount
	result["SuccessRate"] = successRate
	result["LastExecutionTime"] = formatTime(snap.LastExecutionTime)
	result["LastExecutionSuccess"] = snap.LastExecutionSuccess
	result["LastExecutionError"] = snap.LastExecutionError
	result["LastSuccessTime"] = formatTime(snap.LastSuccessTime)
	if p.diagnosticHint != "" {
		result["DiagnosticHint"] = p.diagnosticHint
	}
	return result
}

// Run starts the periodic probe loop. It blocks until ctx is cancelled.
func (p *Probe) Run(ctx context.Context) {
	log.Info("Starting admission controller probe")

	select {
	case <-ctx.Done():
		return
	case <-time.After(p.gracePeriod):
	}

	provider := cloudprovider.DCAGetName(ctx)
	p.diagnosticHint = diagnosticHintForProvider(provider)
	log.Infof("Admission controller probe is now active (namespace=%s, interval=%s)", p.namespace, p.interval)

	// Run the first probe immediately, then on the configured interval.
	if p.isLeaderFunc() {
		p.runProbe(ctx)
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping admission controller probe")
			return
		case <-ticker.C:
			if !p.isLeaderFunc() {
				continue
			}
			p.runProbe(ctx)
		}
	}
}

func (p *Probe) runProbe(ctx context.Context) {
	err := p.execute(ctx)
	now := time.Now()

	p.stats.mu.Lock()
	p.stats.TotalExecutions++
	p.stats.LastExecutionTime = now
	p.stats.ConfigError = ""
	if err == nil {
		p.stats.SuccessCount++
		p.stats.LastExecutionSuccess = true
		p.stats.LastExecutionError = ""
		p.stats.LastSuccessTime = now
	} else {
		p.stats.FailCount++
		p.stats.LastExecutionSuccess = false
		p.stats.LastExecutionError = err.Error()
	}
	p.stats.mu.Unlock()

	if err == nil {
		p.clearHealthIssue()
		return
	}

	p.handleError(ctx, err)
}

func (p *Probe) handleError(ctx context.Context, err error) {
	if k8serrors.IsForbidden(err) {
		wrappedErr := admcommon.WrapIfForbidden(err, "create", "configmaps", p.namespace, "")
		msg := fmt.Sprintf("The cluster agent service account does not have permission to create configmaps in namespace %q: %v", p.namespace, wrappedErr)
		p.stats.mu.Lock()
		p.stats.ConfigError = msg
		p.stats.mu.Unlock()
		if p.logLimiter.ShouldLog() {
			log.Errorf("Admission controller probe misconfigured: %s", msg)
		}
		fixHint := ""
		if fix, ok := rbacFixHint(wrappedErr); ok {
			fixHint = fix
		}
		p.reportHealthIssue(fmt.Sprintf("The cluster agent cannot verify admission webhook connectivity: %s. This does not mean the webhook itself is unreachable.", msg), admissionprobe.CauseProbeConfigForbidden, fixHint)
		return
	}

	if errors.Is(err, errProbeNotReceived) {
		if p.secretController != nil {
			if scErr := p.secretController.LastReconcileError(); scErr != nil {
				if p.logLimiter.ShouldLog() {
					log.Errorf("Admission controller probe failed: the secret controller's last reconciliation attempt failed: %v", scErr)
				}
				cause := admissionprobe.CauseSecretControllerFailed
				fixHint := ""
				if fix, ok := rbacFixHint(scErr); ok {
					cause = admissionprobe.CauseSecretControllerForbidden
					fixHint = fix
				} else if fix, ok := k8sErrorFixHint(scErr); ok {
					fixHint = fix
				}
				p.reportHealthIssue(fmt.Sprintf("The cluster agent failed to create or refresh the TLS certificate Secret used by the admission webhook: %v", scErr), cause, fixHint)
				return
			}
		}

		if p.webhookController != nil {
			if wcErr := p.webhookController.LastReconcileError(); wcErr != nil {
				if p.logLimiter.ShouldLog() {
					log.Errorf("Admission controller probe failed: the webhook controller's last reconciliation attempt failed: %v", wcErr)
				}
				cause := admissionprobe.CauseWebhookControllerFailed
				fixHint := ""
				if fix, ok := rbacFixHint(wcErr); ok {
					cause = admissionprobe.CauseWebhookControllerForbidden
					fixHint = fix
				} else if fix, ok := k8sErrorFixHint(wcErr); ok {
					fixHint = fix
				}
				p.reportHealthIssue(fmt.Sprintf("The cluster agent failed to register the admission webhook configuration with the Kubernetes API server: %v", wcErr), cause, fixHint)
				return
			}
		}

		exists, existsErr := p.webhookExists(ctx)
		if existsErr == nil && !exists {
			if p.logLimiter.ShouldLog() {
				log.Errorf("Admission controller probe failed: webhook configuration %q does not exist. The secret and webhook controllers report no reconciliation error, so it was most likely deleted after being created.", p.webhookName)
			}
			p.reportHealthIssue(fmt.Sprintf("The admission webhook configuration %q does not exist in the cluster. Both the secret controller and the webhook controller last reconciled successfully, so the object was most likely deleted after being created rather than never created.", p.webhookName), admissionprobe.CauseWebhookMissing, "")
			return
		}

		if existsErr != nil {
			if p.logLimiter.ShouldLog() {
				log.Warnf("Admission controller probe: could not determine whether the webhook configuration exists: %v", existsErr)
			}
			cause := admissionprobe.CauseIndeterminate
			fixHint := ""
			if fix, ok := rbacFixHint(existsErr); ok {
				fixHint = fix
			} else if fix, ok := k8sErrorFixHint(existsErr); ok {
				fixHint = fix
			}
			p.reportHealthIssue(fmt.Sprintf("Could not determine whether the admission webhook configuration %q exists: %v.", p.webhookName, existsErr), cause, fixHint)
			return
		}

		if p.logLimiter.ShouldLog() {
			log.Errorf(
				"Admission controller probe failed: the webhook did not handle the probe configmap. "+
					"This indicates a network connectivity issue between the Kubernetes API server "+
					"and the cluster agent admission webhook. %s",
				p.diagnosticHint,
			)
		}
		p.reportHealthIssue("", "", "")
		return
	}

	if p.logLimiter.ShouldLog() {
		log.Errorf("Admission controller probe failed: %v", err)
	}
	p.reportHealthIssue("", "", "")
}

// rbacFixHint reports the exact RBAC permission that was denied, if err
// carries that information (see admcommon.RBACError). It returns ok=false
// when err isn't an RBAC denial, so callers don't have to guess a cause.
func rbacFixHint(err error) (fix string, ok bool) {
	var rbacErr *admcommon.RBACError
	if !errors.As(err, &rbacErr) {
		return "", false
	}
	account := "the cluster agent's Kubernetes service account"
	if rbacErr.Username != "" {
		account = fmt.Sprintf("service account %q", rbacErr.Username)
	}
	if rbacErr.Namespace != "" {
		return fmt.Sprintf("Grant %s %q permission on %s %q in namespace %q", account, rbacErr.Verb, rbacErr.Resource, rbacErr.Name, rbacErr.Namespace), true
	}
	return fmt.Sprintf("Grant %s %q permission on the cluster-scoped %s %q", account, rbacErr.Verb, rbacErr.Resource, rbacErr.Name), true
}

// k8sErrorFixHint classifies common Kubernetes API error conditions that
// aren't RBAC denials, so the remediation can point at the specific
// condition the API server reported instead of telling the user to go find
// the error that's already in the issue description.
func k8sErrorFixHint(err error) (fix string, ok bool) {
	switch {
	case k8serrors.IsTimeout(err) || k8serrors.IsServerTimeout(err):
		return "The Kubernetes API server timed out handling the request; check network connectivity and API server load between the cluster agent and the control plane", true
	case k8serrors.IsTooManyRequests(err):
		return "The Kubernetes API server is rate-limiting the cluster agent's requests (429 Too Many Requests); check API server load", true
	case k8serrors.IsServiceUnavailable(err):
		return "The Kubernetes API server was unavailable (503); check API server health and cluster agent connectivity to the control plane", true
	case k8serrors.IsConflict(err):
		return "The request conflicted with a concurrent update to the same object; the controller retries automatically and this typically resolves on its own", true
	case k8serrors.IsAlreadyExists(err):
		return "The object already exists with content the controller doesn't manage; check for another process creating an object with the same name", true
	case k8serrors.IsInvalid(err):
		return "The Kubernetes API server rejected the object as invalid; see the validation error above for the specific field", true
	case k8serrors.IsNotFound(err):
		return "A dependent Kubernetes object was not found; verify the cluster agent's configured namespace and object names", true
	case k8serrors.IsInternalError(err):
		return "The Kubernetes API server returned an internal error (500); check the API server's own logs for the root cause", true
	}
	return "", false
}

// webhookExists reports whether the enabled webhook configuration objects
// exist in the cluster. False+nil means confirmed missing; non-nil error
// means existence could not be determined.
func (p *Probe) webhookExists(ctx context.Context) (bool, error) {
	if p.mutationEnabled {
		_, err := admcommon.GetMutatingWebhookStatus(ctx, p.webhookName, p.k8sClient)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			err = admcommon.WrapIfForbidden(err, "get", "mutatingwebhookconfigurations", "", p.webhookName)
			return false, fmt.Errorf("checking mutating webhook configuration: %w", err)
		}
	}

	if p.validationEnabled {
		_, err := admcommon.GetValidatingWebhookStatus(ctx, p.webhookName, p.k8sClient)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			err = admcommon.WrapIfForbidden(err, "get", "validatingwebhookconfigurations", "", p.webhookName)
			return false, fmt.Errorf("checking validating webhook configuration: %w", err)
		}
	}

	return true, nil
}

func (p *Probe) reportHealthIssue(issueDescription, cause, fixHint string) {
	context := map[string]string{
		"remediation": p.diagnosticHint,
	}
	if issueDescription != "" {
		context["issue"] = issueDescription
	}
	if cause != "" {
		context["cause"] = cause
	}
	if fixHint != "" {
		context["fix_hint"] = fixHint
	}
	issue, buildErr := (&admissionprobe.AdmissionProbeIssue{}).BuildIssue(context)
	if buildErr != nil {
		issue = &healthplatformpayload.Issue{
			Id:        healthIssueID,
			IssueName: admissionprobe.IssueName,
			IssueType: admissionprobe.IssueType,
			Title:     "Admission Controller Unreachable",
			Source:    "cluster-agent",
		}
	} else {
		issue.Id = healthIssueID
	}

	if reportErr := p.healthPlatform.ReportIssue(issue); reportErr != nil {
		log.Warnf("Failed to report admission probe health issue: %v", reportErr)
	}
}

func (p *Probe) clearHealthIssue() {
	p.healthPlatform.ResolveIssue(healthIssueID)
}

func (p *Probe) execute(ctx context.Context) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "datadog-admission-probe-",
			Namespace:    p.namespace,
			Labels: map[string]string{
				admcommon.ProbeLabelKey: "true",
			},
		},
	}

	result, err := p.k8sClient.CoreV1().ConfigMaps(p.namespace).Create(
		ctx, cm, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}},
	)
	if err != nil {
		return err
	}

	if _, found := result.Annotations[admcommon.ProbeReceivedAnnotationKey]; !found {
		return errProbeNotReceived
	}
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.UTC().Format(time.RFC3339)
}

func diagnosticHintForProvider(provider string) string {
	switch provider {
	case "eks":
		return "EKS detected: ensure your node security groups allow inbound TCP on port 8000 from the cluster security group."
	case "gke":
		return "GKE detected: if using a private cluster, ensure your firewall rules allow ingress over TCP on port 8000 from the control plane CIDR."
	case "aks":
		return "AKS detected: ensure providers.aks.enabled is set to true in your Helm/Operator configuration."
	default:
		return "Ensure proper inbound network connectivity to the cluster agent's node on port 8000."
	}
}
