// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestResolveCoreV1PodOwner(t *testing.T) {
	t.Run("no owner references returns false", func(t *testing.T) {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default"}}
		_, ok := resolveCoreV1PodOwner(pod)
		assert.False(t, ok)
	})

	t.Run("deployment owner is filtered out", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       "default",
				OwnerReferences: []metav1.OwnerReference{{Kind: kubernetes.DeploymentKind, Name: "nginx"}},
			},
		}
		_, ok := resolveCoreV1PodOwner(pod)
		assert.False(t, ok)
	})

	t.Run("replicaset owner is returned", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       "default",
				OwnerReferences: []metav1.OwnerReference{{Kind: kubernetes.ReplicaSetKind, Name: "nginx-bcdfg"}},
			},
		}
		key, ok := resolveCoreV1PodOwner(pod)
		assert.True(t, ok)
		assert.Equal(t, kubernetes.ReplicaSetKind, key.Kind)
		assert.Equal(t, "nginx-bcdfg", key.Name)
		assert.Equal(t, "default", key.Namespace)
	})
}

func TestResolveWLMPodOwner(t *testing.T) {
	t.Run("no owners returns false", func(t *testing.T) {
		pod := &workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{Namespace: "default"},
		}
		_, ok := resolveWLMPodOwner(pod)
		assert.False(t, ok)
	})

	t.Run("deployment owner is filtered out", func(t *testing.T) {
		pod := &workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{Namespace: "default"},
			Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.DeploymentKind, Name: "nginx"}},
		}
		_, ok := resolveWLMPodOwner(pod)
		assert.False(t, ok)
	})

	t.Run("replicaset owner is returned", func(t *testing.T) {
		pod := &workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{Namespace: "default"},
			Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "nginx-bcdfg"}},
		}
		key, ok := resolveWLMPodOwner(pod)
		assert.True(t, ok)
		assert.Equal(t, kubernetes.ReplicaSetKind, key.Kind)
		assert.Equal(t, "nginx-bcdfg", key.Name)
	})
}

func TestResolveRolloutOwner(t *testing.T) {
	t.Run("replicaset with valid deployment name resolves to deployment", func(t *testing.T) {
		key := ownerKey{Namespace: "default", Kind: kubernetes.ReplicaSetKind, Name: "nginx-bcdfg"}
		result, ok := resolveRolloutOwner(key)
		assert.True(t, ok)
		assert.Equal(t, kubernetes.DeploymentKind, result.Kind)
		assert.Equal(t, "nginx", result.Name)
		assert.Equal(t, "default", result.Namespace)
	})

	t.Run("replicaset with no deployment name returns false", func(t *testing.T) {
		// "nginx" has no hyphen suffix with valid chars → ParseDeploymentForReplicaSet returns ""
		key := ownerKey{Namespace: "default", Kind: kubernetes.ReplicaSetKind, Name: "nginx"}
		_, ok := resolveRolloutOwner(key)
		assert.False(t, ok)
	})

	t.Run("statefulset resolves to itself", func(t *testing.T) {
		key := ownerKey{Namespace: "default", Kind: kubernetes.StatefulSetKind, Name: "redis"}
		result, ok := resolveRolloutOwner(key)
		assert.True(t, ok)
		assert.Equal(t, key, result)
	})

	t.Run("unsupported kind returns false", func(t *testing.T) {
		key := ownerKey{Namespace: "default", Kind: kubernetes.DaemonSetKind, Name: "prometheus"}
		_, ok := resolveRolloutOwner(key)
		assert.False(t, ok)
	})
}
