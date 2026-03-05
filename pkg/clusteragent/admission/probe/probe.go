// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package probe periodically tests admission webhook connectivity by sending
// dry-run pod creation requests through the Kubernetes API server.
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

	"github.com/DataDog/datadog-agent/comp/core/config"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var errProbeNotReceived = errors.New("dry-run probe pod was not annotated by the webhook")

// Probe periodically verifies that the admission webhook is reachable by
// creating dry-run pods and checking if they are handled by the webhook.
type Probe struct {
	k8sClient      kubernetes.Interface
	isLeaderFunc   func() bool
	namespace      string
	interval       time.Duration
	gracePeriod    time.Duration
	logLimiter     *log.Limit
	diagnosticHint string

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

// New creates a new admission controller connectivity probe.
func New(k8sClient kubernetes.Interface, isLeaderFunc func() bool, datadogConfig config.Component) *Probe {
	return &Probe{
		k8sClient:    k8sClient,
		isLeaderFunc: isLeaderFunc,
		namespace:    datadogConfig.GetString("admission_controller.probe.namespace"),
		interval:     time.Duration(datadogConfig.GetInt("admission_controller.probe.interval")) * time.Second,
		gracePeriod:  time.Duration(datadogConfig.GetInt("admission_controller.probe.grace_period")) * time.Second,
		logLimiter:   log.NewLogLimit(1, 10*time.Minute),
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

// GetStatsForStatus returns probe stats formatted for the agent status output.
func (p *Probe) GetStatsForStatus() map[string]interface{} {
	snap := p.GetStatsSnapshot()
	result := map[string]interface{}{
		"Namespace": p.namespace,
	}

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

	p.diagnosticHint = diagnosticHintForProvider(cloudprovider.DCAGetName(ctx))
	log.Infof("Admission controller probe is now active (namespace=%s, interval=%s)", p.namespace, p.interval)

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
	if err == nil {
		p.stats.SuccessCount++
		p.stats.LastExecutionSuccess = true
		p.stats.LastExecutionError = ""
		p.stats.LastSuccessTime = now
		p.stats.ConfigError = ""
	} else {
		p.stats.FailCount++
		p.stats.LastExecutionSuccess = false
		p.stats.LastExecutionError = err.Error()
	}
	p.stats.mu.Unlock()

	if err == nil {
		return
	}

	p.handleError(err)
}

func (p *Probe) handleError(err error) {
	if k8serrors.IsNotFound(err) {
		msg := fmt.Sprintf("Probe namespace %q does not exist. Create it to enable admission controller connectivity probing.", p.namespace)
		p.stats.mu.Lock()
		p.stats.ConfigError = msg
		p.stats.mu.Unlock()
		if p.logLimiter.ShouldLog() {
			log.Errorf("Admission controller probe misconfigured: %s", msg)
		}
		return
	}

	if k8serrors.IsForbidden(err) {
		msg := fmt.Sprintf("The cluster agent service account does not have permission to create pods in namespace %q. Grant pod creation RBAC to enable connectivity probing.", p.namespace)
		p.stats.mu.Lock()
		p.stats.ConfigError = msg
		p.stats.mu.Unlock()
		if p.logLimiter.ShouldLog() {
			log.Errorf("Admission controller probe misconfigured: %s", msg)
		}
		return
	}

	if errors.Is(err, errProbeNotReceived) {
		if p.logLimiter.ShouldLog() {
			log.Errorf(
				"Admission controller probe failed: the webhook did not handle the probe pod. "+
					"This indicates a network connectivity issue between the Kubernetes API server "+
					"and the cluster agent admission webhook. %s",
				p.diagnosticHint,
			)
		}
		return
	}

	if p.logLimiter.ShouldLog() {
		log.Errorf("Admission controller probe failed: %v", err)
	}
}

func (p *Probe) execute(ctx context.Context) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "datadog-admission-probe-",
			Namespace:    p.namespace,
			Labels: map[string]string{
				admcommon.EnabledLabelKey: "true",
				admcommon.ProbeLabelKey:   "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "probe",
				Image: "registry.k8s.io/pause:3.9",
			}},
		},
	}

	result, err := p.k8sClient.CoreV1().Pods(p.namespace).Create(
		ctx, pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}},
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
