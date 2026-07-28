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
	"sync/atomic"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
//
// ClusterID reuses the same bound for its background resolution retries —
// it isn't latency-sensitive since it never blocks a caller, but there is
// no reason to keep retrying indefinitely once the Cluster Agent has failed
// to answer a few times in a row.
const (
	defaultResolveRetries    = 5
	defaultResolveRetryDelay = 200 * time.Millisecond
)

// SelfIdent resolves and caches the agent's own DaemonSet UID (deployment_id)
// and cluster id, for use as health-issue identity discriminators.
type SelfIdent struct {
	wmeta workloadmeta.Component

	once         sync.Once
	deploymentID string

	resolveRetries    int
	resolveRetryDelay time.Duration

	clusterIDResolveOnce sync.Once
	clusterID            atomic.Pointer[string]
}

// New creates a SelfIdent. Every binary that wires the health-platform bundle
// also wires workloadmeta's fx module, so wmeta is always a real component in
// production; nil is only ever passed directly by tests that don't care about
// deployment_id resolution, in which case DeploymentID always resolves to
// empty and IssueDiscriminator falls back to the given host id.
func New(wmeta workloadmeta.Component) *SelfIdent {
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
// enrichment only — never part of the issue id itself. Empty if unavailable
// or not resolved yet.
//
// clustername.GetClusterID() reads DD_ORCHESTRATOR_CLUSTER_ID when the
// Helm chart/Operator wires it, but in practice that env var is not set by
// current deployments, so every call falls through to a synchronous HTTP
// request to the Cluster Agent. Blocking on that request from ReportIssue
// would tie issue reporting to Cluster Agent availability for metadata that
// is best-effort by design, so resolution runs in a background goroutine
// instead: the first call kicks it off and returns "" immediately, later
// calls return whatever has been resolved so far.
func (s *SelfIdent) ClusterID() string {
	s.clusterIDResolveOnce.Do(func() {
		go s.resolveClusterID()
	})
	if id := s.clusterID.Load(); id != nil {
		return *id
	}
	return ""
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

// resolveDeploymentID makes one resolution attempt. The second return value
// reports whether the result is definitive: true means the caller can cache
// it permanently (no workloadmeta, no resolvable pod name, or the pod was
// found and its owners inspected); false means the agent's own pod wasn't
// found in workloadmeta yet, which may just mean the initial sync hasn't
// happened — the caller should retry rather than cache a false negative.
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
// (the Helm chart injects it via the downward API), else the container
// hostname, which kubelet defaults to the pod's name unless the pod uses
// hostNetwork or a custom hostname/subdomain. The hostname fallback is
// needed because, unlike the Helm chart, the Datadog Operator does not
// inject DD_POD_NAME into the node agent (only into the cluster agent).
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
