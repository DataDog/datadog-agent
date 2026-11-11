// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"slices"
	"sync"
	"time"

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

	kubeletLock       sync.Mutex
	kubelet           *kubeletInfo
	kubeletLookupTime time.Time
	// seccompDefaultEnabled mirrors the kubelet's --seccomp-default setting.
	seccompDefaultEnabled bool
}

// kubeletLookupInterval bounds how often /proc is scanned for a kubelet that
// restarted or wasn't running yet.
const kubeletLookupInterval = time.Minute

// NewWorkloadmetaResolver returns a resolver backed by wmeta; nil is safe.
func NewWorkloadmetaResolver(wmeta workloadmeta.Component) *WorkloadmetaResolver {
	kubelet := findKubelet()
	r := &WorkloadmetaResolver{
		kubelet:               kubelet,
		kubeletLookupTime:     time.Now(),
		seccompDefaultEnabled: kubeletSeccompDefaultEnabled(kubelet),
	}
	if wmeta != nil {
		r.wmeta = wmeta
	}
	return r
}

// Resolve implements Resolver.
func (r *WorkloadmetaResolver) Resolve(id containerutils.ContainerID) (Key, *SecurityContext) {
	if r == nil || r.wmeta == nil || len(id) == 0 {
		return Key{}, nil
	}

	container, err := r.wmeta.GetContainer(string(id))
	if err != nil || container == nil {
		return Key{}, nil
	}

	// Pod lookup is best-effort so non-k8s containers still get keyed by name.
	key := Key{ContainerName: container.Name}
	var podSC *workloadmeta.PodSecurityContext
	hasPod := false
	if pod, err := r.wmeta.GetKubernetesPodForContainer(string(id)); err == nil && pod != nil {
		hasPod = true
		key.Namespace = pod.Namespace
		key.OwnerKind, key.OwnerName = walkToTopLevelOwner(pod)
		podSC = pod.SecurityContext
	}

	sc := container.SecurityContext
	out := &SecurityContext{}
	if sc != nil {
		out.Privileged = sc.Privileged
		out.RunAsNonRoot = copyBoolPtr(sc.RunAsNonRoot)
		out.AllowPrivilegeEscalation = copyBoolPtr(sc.AllowPrivilegeEscalation)
		out.ReadOnlyRootFilesystem = copyBoolPtr(sc.ReadOnlyRootFilesystem)
		if sc.Capabilities != nil {
			if len(sc.Capabilities.Add) > 0 {
				out.CapabilitiesAdd = slices.Clone(sc.Capabilities.Add)
			}
			if len(sc.Capabilities.Drop) > 0 {
				out.CapabilitiesDrop = slices.Clone(sc.Capabilities.Drop)
			}
		}
		out.Seccomp = seccompFromWmeta(sc.SeccompProfile)
	}

	// Kubernetes lets a pod set seccomp and runAsNonRoot for all its containers;
	// a container that declares its own overrides the pod. Fall back to the pod
	// value only when the container left it unset.
	if podSC != nil {
		if out.Seccomp == nil {
			out.Seccomp = seccompFromWmeta(podSC.SeccompProfile)
		}
		if out.RunAsNonRoot == nil {
			out.RunAsNonRoot = copyBoolPtr(podSC.RunAsNonRoot)
		}
	}

	// When neither the container nor the pod declares a seccomp profile, the
	// kubelet applies RuntimeDefault if --seccomp-default is enabled on the node.
	if hasPod && out.Seccomp == nil {
		if _, seccompDefault := r.currentKubelet(); seccompDefault {
			out.Seccomp = &SeccompProfile{Type: SeccompRuntimeDefault}
		}
	}

	if sc == nil && podSC == nil && out.Seccomp == nil {
		return Key{}, nil
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

// ResolveSeccompFilter returns the effective seccomp filter for a declared
// profile. Localhost profiles are read from the kubelet seccomp directory on
// the node; other types have no on-disk filter and return nil.
func (r *WorkloadmetaResolver) ResolveSeccompFilter(profile *SeccompProfile) (*SeccompFilterResult, error) {
	if profile == nil || profile.Type != SeccompLocalhost {
		return nil, nil
	}
	kubelet, _ := r.currentKubelet()
	return readLocalhostSeccompProfile(kubelet, profile.LocalhostProfile)
}

// currentKubelet returns the cached kubelet and its seccomp default, looking
// the kubelet up again when its process is gone or its PID was reused.
func (r *WorkloadmetaResolver) currentKubelet() (*kubeletInfo, bool) {
	r.kubeletLock.Lock()
	defer r.kubeletLock.Unlock()

	if r.kubelet != nil && r.kubelet.pid != "" {
		if _, ok := kubeletArgs(r.kubelet.pid); ok {
			return r.kubelet, r.seccompDefaultEnabled
		}
		// Copy since callers read it unlocked.
		stale := *r.kubelet
		stale.pid = ""
		r.kubelet = &stale
	}
	if now := time.Now(); now.Sub(r.kubeletLookupTime) >= kubeletLookupInterval {
		r.kubeletLookupTime = now
		r.kubelet = findKubelet()
		r.seccompDefaultEnabled = kubeletSeccompDefaultEnabled(r.kubelet)
	}
	return r.kubelet, r.seccompDefaultEnabled
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
