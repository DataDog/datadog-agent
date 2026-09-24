// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package hardening

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

// implicitCapabilities is the set every supported runtime (containerd, CRI-O,
// Docker) grants a container that declares none; the intersection, so a
// request can never add one a runtime would not have granted.
var implicitCapabilities = []string{
	"CHOWN", "DAC_OVERRIDE", "FSETID", "FOWNER", "SETGID", "SETUID", "SETPCAP",
	"NET_BIND_SERVICE", "KILL",
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
// container may do, and never modifies spec.
func Render(req *Request, spec *corev1.PodSpec) (*corev1.SecurityContext, string) {
	c := FindContainer(spec, req.Target.Container)
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
		var add []corev1.Capability
		for _, name := range req.Parameters.CapabilitiesAdd {
			if !isGranted(sc.Capabilities, name) {
				return nil, fmt.Sprintf("capability %s is not granted to the container today", name)
			}
			add = append(add, corev1.Capability(name))
		}
		slices.Sort(add)
		sc.Capabilities = &corev1.Capabilities{Add: slices.Compact(add), Drop: []corev1.Capability{"ALL"}}
	case ControlReadOnlyRootFS:
		sc.ReadOnlyRootFilesystem = ptr.To(true)
	case ControlSeccompRuntimeDefault:
		profile := sc.SeccompProfile
		if profile == nil && spec.SecurityContext != nil {
			profile = spec.SecurityContext.SeccompProfile
		}
		if profile != nil && profile.Type == corev1.SeccompProfileTypeLocalhost {
			return nil, "container has a Localhost seccomp profile"
		}
		sc.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	default:
		return nil, fmt.Sprintf("unknown control %q", req.Control)
	}
	return sc, ""
}

// isGranted reports whether a container declaring caps runs with capability
// name. The runtimes apply add:[ALL] (every capability), then drop:[ALL]
// (nothing), then the individual adds, then the individual drops — so a
// specific entry always wins over ALL, and drop always wins over add. Checked
// in that order: a specific drop loses the capability; a specific add (or
// ALL) grants it; drop:[ALL] with no matching specific add then denies it;
// otherwise the container gets the implicit default set.
func isGranted(caps *corev1.Capabilities, name string) bool {
	var add, drop []corev1.Capability
	if caps != nil {
		add, drop = caps.Add, caps.Drop
	}
	switch {
	case hasCapability(drop, name):
		return false
	case hasCapability(add, name):
		return true
	case hasCapability(drop, "ALL"):
		return false
	case hasCapability(add, "ALL"):
		return true
	}
	return slices.Contains(implicitCapabilities, name)
}

func hasCapability(list []corev1.Capability, name string) bool {
	return slices.ContainsFunc(list, func(c corev1.Capability) bool { return normalizeCapability(string(c)) == name })
}
