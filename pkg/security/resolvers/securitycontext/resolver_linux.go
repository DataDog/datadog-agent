// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"slices"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// wmetaSource is the subset of workloadmeta.Component this package uses, kept
// package-private so tests can inject a fake without Fx.
type wmetaSource interface {
	GetContainer(id string) (*workloadmeta.Container, error)
	GetKubernetesPodForContainer(containerID string) (*workloadmeta.KubernetesPod, error)
}

// WorkloadmetaResolver resolves the declared container security context via
// workloadmeta.
type WorkloadmetaResolver struct {
	wmeta wmetaSource
}

// NewWorkloadmetaResolver returns a resolver backed by wmeta; nil is safe.
func NewWorkloadmetaResolver(wmeta workloadmeta.Component) *WorkloadmetaResolver {
	if wmeta == nil {
		return &WorkloadmetaResolver{}
	}
	return &WorkloadmetaResolver{wmeta: wmeta}
}

// Resolve implements Resolver.
func (r *WorkloadmetaResolver) Resolve(id containerutils.ContainerID) (Key, *SecurityContext) {
	if r == nil || r.wmeta == nil || len(id) == 0 {
		return Key{}, nil
	}

	container, err := r.wmeta.GetContainer(string(id))
	if err != nil || container == nil || container.SecurityContext == nil {
		return Key{}, nil
	}

	// Pod lookup is best-effort so non-k8s containers still get keyed by name.
	key := Key{ContainerName: container.Name}
	if pod, err := r.wmeta.GetKubernetesPodForContainer(string(id)); err == nil && pod != nil {
		key.Namespace = pod.Namespace
		key.OwnerKind, key.OwnerName = walkToTopLevelOwner(pod)
	}

	sc := container.SecurityContext
	out := &SecurityContext{
		Privileged:               sc.Privileged,
		RunAsNonRoot:             copyBoolPtr(sc.RunAsNonRoot),
		AllowPrivilegeEscalation: copyBoolPtr(sc.AllowPrivilegeEscalation),
		ReadOnlyRootFilesystem:   copyBoolPtr(sc.ReadOnlyRootFilesystem),
	}
	if sc.Capabilities != nil {
		if len(sc.Capabilities.Add) > 0 {
			out.CapabilitiesAdd = slices.Clone(sc.Capabilities.Add)
		}
		if len(sc.Capabilities.Drop) > 0 {
			out.CapabilitiesDrop = slices.Clone(sc.Capabilities.Drop)
		}
	}
	if seccomp := seccompFromWmeta(sc.SeccompProfile); seccomp != nil {
		out.Seccomp = seccomp
	}
	return key, out
}

// walkToTopLevelOwner resolves the pod's OwnerReferences to its top-level
// controller. The kubelet only sees immediate owners, so ReplicaSet → Deployment
// and Job → CronJob are resolved via the shared pkg/util/kubernetes name-suffix
// heuristic. Falls back to ("Pod", pod name) when no owner is known.
func walkToTopLevelOwner(pod *workloadmeta.KubernetesPod) (kind, name string) {
	if pod == nil || len(pod.Owners) == 0 {
		return "Pod", pod.GetID().ID
	}
	owner := pod.Owners[0]
	switch owner.Kind {
	case kubernetes.ReplicaSetKind:
		if dep := kubernetes.ParseDeploymentForReplicaSet(owner.Name); dep != "" {
			return kubernetes.DeploymentKind, dep
		}
	case kubernetes.JobKind:
		if cj, _ := kubernetes.ParseCronJobForJob(owner.Name); cj != "" {
			return kubernetes.CronJobKind, cj
		}
	}
	return owner.Kind, owner.Name
}

// copyBoolPtr returns a fresh *bool with src's value, or nil if src is nil.
func copyBoolPtr(src *bool) *bool {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

func seccompFromWmeta(sp *workloadmeta.SeccompProfile) *SeccompProfile {
	if sp == nil {
		return nil
	}
	t := seccompTypeFromWmeta(sp.Type)
	if t == SeccompUnknown {
		return nil
	}
	out := &SeccompProfile{Type: t}
	if t == SeccompLocalhost {
		out.LocalhostProfile = sp.LocalhostProfile
	}
	return out
}

func seccompTypeFromWmeta(t workloadmeta.SeccompProfileType) SeccompProfileType {
	switch t {
	case workloadmeta.SeccompProfileTypeUnconfined:
		return SeccompUnconfined
	case workloadmeta.SeccompProfileTypeRuntimeDefault:
		return SeccompRuntimeDefault
	case workloadmeta.SeccompProfileTypeLocalhost:
		return SeccompLocalhost
	default:
		return SeccompUnknown
	}
}
