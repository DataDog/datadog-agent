// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// implicitCapabilities is the set every supported runtime (containerd, CRI-O,
// Docker) grants a container that declares none; the intersection, so a
// request can never add one a runtime would not have granted. It applies
// unless every node is known to grant dockerCapabilities.
var implicitCapabilities = []string{
	"CHOWN", "DAC_OVERRIDE", "FSETID", "FOWNER", "SETGID", "SETUID", "SETPCAP",
	"NET_BIND_SERVICE", "KILL",
}

// dockerCapabilities is the set containerd and Docker grant a container that
// declares none.
var dockerCapabilities = append(slices.Clone(implicitCapabilities),
	"AUDIT_WRITE", "MKNOD", "NET_RAW", "SETFCAP", "SYS_CHROOT")

// Defaults is what the cluster's runtimes grant a container that declares no
// capabilities.
type Defaults struct {
	// Capabilities are granted on every node.
	Capabilities []string
	// Exact is set when every node's runtime is known, so no node grants more.
	Exact bool
}

// fallbackDefaults applies until the nodes are listed.
var fallbackDefaults = Defaults{Capabilities: implicitCapabilities}

// RuntimeDefaults returns the Defaults of nodes whose
// status.nodeInfo.containerRuntimeVersion are runtimes (such as
// "containerd://1.7.2"). CRI-O, whose set is configurable, an unknown runtime,
// or no node at all, keep the intersection, and it is not exact.
// ponytail: assumes containerd's default base spec; one with fewer capabilities
// (base_runtime_spec) would let a request add some back.
func RuntimeDefaults(runtimes []string) Defaults {
	grantsDocker := func(runtime string) bool {
		return strings.HasPrefix(runtime, "containerd://") || strings.HasPrefix(runtime, "docker://")
	}
	if len(runtimes) == 0 || slices.ContainsFunc(runtimes, func(r string) bool { return !grantsDocker(r) }) {
		return fallbackDefaults
	}
	return Defaults{Capabilities: dockerCapabilities, Exact: true}
}

// FindContainer returns the container named name in spec.Containers, or nil.
// Init and ephemeral containers are never hardened.
func FindContainer(spec *corev1.PodSpec, name string) *corev1.Container {
	for i := range spec.Containers {
		if spec.Containers[i].Name == name {
			return &spec.Containers[i]
		}
	}
	return nil
}

// Render returns the target container's securityContext with req's control
// applied, or the reason it cannot be applied. It only ever narrows what the
// container may do, given the cluster's runtime defaults, and never modifies spec.
func Render(req *Request, spec *corev1.PodSpec, defaults Defaults) (*corev1.SecurityContext, string) {
	c := FindContainer(spec, req.Workload.Container)
	if c == nil {
		return nil, "container not found"
	}
	sc := &corev1.SecurityContext{}
	if c.SecurityContext != nil {
		sc = c.SecurityContext.DeepCopy()
	}
	if ptr.Deref(sc.Privileged, false) {
		return nil, "container is privileged"
	}

	switch req.Control {
	case ControlCapabilities:
		add, reason := keptCapabilities(req.Parameters.CapabilitiesAdd, sc.Capabilities, defaults)
		if reason != "" {
			return nil, reason
		}
		sc.Capabilities = &corev1.Capabilities{Add: add, Drop: []corev1.Capability{"ALL"}}
	case ControlNonRoot:
		// The kubelet would refuse to start the container, and the rollout would stall.
		if uid := runAsUser(spec, sc); uid != nil && *uid == 0 {
			return nil, "container runs as uid 0"
		}
		sc.RunAsNonRoot = ptr.To(true)
		sc.AllowPrivilegeEscalation = ptr.To(false)
	case ControlReadOnlyRootFS:
		sc.ReadOnlyRootFilesystem = ptr.To(true)
	default:
		return nil, fmt.Sprintf("unknown control %q", req.Control)
	}
	return sc, ""
}

// setsControl reports whether the target container in spec already applies
// req's control on its own, at least as strictly as req would.
func setsControl(req *Request, spec *corev1.PodSpec) bool {
	c := FindContainer(spec, req.Workload.Container)
	if c == nil {
		return false
	}
	sc := ptr.Deref(c.SecurityContext, corev1.SecurityContext{})
	switch req.Control {
	case ControlCapabilities:
		caps := ptr.Deref(sc.Capabilities, corev1.Capabilities{})
		return hasCapability(caps.Drop, "ALL") && !slices.ContainsFunc(caps.Add, func(a corev1.Capability) bool {
			return !slices.Contains(req.Parameters.CapabilitiesAdd, normalizeCapability(string(a)))
		})
	case ControlNonRoot:
		return ptr.Deref(runAsNonRoot(spec, &sc), false) && !ptr.Deref(sc.AllowPrivilegeEscalation, true)
	case ControlReadOnlyRootFS:
		return ptr.Deref(sc.ReadOnlyRootFilesystem, false)
	}
	return false
}

// runAsUser and runAsNonRoot return what a container with sc runs with in
// spec, where the pod's securityContext provides the default.
func runAsUser(spec *corev1.PodSpec, sc *corev1.SecurityContext) *int64 {
	if sc.RunAsUser == nil && spec.SecurityContext != nil {
		return spec.SecurityContext.RunAsUser
	}
	return sc.RunAsUser
}

func runAsNonRoot(spec *corev1.PodSpec, sc *corev1.SecurityContext) *bool {
	if sc.RunAsNonRoot == nil && spec.SecurityContext != nil {
		return spec.SecurityContext.RunAsNonRoot
	}
	return sc.RunAsNonRoot
}

// keptCapabilities returns the requested capabilities the container holds
// today, sorted. One it provably lacks is left out: the container already runs
// without it, so a check the workload makes for it keeps failing the same way.
// One only some nodes' runtime may grant makes the request unsafe.
func keptCapabilities(requested []string, declared *corev1.Capabilities, defaults Defaults) ([]corev1.Capability, string) {
	var add []corev1.Capability
	for _, name := range requested {
		switch capabilityGrant(declared, name, defaults) {
		case grantUnknown:
			return nil, fmt.Sprintf("capability %s is granted by default on some runtimes only", name)
		case granted:
			add = append(add, corev1.Capability(name))
		}
	}
	slices.Sort(add)
	return slices.Compact(add), ""
}

type grant int

const (
	granted grant = iota
	notGranted
	grantUnknown
)

// capabilityGrant reports whether a container declaring caps runs with
// capability name. The runtimes apply add:[ALL] (every capability), then
// drop:[ALL] (nothing), then the individual adds, then the individual drops —
// so a specific entry always wins over ALL, and drop always wins over add.
// Checked in that order: a specific drop loses the capability; a specific add
// (or ALL) grants it; drop:[ALL] with no matching specific add then denies it;
// otherwise the runtime default decides, which is unknown for a capability
// outside defaults.Capabilities unless defaults is exact.
func capabilityGrant(caps *corev1.Capabilities, name string, defaults Defaults) grant {
	var add, drop []corev1.Capability
	if caps != nil {
		add, drop = caps.Add, caps.Drop
	}
	switch {
	case hasCapability(drop, name):
		return notGranted
	case hasCapability(add, name):
		return granted
	case hasCapability(drop, "ALL"):
		return notGranted
	case hasCapability(add, "ALL"):
		return granted
	case slices.Contains(defaults.Capabilities, name):
		return granted
	case defaults.Exact:
		return notGranted
	}
	return grantUnknown
}

func hasCapability(list []corev1.Capability, name string) bool {
	return slices.ContainsFunc(list, func(c corev1.Capability) bool { return normalizeCapability(string(c)) == name })
}
