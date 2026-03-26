// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stringDataPtr returns the pointer to the underlying byte array of a string.
// Two strings sharing backing memory will have the same data pointer.
func stringDataPtr(s string) uintptr {
	return (*(*[2]uintptr)(unsafe.Pointer(&s)))[0]
}

func TestInternStrings_EntityMeta(t *testing.T) {
	a := EntityMeta{
		Name:      "my-pod",
		Namespace: "production",
		Labels: map[string]string{
			"app": "web",
		},
		Annotations: map[string]string{
			"note": "value",
		},
		UID: "unique-uid-123",
	}
	b := EntityMeta{
		Name:      string([]byte("my-pod")),    // force distinct allocation
		Namespace: string([]byte("production")), // force distinct allocation
		Labels: map[string]string{
			string([]byte("app")): string([]byte("web")),
		},
		UID: "different-uid",
	}

	// Before interning, the strings have different backing arrays.
	require.NotEqual(t, stringDataPtr(a.Name), stringDataPtr(b.Name),
		"pre-condition: distinct allocations should have different data pointers")

	a.InternStrings()
	b.InternStrings()

	// After interning, identical strings should share the same backing memory.
	assert.Equal(t, stringDataPtr(a.Name), stringDataPtr(b.Name),
		"interned 'my-pod' should share backing memory")
	assert.Equal(t, stringDataPtr(a.Namespace), stringDataPtr(b.Namespace),
		"interned 'production' should share backing memory")

	// Label keys and values should be interned too.
	var aLabelKey, bLabelKey string
	for k := range a.Labels {
		aLabelKey = k
	}
	for k := range b.Labels {
		bLabelKey = k
	}
	assert.Equal(t, stringDataPtr(aLabelKey), stringDataPtr(bLabelKey),
		"interned label key 'app' should share backing memory")

	// UID should be unchanged (not tagged).
	assert.Equal(t, "unique-uid-123", a.UID)

	// Nil maps should stay nil.
	c := EntityMeta{}
	c.InternStrings()
	assert.Nil(t, c.Labels)
	assert.Nil(t, c.Annotations)
}

func TestInternStrings_KubernetesPod_Recursive(t *testing.T) {
	pod := KubernetesPod{
		EntityMeta: EntityMeta{
			Namespace: string([]byte("kube-system")),
			Labels: map[string]string{
				string([]byte("component")): string([]byte("kube-proxy")),
			},
		},
		Phase:    string([]byte("Running")),
		QOSClass: string([]byte("BestEffort")),
		NodeName: string([]byte("node-1")),
		Owners: []KubernetesPodOwner{
			{Kind: string([]byte("DaemonSet")), Name: string([]byte("kube-proxy"))},
		},
		Tolerations: []KubernetesPodToleration{
			{Key: string([]byte("node.kubernetes.io/not-ready")), Effect: string([]byte("NoSchedule"))},
		},
		Conditions: []KubernetesPodCondition{
			{Type: string([]byte("Ready")), Status: string([]byte("True"))},
		},
	}

	pod2 := KubernetesPod{
		EntityMeta: EntityMeta{
			Namespace: string([]byte("kube-system")),
		},
		Phase:    string([]byte("Running")),
		NodeName: string([]byte("node-1")),
		Owners: []KubernetesPodOwner{
			{Kind: string([]byte("DaemonSet"))},
		},
	}

	pod.InternStrings()
	pod2.InternStrings()

	assert.Equal(t, stringDataPtr(pod.Phase), stringDataPtr(pod2.Phase))
	assert.Equal(t, stringDataPtr(pod.NodeName), stringDataPtr(pod2.NodeName))
	assert.Equal(t, stringDataPtr(pod.Namespace), stringDataPtr(pod2.Namespace))
	assert.Equal(t, stringDataPtr(pod.Owners[0].Kind), stringDataPtr(pod2.Owners[0].Kind))
}

func TestInternStrings_Container(t *testing.T) {
	c := Container{
		EntityMeta: EntityMeta{
			Name: string([]byte("main")),
		},
		EnvVars: map[string]string{
			string([]byte("DD_AGENT_HOST")): string([]byte("localhost")),
		},
		Hostname: string([]byte("my-host")),
		Image: ContainerImage{
			Registry:  string([]byte("gcr.io")),
			ShortName: string([]byte("my-app")),
			Tag:       string([]byte("latest")),
		},
		CollectorTags: []string{string([]byte("env:prod"))},
		SecurityContext: &ContainerSecurityContext{
			Capabilities: &Capabilities{
				Drop: []string{string([]byte("NET_RAW"))},
			},
		},
		Resources: ContainerResources{
			GPUVendorList: []string{string([]byte("nvidia"))},
			RawRequests: map[string]string{
				string([]byte("cpu")): string([]byte("100m")),
			},
		},
	}

	c.InternStrings()

	assert.Equal(t, "main", c.EntityMeta.Name)
	assert.Equal(t, "localhost", c.EnvVars["DD_AGENT_HOST"])
	assert.Equal(t, "my-host", c.Hostname)
	assert.Equal(t, "gcr.io", c.Image.Registry)
	assert.Equal(t, "latest", c.Image.Tag)
	assert.Equal(t, []string{"env:prod"}, c.CollectorTags)
	assert.Equal(t, []string{"NET_RAW"}, c.SecurityContext.Capabilities.Drop)
	assert.Equal(t, []string{"nvidia"}, c.Resources.GPUVendorList)
	assert.Equal(t, "100m", c.Resources.RawRequests["cpu"])
}

func TestInternStrings_NilSafety(t *testing.T) {
	c := Container{}
	assert.NotPanics(t, func() { c.InternStrings() })

	p := KubernetesPod{}
	assert.NotPanics(t, func() { p.InternStrings() })

	sc := ContainerSecurityContext{}
	assert.NotPanics(t, func() { sc.InternStrings() })

	ks := KubernetesContainerState{}
	assert.NotPanics(t, func() { ks.InternStrings() })
}
