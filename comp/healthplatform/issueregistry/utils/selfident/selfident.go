// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package selfident resolves the agent's own Kubernetes DaemonSet identity,
// so issues caused by a cluster-distributed template (a bad cluster check,
// a cluster-distributed config file) share one discriminator across every
// node agent, letting the backend collapse them into a single issue.
package selfident

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	podNameEnvVar      = "DD_POD_NAME"
	daemonSetOwnerKind = "DaemonSet"

	// defaultResolveRetries/defaultResolveRetryDelay bound how long DeploymentID
	// waits for workloadmeta to observe the agent's own pod before giving up,
	// and how long ClusterID retries the Cluster Agent in the background. Kept
	// short (~1s) since DeploymentID can block a synchronous ReportIssue caller.
	defaultResolveRetries    = 5
	defaultResolveRetryDelay = 200 * time.Millisecond
)

// SelfIdent resolves and caches the agent's own DaemonSet UID (deployment_id)
// and cluster id, for use as health-issue identity discriminators.
type SelfIdent struct {
	wmeta workloadmeta.Component

	resolveMu    sync.Mutex
	deploymentID atomic.Pointer[string]

	resolveRetries    int
	resolveRetryDelay time.Duration

	clusterIDResolveOnce sync.Once
	clusterID            atomic.Pointer[string]
}

// New creates a SelfIdent. Outside Kubernetes it returns a no-op instance that
// never touches workloadmeta or the Cluster Agent, since deployment_id/cluster
// id only make sense there. wmeta is nil only in tests that don't care about
// deployment_id resolution, in which case DeploymentID resolves to empty.
func New(wmeta workloadmeta.Component) *SelfIdent {
	s := &SelfIdent{
		wmeta:             wmeta,
		resolveRetries:    defaultResolveRetries,
		resolveRetryDelay: defaultResolveRetryDelay,
	}
	if !env.IsFeaturePresent(env.Kubernetes) {
		empty := ""
		s.deploymentID.Store(&empty)
		s.clusterIDResolveOnce.Do(func() {})
		s.clusterID.Store(&empty)
	}
	return s
}

// DeploymentID returns the UID of the DaemonSet that owns this agent's pod,
// or "" if not running under one. Resolution is retried a bounded number of
// times before returning; a definitive outcome (found, or the pod/wmeta
// isn't resolvable at all) is cached for the process lifetime, but a
// transient "pod not synced yet" outcome is not cached, so a later call —
// e.g. from another issue module reporting after this one — gets a fresh
// attempt instead of being stuck replaying a stale "".
func (s *SelfIdent) DeploymentID() string {
	if cached := s.deploymentID.Load(); cached != nil {
		return *cached
	}

	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if cached := s.deploymentID.Load(); cached != nil {
		return *cached
	}

	podNamespace := namespace.GetMyNamespace()
	var id string
	var definitive bool
	for attempt := 0; ; attempt++ {
		id, definitive = s.resolveDeploymentID(podNamespace)
		if definitive || attempt >= s.resolveRetries {
			break
		}
		time.Sleep(s.resolveRetryDelay)
	}
	if definitive {
		s.deploymentID.Store(&id)
	}
	return id
}

// IssueDiscriminator returns DeploymentID() when non-empty, so all agents in
// the same DaemonSet emit identical issue ids for the same template-induced
// problem. Otherwise it falls back to hostID, or the OS hostname if hostID
// is empty, preserving today's per-host behavior for non-Kubernetes agents.
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

// ClusterID returns the best-effort Kubernetes cluster id for payload
// enrichment only — never part of the issue id. Resolution runs in the
// background since clustername.GetClusterID() usually makes a synchronous
// Cluster Agent HTTP call, but a caller made before resolution finishes
// blocks up to resolveRetries*resolveRetryDelay for it — a startup-only
// health check (e.g. invalidconfig) calls this exactly once and never
// re-reports, so returning immediately would permanently miss the id.
// Callers made after resolution settles return immediately from cache.
func (s *SelfIdent) ClusterID() string {
	s.clusterIDResolveOnce.Do(func() {
		go s.resolveClusterID()
	})
	for attempt := 0; ; attempt++ {
		if id := s.clusterID.Load(); id != nil {
			return *id
		}
		if attempt >= s.resolveRetries {
			return ""
		}
		time.Sleep(s.resolveRetryDelay)
	}
}

// resolveClusterID retries clustername.GetClusterID() a bounded number of
// times (clustername caches a successful result process-wide, so retries
// here only matter while the Cluster Agent hasn't answered yet) before
// giving up and caching empty for the process lifetime.
func (s *SelfIdent) resolveClusterID() {
	for attempt := 0; ; attempt++ {
		id, err := clustername.GetClusterID()
		if err == nil {
			s.clusterID.Store(&id)
			return
		}
		if attempt >= s.resolveRetries {
			log.Debugf("selfident: cluster id unavailable after %d attempts: %v", attempt+1, err)
			empty := ""
			s.clusterID.Store(&empty)
			return
		}
		time.Sleep(s.resolveRetryDelay)
	}
}

// resolveDeploymentID makes one resolution attempt. definitive is true when
// the caller can cache the result permanently (no workloadmeta, no resolvable
// pod name, or the pod was found); false means the pod isn't in workloadmeta
// yet, so the caller should retry rather than cache a false negative.
func (s *SelfIdent) resolveDeploymentID(podNamespace string) (id string, definitive bool) {
	if s.wmeta == nil {
		return "", true
	}
	podName, ok := selfPodName()
	if !ok {
		return "", true
	}
	pod, err := s.wmeta.GetKubernetesPodByName(podName, podNamespace)
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

// selfPodName returns this container's own pod name: DD_POD_NAME when set
// (Helm chart via the downward API), else the container hostname, which
// kubelet defaults to the pod's name. The hostname fallback matters because
// the Datadog Operator only injects DD_POD_NAME into the cluster agent.
func selfPodName() (string, bool) {
	if podName, ok := os.LookupEnv(podNameEnvVar); ok {
		return podName, true
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", false
	}
	return hostname, true
}
