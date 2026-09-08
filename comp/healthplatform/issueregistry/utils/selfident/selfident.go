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
	// and how long the ClusterID resolver retries the Cluster Agent/API server.
	// Kept short (~1s) since both feed the synchronous ReportIssue path.
	defaultResolveRetries    = 5
	defaultResolveRetryDelay = 200 * time.Millisecond

	// defaultClusterResolveTimeout bounds how long a ClusterID caller blocks
	// waiting for the resolver, independent of how long an individual lookup
	// blocks (the node-agent HTTP client can take up to ~10s, and the Cluster
	// Agent's Kubernetes calls take no caller deadline). Without this, a
	// Cluster Agent/API server outage could stall ReportIssue — and, through
	// it, agent shutdown, which waits on the reporting path — for many seconds.
	defaultClusterResolveTimeout = defaultResolveRetries * defaultResolveRetryDelay
)

// SelfIdent resolves and caches the agent's own DaemonSet UID (deployment_id)
// and cluster id, for use as health-issue identity discriminators.
type SelfIdent struct {
	wmeta workloadmeta.Component

	resolveMu    sync.Mutex
	deploymentID atomic.Pointer[string]

	resolveRetries    int
	resolveRetryDelay time.Duration

	// clusterResolveTimeout bounds how long a ClusterID caller blocks waiting
	// for the shared resolver; clusterResolveMu guards clusterResolving, the
	// channel closed when the single in-flight resolution finishes (nil when
	// none is running).
	clusterResolveTimeout time.Duration
	clusterResolveMu      sync.Mutex
	clusterResolving      chan struct{}
	clusterID             atomic.Pointer[string]
}

// New creates a SelfIdent. Outside Kubernetes it returns a no-op instance that
// never touches workloadmeta or the Cluster Agent, since deployment_id/cluster
// id only make sense there. wmeta is nil only in tests that don't care about
// deployment_id resolution, in which case DeploymentID resolves to empty.
func New(wmeta workloadmeta.Component) *SelfIdent {
	s := &SelfIdent{
		wmeta:                 wmeta,
		resolveRetries:        defaultResolveRetries,
		resolveRetryDelay:     defaultResolveRetryDelay,
		clusterResolveTimeout: defaultClusterResolveTimeout,
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
// clusterResolveTimeout while resolution is in flight — long enough to give a
// one-shot startup check (e.g. invalidconfig, which calls this exactly once
// and never re-reports) a real chance at getting the id, but bounded so it
// can't block forever even when an individual lookup hangs (the node-agent
// HTTP client can take ~10s, and the Cluster Agent's Kubernetes calls take no
// caller deadline — so bounding only the retry sleeps would not bound the
// caller). A successful result is cached for the process lifetime; a failed
// resolution is deliberately NOT cached, so a later call (e.g. the next
// periodic report) gets a fresh attempt instead of being stuck with an empty
// id forever just because the Cluster Agent/API server was still starting up
// the first time this was called — the same guarantee DeploymentID already
// gives a transient workloadmeta miss.
//
// Because lookup() cannot be cancelled (it takes no context), the retry loop
// runs in a single shared resolver goroutine and callers wait on it with a
// deadline: a caller that hits the deadline returns "" without waiting for a
// slow lookup to finish, and concurrent callers share that one resolver's
// retry budget instead of each repeating it. At most one resolver runs at a
// time and it self-terminates after exhausting the retry budget, so a lookup
// that outlives the caller is bounded rather than an unbounded leak.
func (s *SelfIdent) ClusterID() string {
	if cached := s.clusterID.Load(); cached != nil {
		return *cached
	}

	resolved := s.startClusterResolve()
	select {
	case <-resolved:
	case <-time.After(s.clusterResolveTimeout):
	}
	if cached := s.clusterID.Load(); cached != nil {
		return *cached
	}
	return ""
}

// startClusterResolve returns a channel closed when the current cluster id
// resolution finishes, starting a single shared resolver goroutine if none is
// already in flight so concurrent callers share one retry budget.
func (s *SelfIdent) startClusterResolve() <-chan struct{} {
	s.clusterResolveMu.Lock()
	defer s.clusterResolveMu.Unlock()
	if s.clusterResolving != nil {
		return s.clusterResolving
	}
	done := make(chan struct{})
	s.clusterResolving = done
	go func() {
		// Clear clusterResolving before closing done so that a caller woken by
		// the close (and any call it makes next) observes no in-flight
		// resolver and can start a fresh attempt after a failure.
		defer close(done)
		defer func() {
			s.clusterResolveMu.Lock()
			s.clusterResolving = nil
			s.clusterResolveMu.Unlock()
		}()
		s.resolveClusterID()
	}()
	return done
}

// resolveClusterID retries the flavor-appropriate cluster id lookup a bounded
// number of times, caching a successful result for the process lifetime and
// leaving the cache untouched on failure so a later caller retries.
func (s *SelfIdent) resolveClusterID() {
	// clustername.GetClusterID() is meant for the node agent to call — on
	// the Cluster Agent itself it targets an HTTP endpoint designed for the
	// node agent to reach the Cluster Agent, which is broken when the
	// Cluster Agent tries to reach itself — so the Cluster Agent resolves
	// its own cluster id the same way comp/metadata/clusteragent does.
	lookup := nodeAgentClusterIDFunc
	if flavor.GetFlavor() == flavor.ClusterAgent {
		lookup = clusterAgentClusterIDFunc
	}

	for attempt := 0; ; attempt++ {
		id, err := lookup()
		if err == nil {
			s.clusterID.Store(&id)
			return
		}
		if attempt >= s.resolveRetries {
			log.Debugf("selfident: cluster id unavailable after %d attempts: %v", attempt+1, err)
			return
		}
		time.Sleep(s.resolveRetryDelay)
	}
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
