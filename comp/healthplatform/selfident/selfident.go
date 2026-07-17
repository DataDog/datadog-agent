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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const podNameEnvVar = "DD_POD_NAME"

const daemonSetOwnerKind = "DaemonSet"

// SelfIdent resolves and caches the agent's own DaemonSet UID (deployment_id)
// and cluster id, for use as health-issue identity discriminators.
type SelfIdent struct {
	wmeta option.Option[workloadmeta.Component]

	once         sync.Once
	deploymentID string

	clusterIDOnce sync.Once
	clusterID     string
}

// New creates a SelfIdent. wmeta may be unset (e.g. builds without
// workloadmeta), in which case DeploymentID always resolves to empty and
// IssueDiscriminator falls back to the given host id.
func New(wmeta option.Option[workloadmeta.Component]) *SelfIdent {
	return &SelfIdent{wmeta: wmeta}
}

// DeploymentID returns the UID of the DaemonSet that owns this agent's pod,
// or "" if the agent is not running under a DaemonSet (not on Kubernetes, or
// no DaemonSet owner reference). Resolved once and cached for the process
// lifetime, since pod ownership cannot change without a pod restart.
func (s *SelfIdent) DeploymentID() string {
	s.once.Do(func() {
		s.deploymentID = s.resolveDeploymentID()
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

func (s *SelfIdent) resolveDeploymentID() string {
	wmeta, ok := s.wmeta.Get()
	if !ok {
		return ""
	}
	podName := os.Getenv(podNameEnvVar)
	if podName == "" {
		return ""
	}
	pod, err := wmeta.GetKubernetesPodByName(podName, namespace.GetMyNamespace())
	if err != nil {
		log.Debugf("selfident: could not find own pod %q in workloadmeta: %v", podName, err)
		return ""
	}
	for _, owner := range pod.Owners {
		if owner.Kind == daemonSetOwnerKind {
			return owner.ID
		}
	}
	return ""
}
