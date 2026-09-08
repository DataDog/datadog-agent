// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package securitycontext resolves the declared hardening posture for a workload.
package securitycontext

import "github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"

// SeccompProfileType mirrors adproto.SeccompProfile_Type.
type SeccompProfileType uint8

// Zero value is the "unknown" sentinel so an unset field reads as "no data".
const (
	SeccompUnknown SeccompProfileType = iota
	SeccompUnconfined
	SeccompRuntimeDefault
	SeccompLocalhost
)

// SeccompProfile is the declared seccomp profile. LocalhostProfile is set
// only when Type == SeccompLocalhost.
type SeccompProfile struct {
	Type             SeccompProfileType
	LocalhostProfile string
}

// Declared mirrors adproto.HardeningDeclared.
type Declared struct {
	Privileged       bool
	Seccomp          *SeccompProfile
	CapabilitiesAdd  []string
	CapabilitiesDrop []string
}

// Resolver resolves the declared hardening posture of a workload.
// Implementations MUST return nil when no data is available.
type Resolver interface {
	Resolve(id containerutils.ContainerID) *Declared
}

// NoopResolver always returns nil.
type NoopResolver struct{}

// Resolve implements Resolver.
func (NoopResolver) Resolve(_ containerutils.ContainerID) *Declared { return nil }
