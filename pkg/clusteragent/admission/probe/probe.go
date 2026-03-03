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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultInterval    = 60 * time.Second
	defaultGracePeriod = 60 * time.Second
	probeNamespace     = "default"
)

// Probe periodically verifies that the admission webhook is reachable by
// creating dry-run pods and checking if they are handled by the webhook.
type Probe struct {
	k8sClient      kubernetes.Interface
	isLeaderFunc   func() bool
	interval       time.Duration
	gracePeriod    time.Duration
	logLimiter     *log.Limit
	diagnosticHint string
}

// New creates a new admission controller connectivity probe.
func New(k8sClient kubernetes.Interface, isLeaderFunc func() bool) *Probe {
	return &Probe{
		k8sClient:    k8sClient,
		isLeaderFunc: isLeaderFunc,
		interval:     defaultInterval,
		gracePeriod:  defaultGracePeriod,
		logLimiter:   log.NewLogLimit(1, 10*time.Minute),
	}
}

// Run starts the periodic probe loop. It blocks until ctx is cancelled.
func (p *Probe) Run(ctx context.Context) {
	log.Info("Starting admission controller probe")

	// Wait for the webhook to be registered with the API server.
	select {
	case <-ctx.Done():
		return
	case <-time.After(p.gracePeriod):
	}

	p.diagnosticHint = diagnosticHintForProvider(cloudprovider.DCAGetName(ctx))
	log.Info("Admission controller probe is now active")

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
			if err := p.execute(ctx); err != nil {
				if p.logLimiter.ShouldLog() {
					log.Errorf(
						"Admission controller probe failed: the webhook did not handle the probe pod. "+
							"This indicates a network connectivity issue between the Kubernetes API server "+
							"and the cluster agent admission webhook. %s Error: %v",
						p.diagnosticHint, err,
					)
				}
			}
		}
	}
}

func (p *Probe) execute(ctx context.Context) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "datadog-admission-probe-",
			Namespace:    probeNamespace,
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

	result, err := p.k8sClient.CoreV1().Pods(probeNamespace).Create(
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

var errProbeNotReceived = errors.New("dry-run probe pod was not annotated by the webhook")

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
