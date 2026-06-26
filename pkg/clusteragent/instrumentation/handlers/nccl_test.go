// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"context"
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

// ncclCR builds a DatadogInstrumentation CR; nccl=nil means no ncclProfiler section.
func ncclCR(name, namespace, targetKind, targetName string, nccl *datadoghq.DatadogInstrumentationNCCLConfig) *datadoghq.DatadogInstrumentation {
	return &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{Kind: targetKind, Name: targetName},
			Config:    datadoghq.DatadogInstrumentationConfig{NCCLProfiler: nccl},
		},
	}
}

func newNCCLHandler() (*NCCLHandler, *NCCLProfilerStore) {
	s := NewNCCLProfilerStore()
	return &NCCLHandler{store: s}, s
}

func TestNCCLHandler_HasSection(t *testing.T) {
	h, _ := newNCCLHandler()
	assert.False(t, h.HasSection(ncclCR("c", "ns", "PyTorchJob", "j", nil)))
	assert.True(t, h.HasSection(ncclCR("c", "ns", "PyTorchJob", "j", &datadoghq.DatadogInstrumentationNCCLConfig{Enabled: true})))
}

func TestNCCLHandler_SupportsTarget(t *testing.T) {
	h, _ := newNCCLHandler()
	for _, kind := range []string{"StatefulSet", "DaemonSet", "Job", "RayCluster", "PyTorchJob", "Namespace"} {
		assert.Truef(t, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: kind}), "kind %s should be supported", kind)
	}
	// Indirectly-owned kinds are not matchable by the webhook → not supported.
	for _, kind := range []string{"Deployment", "CronJob", "Pod", "Service"} {
		assert.Falsef(t, h.SupportsTarget(autoscalingv2.CrossVersionObjectReference{Kind: kind}), "kind %s should NOT be supported", kind)
	}
}

func TestNCCLHandler_HandleUpsertAndDelete(t *testing.T) {
	h, store := newNCCLHandler()
	cr := ncclCR("cr1", "train", "PyTorchJob", "gpt-job", &datadoghq.DatadogInstrumentationNCCLConfig{
		Enabled:       true,
		InjectorImage: "img:1",
		Env:           []corev1.EnvVar{{Name: "NCCL_DEBUG", Value: "INFO"}},
	})

	st, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)
	assert.Equal(t, metav1.ConditionTrue, st.Status)

	target := NCCLProfilerTarget{Kind: "PyTorchJob", Namespace: "train", Name: "gpt-job"}
	cfg, ok := store.Get(target)
	require.True(t, ok)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "img:1", cfg.InjectorImage)
	assert.Equal(t, []corev1.EnvVar{{Name: "NCCL_DEBUG", Value: "INFO"}}, cfg.Env)

	_, err = h.Handle(context.Background(), instrumentation.EventDelete, cr)
	require.NoError(t, err)
	_, ok = store.Get(target)
	assert.False(t, ok, "DeleteByCR should remove the target")
}

func TestNCCLHandler_NamespaceTargetNormalized(t *testing.T) {
	h, store := newNCCLHandler()
	// targetRef.Name deliberately differs from the namespace; Handle normalizes the
	// store key to the CR's own namespace so the webhook ns-fallback matches.
	cr := ncclCR("cr1", "train", "Namespace", "ignored-name", &datadoghq.DatadogInstrumentationNCCLConfig{Enabled: true})
	_, err := h.Handle(context.Background(), instrumentation.EventCreate, cr)
	require.NoError(t, err)

	_, ok := store.Get(NCCLProfilerTarget{Kind: NamespaceKind, Namespace: "train", Name: "train"})
	assert.True(t, ok, "namespace target must be keyed by the CR namespace")
}

func TestNCCLHandler_Validate(t *testing.T) {
	h, _ := newNCCLHandler()

	// Empty env name.
	errs := h.Validate(ncclCR("cr", "ns", "Job", "j", &datadoghq.DatadogInstrumentationNCCLConfig{
		Enabled: true,
		Env:     []corev1.EnvVar{{Name: "", Value: "x"}},
	}))
	require.Len(t, errs, 1)
	assert.Equal(t, reasonNCCLInvalidEnv, errs[0].Reason)

	// Valid.
	assert.Empty(t, h.Validate(ncclCR("cr", "ns", "Job", "j", &datadoghq.DatadogInstrumentationNCCLConfig{
		Enabled: true,
		Env:     []corev1.EnvVar{{Name: "NCCL_DEBUG", Value: "INFO"}},
	})))

	// No section.
	assert.Empty(t, h.Validate(ncclCR("cr", "ns", "Job", "j", nil)))
}

func TestNCCLProfilerStore_DeleteByCRGuard(t *testing.T) {
	store := NewNCCLProfilerStore()
	target := NCCLProfilerTarget{Kind: "Job", Namespace: "ns", Name: "j"}
	crA := types.NamespacedName{Namespace: "ns", Name: "a"}
	crB := types.NamespacedName{Namespace: "ns", Name: "b"}

	store.Upsert(target, NCCLProfilerConfig{CR: crA, Enabled: true})
	// A different CR overwrites the same target.
	store.Upsert(target, NCCLProfilerConfig{CR: crB, Enabled: true})

	// Deleting the stale CR (A) must NOT remove the entry now owned by B.
	store.DeleteByCR(crA)
	_, ok := store.Get(target)
	assert.True(t, ok, "delete of a stale CR must not evict the current owner's entry")

	store.DeleteByCR(crB)
	_, ok = store.Get(target)
	assert.False(t, ok)
}
