// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package securitycontext resolves the declared container security context for a workload.
package securitycontext

import "github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"

// SeccompProfileType mirrors adproto.SeccompProfile_Type.
type SeccompProfileType uint8

// Seccomp profile types. Zero value is Unknown.
const (
	SeccompUnknown SeccompProfileType = iota
	SeccompUnconfined
	SeccompRuntimeDefault
	SeccompLocalhost
)

// SeccompProfile is the declared seccomp profile.
type SeccompProfile struct {
	Type             SeccompProfileType
	LocalhostProfile string
}

// SecurityContext holds the declared container security context. *bool fields
// preserve Kubernetes tri-state semantics (nil = unset, not false).
type SecurityContext struct {
	Privileged               bool
	Seccomp                  *SeccompProfile
	CapabilitiesAdd          []string
	CapabilitiesDrop         []string
	RunAsNonRoot             *bool
	AllowPrivilegeEscalation *bool
	ReadOnlyRootFilesystem   *bool
}

// Key identifies a container slot in a Kubernetes workload template. Pod name
// is intentionally not part of the key so replicas, restarts, and rollouts
// collapse into a single entry.
type Key struct {
	Namespace     string
	OwnerKind     string
	OwnerName     string
	ContainerName string
}

// IsZero reports whether k carries no attribution data.
func (k Key) IsZero() bool { return k == Key{} }

// Resolver resolves the declared container security context of a workload.
// It returns (Key{}, nil) when no data is available.
type Resolver interface {
	Resolve(id containerutils.ContainerID) (Key, *SecurityContext)
}

// NoopResolver always returns (Key{}, nil).
type NoopResolver struct{}

// Resolve implements Resolver.
func (NoopResolver) Resolve(_ containerutils.ContainerID) (Key, *SecurityContext) {
	return Key{}, nil
}
