// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package selfident resolves the identity of the agent's own Kubernetes
// DaemonSet, so that health issues caused by a cluster-distributed template
// (a bad cluster check, a cluster-distributed config file) can be reported
// with a shared discriminator across every node agent it was applied to,
// letting the backend collapse them into a single issue instead of one per
// host.
package selfident

import (
	"os"
	"sync"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const podNameEnvVar = "DD_POD_NAME"

const daemonSetOwnerKind = "DaemonSet"

// defaultResolveRetries/defaultResolveRetryDelay bound how long DeploymentID
// waits for workloadmeta's kubelet collector to observe the agent's own pod
// at startup, before giving up. Resolution happens once (sync.Once) for the
// whole process, so whichever caller is first to reach DeploymentID after
// startup pays this bounded wait — typically a built-in startup health check
// (invalidconfig, invalidsysprobeconfig) firing from its own goroutine as soon
// as the health-platform bundle starts, but it can also be a synchronous
// ReportIssue caller (a Python check's report_issue(), the GPU check's Run())
// if one reports before the startup checks do. Without this retry, losing the
// race against workloadmeta's initial sync would permanently cache an empty
// deployment_id for the life of the process; the bound is kept short (~1s)
// specifically so it stays acceptable on those latency-sensitive paths too.
const (
	defaultResolveRetries    = 5
	defaultResolveRetryDelay = 200 * time.Millisecond
)

// SelfIdent resolves and caches the agent's own DaemonSet UID (deployment_id)
// and cluster id, for use as health-issue identity discriminators.
type SelfIdent struct {
	wmeta option.Option[workloadmeta.Component]

	once         sync.Once
	deploymentID string

	resolveRetries    int
	resolveRetryDelay time.Duration

	clusterIDOnce sync.Once
	clusterID     string
}

// New creates a SelfIdent. wmeta may be unset (e.g. builds without
// workloadmeta), in which case DeploymentID always resolves to empty and
// IssueDiscriminator falls back to the given host id.
func New(wmeta option.Option[workloadmeta.Component]) *SelfIdent {
	return &SelfIdent{
		wmeta:             wmeta,
		resolveRetries:    defaultResolveRetries,
		resolveRetryDelay: defaultResolveRetryDelay,
	}
}

// DeploymentID returns the UID of the DaemonSet that owns this agent's pod,
// or "" if the agent is definitively not running under a DaemonSet (not on
// Kubernetes, or no DaemonSet owner reference). Resolved once and cached for
// the process lifetime, since pod ownership cannot change without a pod
// restart. If the agent's own pod simply hasn't appeared in workloadmeta yet,
// resolution is retried a bounded number of times before caching empty.
func (s *SelfIdent) DeploymentID() string {
	s.once.Do(func() {
		podNamespace := namespace.GetMyNamespace()
		for attempt := 0; ; attempt++ {
			id, found := s.resolveDeploymentID(podNamespace)
			if found || attempt >= s.resolveRetries {
				s.deploymentID = id
				return
			}
			time.Sleep(s.resolveRetryDelay)
		}
	})
	return s.deploymentID
}

// IssueDiscriminator returns DeploymentID() when non-empty, so all agents in
// the same DaemonSet emit identical issue ids for the same template-induced
// problem. Otherwise it falls back to hostID, preserving today's per-host
// behavior for non-Kubernetes agents. If hostID is empty (caller has no
// hostname component handy), it falls back further to the OS hostname so
// per-host uniqueness is still preserved.
func (s *SelfIdent) IssueDiscriminator(hostID string) string {
	if deploymentID := s.DeploymentID(); deploymentID != "" {
		return deploymentID
	}
	if hostID != "" {
		return hostID
	}
	if osHostname, err := os.Hostname(); err == nil {
		return osHostname
	}
	return ""
}

// ClusterID returns the best-effort Kubernetes cluster id, for payload
// enrichment only — never part of the issue id itself. Empty if unavailable.
func (s *SelfIdent) ClusterID() string {
	s.clusterIDOnce.Do(func() {
		id, err := clustername.GetClusterID()
		if err != nil {
			log.Debugf("selfident: cluster id unavailable: %v", err)
			return
		}
		s.clusterID = id
	})
	return s.clusterID
}

// resolveDeploymentID makes one resolution attempt. The second return value
// reports whether the result is definitive: true means the caller can cache
// it permanently (no workloadmeta, no pod name env var, or the pod was found
// and its owners inspected); false means the agent's own pod wasn't found in
// workloadmeta yet, which may just mean the initial sync hasn't happened —
// the caller should retry rather than cache a false negative.
func (s *SelfIdent) resolveDeploymentID(podNamespace string) (id string, definitive bool) {
	wmeta, ok := s.wmeta.Get()
	if !ok {
		return "", true
	}
	podName := os.Getenv(podNameEnvVar)
	if podName == "" {
		return "", true
	}
	pod, err := wmeta.GetKubernetesPodByName(podName, podNamespace)
	if err != nil {
		log.Debugf("selfident: own pod %q not yet in workloadmeta: %v", podName, err)
		return "", false
	}
	for _, owner := range pod.Owners {
		if owner.Kind == daemonSetOwnerKind {
			return owner.ID, true
		}
	}
	return "", true
}
