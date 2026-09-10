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
)

// wmetaSource is the subset of workloadmeta.Component this package uses.
// Kept package-private so tests can inject a fake without Fx.
type wmetaSource interface {
	GetContainer(id string) (*workloadmeta.Container, error)
}

// WorkloadmetaResolver resolves the declared container security context via
// workloadmeta. Only the kubelet collector populates SecurityContext today.
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
//
// Wire convention: nil == unknown posture; a present SecurityContext means
// workloadmeta had an answer, so proto3 defaults on sub-fields are meaningful.
func (r *WorkloadmetaResolver) Resolve(id containerutils.ContainerID) *SecurityContext {
	if r == nil || r.wmeta == nil || len(id) == 0 {
		return nil
	}

	container, err := r.wmeta.GetContainer(string(id))
	if err != nil || container == nil || container.SecurityContext == nil {
		return nil
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
	return out
}

// copyBoolPtr returns a fresh *bool with src's value, or nil if src is nil,
// so a mutation on the returned SecurityContext can never leak back into the
// workloadmeta cache.
func copyBoolPtr(src *bool) *bool {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// seccompFromWmeta returns nil for unknown/empty types so "no declared
// seccomp" doesn't collide with a real value on the wire.
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
