// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	instrumentationhandlers "github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation/handlers"
)

func podWithOwners(ns string, owners ...metav1.OwnerReference) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, OwnerReferences: owners},
	}
}

func owner(kind, name string) metav1.OwnerReference {
	return metav1.OwnerReference{Kind: kind, Name: name}
}

func TestWorkloadTargetsFromPod(t *testing.T) {
	// PyTorchJob-owned pod: emits the direct controller target + the namespace fallback.
	pt := podWithOwners("train", owner("PyTorchJob", "gpt-job"))
	got := workloadTargetsFromPod(pt)
	assert.Equal(t, []instrumentationhandlers.NCCLProfilerTarget{
		{Kind: "PyTorchJob", Namespace: "train", Name: "gpt-job"},
		{Kind: instrumentationhandlers.NamespaceKind, Namespace: "train", Name: "train"},
	}, got)

	// No owners: only the namespace fallback.
	orphan := podWithOwners("ns1")
	assert.Equal(t, []instrumentationhandlers.NCCLProfilerTarget{{Kind: instrumentationhandlers.NamespaceKind, Namespace: "ns1", Name: "ns1"}}, workloadTargetsFromPod(orphan))
}

func TestDDIConfig_MatchAndGating(t *testing.T) {
	store := instrumentationhandlers.NewNCCLProfilerStore()
	store.Upsert(
		instrumentationhandlers.NCCLProfilerTarget{Kind: "PyTorchJob", Namespace: "train", Name: "gpt-job"},
		instrumentationhandlers.NCCLProfilerConfig{CR: types.NamespacedName{Namespace: "train", Name: "cr"}, Enabled: true, InjectorImage: "img:1"},
	)
	pod := podWithOwners("train", owner("PyTorchJob", "gpt-job"))

	// DDI off → never matches, even with a populated store.
	wOff := &Webhook{ddi: false, store: store}
	_, ok := wOff.ddiConfig(pod)
	assert.False(t, ok, "ddiConfig must be a no-op when ddi is off")

	// DDI on → matches the enabled target.
	wOn := &Webhook{ddi: true, store: store}
	cfg, ok := wOn.ddiConfig(pod)
	assert.True(t, ok)
	assert.Equal(t, "img:1", cfg.InjectorImage)

	// nil store → no panic, no match.
	wNil := &Webhook{ddi: true, store: nil}
	_, ok = wNil.ddiConfig(pod)
	assert.False(t, ok)

	// Disabled target → no match.
	store.Upsert(
		instrumentationhandlers.NCCLProfilerTarget{Kind: "PyTorchJob", Namespace: "train", Name: "gpt-job"},
		instrumentationhandlers.NCCLProfilerConfig{CR: types.NamespacedName{Namespace: "train", Name: "cr"}, Enabled: false},
	)
	_, ok = wOn.ddiConfig(pod)
	assert.False(t, ok, "a disabled target must not match")
}

func TestEnabledLabelValue(t *testing.T) {
	val, exists := enabledLabelValue(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{EnabledLabel: "true"}}})
	assert.True(t, exists)
	assert.True(t, val)

	val, exists = enabledLabelValue(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{EnabledLabel: "false"}}})
	assert.True(t, exists)
	assert.False(t, val)

	_, exists = enabledLabelValue(&corev1.Pod{})
	assert.False(t, exists)
}
