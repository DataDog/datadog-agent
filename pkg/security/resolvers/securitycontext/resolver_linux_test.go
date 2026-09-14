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

// fakeWmeta implements wmetaSource for tests.
type fakeWmeta struct {
	containers map[string]*workloadmeta.Container
	pods       map[string]*workloadmeta.KubernetesPod
}

func (f *fakeWmeta) GetContainer(id string) (*workloadmeta.Container, error) {
	if c, ok := f.containers[id]; ok {
		return c, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeWmeta) GetKubernetesPodForContainer(id string) (*workloadmeta.KubernetesPod, error) {
	if p, ok := f.pods[id]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

func newPod(id, namespace string, owners []workloadmeta.KubernetesPodOwner) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   id,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      id,
			Namespace: namespace,
		},
		Owners: owners,
	}
}

func TestWorkloadmetaResolver_NoWmeta(t *testing.T) {
	r := NewWorkloadmetaResolver(nil)
	key, sc := r.Resolve(containerutils.ContainerID("cid"))
	assert.True(t, key.IsZero())
	assert.Nil(t, sc)
}

func TestWorkloadmetaResolver_EmptyID(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{}}
	key, sc := r.Resolve(containerutils.ContainerID(""))
	assert.True(t, key.IsZero())
	assert.Nil(t, sc)
}

func TestWorkloadmetaResolver_NotFound(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{}}
	key, sc := r.Resolve(containerutils.ContainerID("unknown-cid"))
	assert.True(t, key.IsZero())
	assert.Nil(t, sc)
}

func TestWorkloadmetaResolver_NoSecurityContext(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{
		containers: map[string]*workloadmeta.Container{"cid": {}},
	}}
	key, sc := r.Resolve(containerutils.ContainerID("cid"))
	assert.True(t, key.IsZero())
	assert.Nil(t, sc)
}

func TestWorkloadmetaResolver_PrivilegedOnly(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{Privileged: true},
			},
		},
	}}
	key, sc := r.Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	assert.True(t, sc.Privileged)
	assert.Nil(t, sc.Seccomp)
	assert.Nil(t, sc.CapabilitiesAdd)
	assert.Nil(t, sc.CapabilitiesDrop)
	assert.Equal(t, Key{ContainerName: "nginx"}, key)
}

func TestWorkloadmetaResolver_EmptySecurityContext(t *testing.T) {
	r := &WorkloadmetaResolver{wmeta: &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{},
			},
		},
	}}
	key, sc := r.Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	assert.False(t, sc.Privileged)
	assert.Nil(t, sc.CapabilitiesAdd)
	assert.Nil(t, sc.CapabilitiesDrop)
	assert.Nil(t, sc.Seccomp)
	assert.Nil(t, sc.RunAsNonRoot)
	assert.Nil(t, sc.AllowPrivilegeEscalation)
	assert.Nil(t, sc.ReadOnlyRootFilesystem)
	assert.Equal(t, Key{ContainerName: "nginx"}, key)
}

func TestWorkloadmetaResolver_TriStateBooleansPreserveExplicitFalse(t *testing.T) {
	no, yes := false, true
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta: workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					RunAsNonRoot:             &yes,
					AllowPrivilegeEscalation: &no,
					ReadOnlyRootFilesystem:   &yes,
				},
			},
		},
	}
	_, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.ReadOnlyRootFilesystem)
	assert.True(t, *sc.ReadOnlyRootFilesystem)
}

func TestWorkloadmetaResolver_TriStateBooleansCloned(t *testing.T) {
	yes := true
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{RunAsNonRoot: &yes},
			},
		},
	}
	_, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	require.NotNil(t, sc.RunAsNonRoot)
	*sc.RunAsNonRoot = false
	assert.True(t, *src.containers["cid"].SecurityContext.RunAsNonRoot)
}

func TestWorkloadmetaResolver_Full(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta: workloadmeta.EntityMeta{Name: "nginx"},
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
	_, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	assert.True(t, sc.Privileged)
	assert.Equal(t, []string{"NET_ADMIN", "SYS_PTRACE"}, sc.CapabilitiesAdd)
	assert.Equal(t, []string{"MKNOD"}, sc.CapabilitiesDrop)
	require.NotNil(t, sc.Seccomp)
	assert.Equal(t, SeccompLocalhost, sc.Seccomp.Type)
	assert.Equal(t, "profiles/audit.json", sc.Seccomp.LocalhostProfile)
}

func TestWorkloadmetaResolver_CapabilitiesCloned(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta: workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					Capabilities: &workloadmeta.Capabilities{Add: []string{"NET_ADMIN"}},
				},
			},
		},
	}
	_, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	require.Len(t, sc.CapabilitiesAdd, 1)
	sc.CapabilitiesAdd[0] = "MUTATED"
	assert.Equal(t, "NET_ADMIN", src.containers["cid"].SecurityContext.Capabilities.Add[0])
}

func TestWorkloadmetaResolver_SeccompTypeAbsentNotSet(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta: workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					Privileged:     true,
					SeccompProfile: &workloadmeta.SeccompProfile{Type: ""},
				},
			},
		},
	}
	_, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	assert.Nil(t, sc.Seccomp)
}

func TestWorkloadmetaResolver_SeccompLocalhostPathIgnoredForNonLocalhost(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta: workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{
					SeccompProfile: &workloadmeta.SeccompProfile{
						Type:             workloadmeta.SeccompProfileTypeRuntimeDefault,
						LocalhostProfile: "bogus.json",
					},
				},
			},
		},
	}
	_, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	require.NotNil(t, sc.Seccomp)
	assert.Equal(t, SeccompRuntimeDefault, sc.Seccomp.Type)
	assert.Empty(t, sc.Seccomp.LocalhostProfile)
}

func TestWorkloadmetaResolver_KeyFromDeploymentOwnedPod(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "nginx"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{Privileged: true},
			},
		},
		pods: map[string]*workloadmeta.KubernetesPod{
			"cid": newPod("frontend-web-abc", "frontend", []workloadmeta.KubernetesPodOwner{
				{Kind: "ReplicaSet", Name: "web-56c89cfff7"},
			}),
		},
	}
	key, sc := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	require.NotNil(t, sc)
	assert.Equal(t, Key{
		Namespace:     "frontend",
		OwnerKind:     "Deployment",
		OwnerName:     "web",
		ContainerName: "nginx",
	}, key)
}

func TestWorkloadmetaResolver_KeyFromCronJobOwnedPod(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "backup"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{},
			},
		},
		pods: map[string]*workloadmeta.KubernetesPod{
			"cid": newPod("backup-1725969600-xyz", "batch", []workloadmeta.KubernetesPodOwner{
				{Kind: "Job", Name: "nightly-1725969600"},
			}),
		},
	}
	key, _ := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	assert.Equal(t, Key{
		Namespace:     "batch",
		OwnerKind:     "CronJob",
		OwnerName:     "nightly",
		ContainerName: "backup",
	}, key)
}

func TestWorkloadmetaResolver_KeyFromDaemonSetOwnedPod(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "agent"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{},
			},
		},
		pods: map[string]*workloadmeta.KubernetesPod{
			"cid": newPod("datadog-agent-xyz", "datadog", []workloadmeta.KubernetesPodOwner{
				{Kind: "DaemonSet", Name: "datadog-agent"},
			}),
		},
	}
	key, _ := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	assert.Equal(t, Key{
		Namespace:     "datadog",
		OwnerKind:     "DaemonSet",
		OwnerName:     "datadog-agent",
		ContainerName: "agent",
	}, key)
}

func TestWorkloadmetaResolver_KeyFallsBackToPodForBarePod(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "debug"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{},
			},
		},
		pods: map[string]*workloadmeta.KubernetesPod{
			"cid": newPod("adhoc-debug", "default", nil),
		},
	}
	key, _ := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	assert.Equal(t, Key{
		Namespace:     "default",
		OwnerKind:     "Pod",
		OwnerName:     "adhoc-debug",
		ContainerName: "debug",
	}, key)
}

func TestWorkloadmetaResolver_KeyFallsBackWhenReplicaSetIsNotDeploymentManaged(t *testing.T) {
	src := &fakeWmeta{
		containers: map[string]*workloadmeta.Container{
			"cid": {
				EntityMeta:      workloadmeta.EntityMeta{Name: "worker"},
				SecurityContext: &workloadmeta.ContainerSecurityContext{},
			},
		},
		pods: map[string]*workloadmeta.KubernetesPod{
			"cid": newPod("manually-created-abc", "workshop", []workloadmeta.KubernetesPodOwner{
				{Kind: "ReplicaSet", Name: "manually-created"},
			}),
		},
	}
	key, _ := (&WorkloadmetaResolver{wmeta: src}).Resolve(containerutils.ContainerID("cid"))
	assert.Equal(t, Key{
		Namespace:     "workshop",
		OwnerKind:     "ReplicaSet",
		OwnerName:     "manually-created",
		ContainerName: "worker",
	}, key)
}

func TestNoopResolver(t *testing.T) {
	var r Resolver = NoopResolver{}
	key, sc := r.Resolve(containerutils.ContainerID("anything"))
	assert.True(t, key.IsZero())
	assert.Nil(t, sc)
}
