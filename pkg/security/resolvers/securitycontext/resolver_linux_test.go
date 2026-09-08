// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securitycontext

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

type fakeWmeta struct {
	containers map[string]*workloadmeta.Container
}

func (f *fakeWmeta) GetContainer(id string) (*workloadmeta.Container, error) {
	if c, ok := f.containers[id]; ok {
		return c, nil
	}
	return nil, errors.New("not found")
}

func TestWorkloadmetaResolver_NoWmeta(t *testing.T) {
	r := NewWorkloadmetaResolver(nil)
	assert.Nil(t, r.Resolve(containerutils.ContainerID("cid")))
}

func TestWorkloadmetaResolver_EmptyID(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{}}
	assert.Nil(t, r.Resolve(containerutils.ContainerID("")))
}

func TestWorkloadmetaResolver_NotFound(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{}}
	assert.Nil(t, r.Resolve(containerutils.ContainerID("unknown-cid")))
}

func TestWorkloadmetaResolver_NoSecurityContext(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{
		containers: map[string]*workloadmeta.Container{"cid": {}},
	}}
	assert.Nil(t, r.Resolve(containerutils.ContainerID("cid")))
}

func TestWorkloadmetaResolver_PrivilegedOnly(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {SecurityContext: &workloadmeta.ContainerSecurityContext{Privileged: true}},
		},
	}}
	got := r.Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, got)
	assert.True(t, got.Privileged)
	assert.Nil(t, got.Seccomp)
	assert.Nil(t, got.CapabilitiesAdd)
	assert.Nil(t, got.CapabilitiesDrop)
}

func TestWorkloadmetaResolver_EmptySecurityContext(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {SecurityContext: &workloadmeta.ContainerSecurityContext{}},
		},
	}}
	assert.Nil(t, r.Resolve(containerutils.ContainerID("cid")))
}

func TestWorkloadmetaResolver_Full(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					Privileged: true,
					Capabilities: &workloadmeta.Capabilities{
						Add:  []string{"NET_ADMIN", "SYS_PTRACE"},
						Drop: []string{"MKNOD"},
					},
					SeccompProfile: &workloadmeta.SeccompProfile{
						Type:             workloadmeta.SeccompProfileTypeLocalhost,
						LocalhostProfile: "profiles/audit.json",
					},
				},
			},
		},
	}
	got := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, got)
	assert.True(t, got.Privileged)
	assert.Equal(t, []string{"NET_ADMIN", "SYS_PTRACE"}, got.CapabilitiesAdd)
	assert.Equal(t, []string{"MKNOD"}, got.CapabilitiesDrop)
	require.NotNil(t, got.Seccomp)
	assert.Equal(t, SeccompLocalhost, got.Seccomp.Type)
	assert.Equal(t, "profiles/audit.json", got.Seccomp.LocalhostProfile)
}

func TestWorkloadmetaResolver_CapabilitiesCloned(t *testing.T) {
	// Mutating the returned slice must not corrupt the shared wmeta cache.
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					Capabilities: &workloadmeta.Capabilities{Add: []string{"NET_ADMIN"}},
				},
			},
		},
	}
	got := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, got)
	require.Len(t, got.CapabilitiesAdd, 1)
	got.CapabilitiesAdd[0] = "MUTATED"
	assert.Equal(t, "NET_ADMIN", src.containers["cid"].SecurityContext.Capabilities.Add[0])
}

func TestWorkloadmetaResolver_SeccompTypeAbsentNotSet(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					Privileged:     true,
					SeccompProfile: &workloadmeta.SeccompProfile{Type: ""},
				},
			},
		},
	}
	got := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, got)
	assert.Nil(t, got.Seccomp)
}

func TestWorkloadmetaResolver_SeccompLocalhostPathIgnoredForNonLocalhost(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					SeccompProfile: &workloadmeta.SeccompProfile{
						Type:             workloadmeta.SeccompProfileTypeRuntimeDefault,
						LocalhostProfile: "bogus.json",
					},
				},
			},
		},
	}
	got := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, got)
	require.NotNil(t, got.Seccomp)
	assert.Equal(t, SeccompRuntimeDefault, got.Seccomp.Type)
	assert.Empty(t, got.Seccomp.LocalhostProfile)
}

func TestNoopResolver(t *testing.T) {
	var r Resolver = NoopResolver{}
	assert.Nil(t, r.Resolve(containerutils.ContainerID("anything")))
}
