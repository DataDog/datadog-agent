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

// Render returns the container's securityContext with control applied, or
// the reason it cannot be applied. It only ever narrows what the container
// may do, and never modifies spec.
func Render(control Control, container string, spec *corev1.PodSpec) (*corev1.SecurityContext, string) {
	c := FindContainer(spec, container)
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
	switch control {
	case ControlCapabilities:
		add, reason := keptCapabilities(sc.Capabilities)
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
	case ControlNoPrivilegeEscalation:
		sc.AllowPrivilegeEscalation = ptr.To(false)
	default:
		return nil, fmt.Sprintf("unknown control %q", control)
	}
	return sc, ""
}

// keptCapabilities returns the manifest's add list without the capabilities
// it also drops, which the container never held.
func keptCapabilities(declared *corev1.Capabilities) ([]corev1.Capability, string) {
	if declared == nil {
		return nil, ""
	}
	if hasCapability(declared.Add, "ALL") {
		return nil, "container adds ALL capabilities"
	}
	if hasCapability(declared.Drop, "ALL") {
		return slices.Clone(declared.Add), ""
	}
	return slices.DeleteFunc(slices.Clone(declared.Add), func(c corev1.Capability) bool {
		return hasCapability(declared.Drop, normalizeCapability(string(c)))
	}), ""
}

// SetsControl reports whether the container in spec already applies control
// on its own.
func SetsControl(control Control, container string, spec *corev1.PodSpec) bool {
	c := FindContainer(spec, container)
	if c == nil {
		return false
	}
	sc := ptr.Deref(c.SecurityContext, corev1.SecurityContext{})
	switch control {
	case ControlCapabilities:
		return hasCapability(ptr.Deref(sc.Capabilities, corev1.Capabilities{}).Drop, "ALL")
	case ControlNonRoot:
		return ptr.Deref(runAsNonRoot(spec, &sc), false)
	case ControlNoPrivilegeEscalation:
		return !ptr.Deref(sc.AllowPrivilegeEscalation, true)
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

// normalizeCapability returns the Kubernetes spelling of a capability name.
func normalizeCapability(c string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")
}

func hasCapability(list []corev1.Capability, name string) bool {
	return slices.ContainsFunc(list, func(c corev1.Capability) bool { return normalizeCapability(string(c)) == name })
}
