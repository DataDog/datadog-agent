// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package selfident

import (
	"context"
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

	// Keep the default wait short for the synchronous ReportIssue path.
	defaultResolveTimeout    = time.Second
	defaultResolveRetryDelay = 200 * time.Millisecond
)

// SelfIdent resolves and caches the agent's own DaemonSet UID (deployment_id)
// and cluster id, for use as health-issue identity discriminators.
type SelfIdent struct {
	wmeta workloadmeta.Component

	resolveMu    sync.Mutex
	deploymentID atomic.Pointer[string]

	resolveTimeout    time.Duration
	resolveRetryDelay time.Duration

	// Concurrent callers share one in-flight cluster lookup.
	clusterResolveMu sync.Mutex
	clusterResolving chan struct{}
	clusterID        atomic.Pointer[string]
}

// New creates a SelfIdent. Outside Kubernetes it returns a no-op instance that
// never touches workloadmeta or the Cluster Agent, since deployment_id/cluster
// id only make sense there. wmeta is nil only in tests that don't care about
// deployment_id resolution, in which case DeploymentID resolves to empty.
func New(wmeta workloadmeta.Component) *SelfIdent {
	s := &SelfIdent{
		wmeta:             wmeta,
		resolveTimeout:    defaultResolveTimeout,
		resolveRetryDelay: defaultResolveRetryDelay,
	}
	if !env.IsFeaturePresent(env.Kubernetes) {
		empty := ""
		s.deploymentID.Store(&empty)
		s.clusterID.Store(&empty)
	}
	return s
}

// DeploymentID resolves the owning DaemonSet UID using the default wait.
func (s *SelfIdent) DeploymentID() string {
	if cached := s.deploymentID.Load(); cached != nil {
		return *cached
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.resolveTimeout)
	defer cancel()
	return s.DeploymentIDWithContext(ctx)
}

// DeploymentIDWithContext waits for the owning DaemonSet UID until ctx expires.
func (s *SelfIdent) DeploymentIDWithContext(ctx context.Context) string {
	podNamespace := namespace.GetMyNamespace()
	for ctx.Err() == nil {
		s.resolveMu.Lock()
		if cached := s.deploymentID.Load(); cached != nil {
			s.resolveMu.Unlock()
			return *cached
		}
		id, definitive := s.resolveDeploymentID(podNamespace)
		// A transient miss must remain retryable by later callers.
		if definitive {
			s.deploymentID.Store(&id)
		}
		s.resolveMu.Unlock()
		if definitive {
			return id
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(s.resolveRetryDelay):
		}
	}
	return ""
}

// IssueDiscriminator returns DeploymentID(), so all agents in the same
// DaemonSet emit identical issue ids for the same template-induced problem.
// It is empty when this agent is not owned by a DaemonSet; callers apply the
// per-host fallback on that empty result — see issues.IssueDiscriminator.
func (s *SelfIdent) IssueDiscriminator() string {
	return s.DeploymentID()
}

// ClusterID resolves the Kubernetes cluster ID using the default wait.
func (s *SelfIdent) ClusterID() string {
	if cached := s.clusterID.Load(); cached != nil {
		return *cached
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.resolveTimeout)
	defer cancel()
	return s.ClusterIDWithContext(ctx)
}

// ClusterIDWithContext waits for the Kubernetes cluster ID until ctx expires.
func (s *SelfIdent) ClusterIDWithContext(ctx context.Context) string {
	for ctx.Err() == nil {
		if cached := s.clusterID.Load(); cached != nil {
			return *cached
		}
		select {
		case <-s.startClusterResolve():
		case <-ctx.Done():
			return ""
		}
		if cached := s.clusterID.Load(); cached != nil {
			return *cached
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(s.resolveRetryDelay):
		}
	}
	return ""
}

// The lookup has no context, so callers share a goroutine and bound their own wait.
func (s *SelfIdent) startClusterResolve() <-chan struct{} {
	s.clusterResolveMu.Lock()
	defer s.clusterResolveMu.Unlock()
	if s.clusterResolving != nil {
		return s.clusterResolving
	}
	done := make(chan struct{})
	// Re-check under the lock: another resolver may have populated the cache
	// since the caller's Load. Return an already-closed channel so the caller
	// reads it immediately instead of waiting out the timeout behind a
	// redundant lookup.
	if s.clusterID.Load() != nil {
		close(done)
		return done
	}
	s.clusterResolving = done
	go func() {
		// Clear clusterResolving before closing done so a caller woken by the
		// close sees no in-flight resolver and can start a fresh attempt after a
		// failure.
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

func (s *SelfIdent) resolveClusterID() {
	// clustername.GetClusterID() is node-agent-only: on the Cluster Agent it
	// targets an endpoint meant for node→DCA calls, broken when the DCA calls
	// itself, so the Cluster Agent resolves its own id like
	// comp/metadata/clusteragent does.
	lookup := nodeAgentClusterIDFunc
	if flavor.GetFlavor() == flavor.ClusterAgent {
		lookup = clusterAgentClusterIDFunc
	}

	id, err := lookup()
	if err != nil {
		log.Debugf("selfident: cluster id unavailable: %v", err)
		return
	}
	s.clusterID.Store(&id)
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
