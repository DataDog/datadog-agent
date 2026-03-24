// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func newTestWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
) *WorkloadWatcher {
	return &WorkloadWatcher{
		profileStore: profileStore,
		isLeader:     func() bool { return true },
	}
}

func validTestProfile(name string) model.PodAutoscalerProfileInternal {
	pi, _ := model.NewPodAutoscalerProfileInternal(&datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1},
		Spec:       datadoghq.DatadogPodAutoscalerProfileSpec{Template: validTemplate()},
	})
	return pi
}

func newUnstructuredWorkload(kind, apiVersion, namespace, name string, lbls map[string]string) *unstructured.Unstructured {
	metadata := map[string]any{
		"namespace": namespace,
		"name":      name,
	}
	if len(lbls) > 0 {
		labels := make(map[string]any, len(lbls))
		for k, v := range lbls {
			labels[k] = v
		}
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   metadata,
		},
	}
}

func deploymentGVKR() GroupVersionKindResource {
	return GroupVersionKindResource{
		GroupVersionResource: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		Kind:                 "Deployment",
	}
}

var deploymentGR = schema.GroupResource{Group: "apps", Resource: "deployments"}

func TestWorkloadWatcherScanWorkloads(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Labelled Deployment with existing profile", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		deployment := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanWorkloads(gvkr, newTestLister(deploymentGR, deployment), refs)

		require.Len(t, refs["high-cpu"], 1)
		ref := refs["high-cpu"][0]
		assert.Equal(t, "prod", ref.Namespace)
		assert.Equal(t, "web-app", ref.Name)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, "v1", ref.Version)
		assert.Equal(t, "apps", ref.Group)
	})

	t.Run("Profile not found - skipped", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		deployment := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "nonexistent"})
		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanWorkloads(gvkr, newTestLister(deploymentGR, deployment), refs)

		assert.Empty(t, refs)
	})

	t.Run("No label - skipped", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		deployment := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-app", nil)
		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanWorkloads(gvkr, newTestLister(deploymentGR, deployment), refs)

		assert.Empty(t, refs)
	})

	t.Run("Multiple workloads one profile", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		dep1 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		dep2 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-2",
			map[string]string{model.ProfileLabelKey: "high-cpu"})

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanWorkloads(gvkr, newTestLister(deploymentGR, dep1, dep2), refs)

		require.Len(t, refs["high-cpu"], 2)
	})

	t.Run("Multiple profiles", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("foo", validTestProfile("foo"), "test")
		profileStore.Set("bar", validTestProfile("bar"), "test")
		w := newTestWorkloadWatcher(profileStore)

		dep1 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1",
			map[string]string{model.ProfileLabelKey: "foo"})
		dep2 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-2",
			map[string]string{model.ProfileLabelKey: "bar"})

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanWorkloads(gvkr, newTestLister(deploymentGR, dep1, dep2), refs)

		require.Len(t, refs["foo"], 1)
		require.Len(t, refs["bar"], 1)
	})
}

func TestWorkloadWatcherReconcile(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Sets workload refs on profile", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		deployment := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, deployment)},
		}

		w.reconcile()

		pi, ok := profileStore.Get("high-cpu")
		require.True(t, ok)
		require.Len(t, pi.Workloads(), 1)

		for _, ref := range pi.Workloads() {
			assert.Equal(t, "web-app", ref.Name)
			assert.Equal(t, "prod", ref.Namespace)
		}
	})

	t.Run("Clears workload refs when label removed", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		prof := validTestProfile("high-cpu")
		prof.UpdateWorkloads([]model.NamespacedObjectReference{testRef("prod", "web-app")})
		profileStore.Set("high-cpu", prof, "test")
		w := newTestWorkloadWatcher(profileStore)

		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR)},
		}

		w.reconcile()

		pi, ok := profileStore.Get("high-cpu")
		require.True(t, ok)
		assert.Empty(t, pi.Workloads(), "Workload refs should be cleared")
	})

	t.Run("No change when refs unchanged", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		prof := validTestProfile("high-cpu")
		prof.UpdateWorkloads([]model.NamespacedObjectReference{testRef("prod", "web-app")})
		profileStore.Set("high-cpu", prof, "test")

		updated := false
		profileStore.RegisterObserver(autoscaling.Observer{
			SetFunc: func(_ string, sender autoscaling.SenderID) {
				if sender == workloadWatcherStoreID {
					updated = true
				}
			},
		})

		w := newTestWorkloadWatcher(profileStore)
		deployment := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, deployment)},
		}

		w.reconcile()

		assert.False(t, updated, "Should not trigger store update when refs unchanged")
	})
}

func newTestLister(gr schema.GroupResource, objs ...*unstructured.Unstructured) cache.GenericLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, obj := range objs {
		_ = indexer.Add(obj)
	}
	return cache.NewGenericLister(indexer, gr)
}
