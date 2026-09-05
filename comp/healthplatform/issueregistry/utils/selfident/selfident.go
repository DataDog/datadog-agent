// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package selfident

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apiservercommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	podNameEnvVar      = "DD_POD_NAME"
	daemonSetOwnerKind = "DaemonSet"

	// defaultResolveRetries/defaultResolveRetryDelay bound how long DeploymentID
	// waits for workloadmeta to observe the agent's own pod before giving up,
	// and how long ClusterID retries the Cluster Agent/API server. Kept short
	// (~1s) since both are called from the synchronous ReportIssue path.
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

	clusterResolveMu sync.Mutex
	clusterID        atomic.Pointer[string]
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

// IssueDiscriminator returns DeploymentID(), so all agents in the same
// DaemonSet emit identical issue ids for the same template-induced problem.
// It is empty when this agent is not owned by a DaemonSet; callers apply the
// per-host fallback on that empty result — see issues.IssueDiscriminator.
func (s *SelfIdent) IssueDiscriminator() string {
	return s.DeploymentID()
}

// ClusterID returns the best-effort Kubernetes cluster id for payload
// enrichment only — never part of the issue id. A caller blocks up to
// resolveRetries*resolveRetryDelay while resolution is in flight — long
// enough to give a one-shot startup check (e.g. invalidconfig, which calls
// this exactly once and never re-reports) a real chance at getting the id,
// but bounded so it can't block forever. A successful result is cached for
// the process lifetime; a failed resolution is deliberately NOT cached, so
// a later call (e.g. the next periodic report) gets a fresh attempt instead
// of being stuck with an empty id forever just because the Cluster
// Agent/API server was still starting up the first time this was called —
// the same guarantee DeploymentID already gives a transient workloadmeta miss.
//
// Concurrent callers serialize on clusterResolveMu rather than resolving in
// parallel: this only matters while the Cluster Agent/API server is down,
// in which case every caller is going to wait out the same bounded retry
// budget anyway, and serializing keeps the resolution logic a single,
// easy-to-reason-about synchronous path (mirroring DeploymentID) instead of
// a background goroutine, which — as a prior version of this method did —
// can outlive the call that spawned it and is easy to get wrong.
func (s *SelfIdent) ClusterID() string {
	if cached := s.clusterID.Load(); cached != nil {
		return *cached
	}

	s.clusterResolveMu.Lock()
	defer s.clusterResolveMu.Unlock()
	if cached := s.clusterID.Load(); cached != nil {
		return *cached
	}

	// clustername.GetClusterID() is meant for the node agent to call — on
	// the Cluster Agent itself it targets an HTTP endpoint designed for the
	// node agent to reach the Cluster Agent, which is broken when the
	// Cluster Agent tries to reach itself — so the Cluster Agent resolves
	// its own cluster id the same way comp/metadata/clusteragent does.
	lookup := nodeAgentClusterIDFunc
	if flavor.GetFlavor() == flavor.ClusterAgent {
		lookup = clusterAgentClusterIDFunc
	}

	var id string
	var err error
	for attempt := 0; ; attempt++ {
		id, err = lookup()
		if err == nil {
			break
		}
		if attempt >= s.resolveRetries {
			log.Debugf("selfident: cluster id unavailable after %d attempts: %v", attempt+1, err)
			return ""
		}
		time.Sleep(s.resolveRetryDelay)
	}
	s.clusterID.Store(&id)
	return id
}

// nodeAgentClusterIDFunc/clusterAgentClusterIDFunc are the per-flavor cluster
// id lookups used by ClusterID, overridable in tests so dispatch can be
// verified without a real Cluster Agent or Kubernetes API server.
var (
	nodeAgentClusterIDFunc    = clustername.GetClusterID
	clusterAgentClusterIDFunc = clusterAgentOwnClusterID
)

// clusterAgentOwnClusterID resolves the cluster id from the Cluster Agent's
// own Kubernetes API client, mirroring
// comp/metadata/clusteragent/impl/cluster_agent.go's getClusterID.
func clusterAgentOwnClusterID() (string, error) {
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return "", err
	}
	return apiservercommon.GetOrCreateClusterID(cl.Cl.CoreV1())
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
