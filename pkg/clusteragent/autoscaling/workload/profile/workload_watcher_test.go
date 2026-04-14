// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

type fakeNamespaceLister struct {
	namespaces map[string]map[string]string
}

func (f *fakeNamespaceLister) ListNamespaces() map[string]map[string]string {
	return f.namespaces
}

func newTestWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
) *WorkloadWatcher {
	return &WorkloadWatcher{
		profileStore:      profileStore,
		isLeader:          func() bool { return true },
		dynamicClient:     newFakeWorkloadDynamicClient(),
		workloadResources: []GroupVersionKindResource{deploymentGVKR()},
		nsLister:          &fakeNamespaceLister{namespaces: map[string]map[string]string{}},
		nsWatchers:        make(map[string]*nsWorkloadWatcher),
	}
}

func newFakeWorkloadDynamicClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "deployments"}:  "DeploymentList",
			{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
		},
		objs...,
	)
}

func newTestNsWatcher(profileName string, gvkr GroupVersionKindResource, objs ...*unstructured.Unstructured) *nsWorkloadWatcher {
	_, cancel := context.WithCancel(context.Background())
	gr := schema.GroupResource{Group: gvkr.GroupVersionResource.Group, Resource: gvkr.GroupVersionResource.Resource}
	lister := newTestLister(gr, objs...)
	return &nsWorkloadWatcher{
		profileName: profileName,
		cancel:      cancel,
		informers: []workloadInformer{
			{gvkr: gvkr, lister: lister, synced: func() bool { return true }},
		},
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

func TestScanNsWorkloads(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Collects workloads from per-namespace informers", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		dep1 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		dep2 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-2", nil)
		w.nsWatchers["prod"] = newTestNsWatcher("high-cpu", gvkr, dep1, dep2)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		require.Len(t, refs["high-cpu"], 2)
		assert.ElementsMatch(t, []string{"web-1", "web-2"}, []string{refs["high-cpu"][0].Name, refs["high-cpu"][1].Name})
	})

	t.Run("Skips workloads with profile label", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		labeledDep := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-labeled",
			map[string]string{model.ProfileLabelKey: "some-profile"})
		unlabeledDep := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-unlabeled", nil)
		w.nsWatchers["prod"] = newTestNsWatcher("ns-profile", gvkr, labeledDep, unlabeledDep)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		require.Len(t, refs["ns-profile"], 1)
		assert.Equal(t, "web-unlabeled", refs["ns-profile"][0].Name)
	})

	t.Run("Skips workloads with profile-enabled=false label", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		optedOut := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-opted-out",
			map[string]string{model.ProfileEnabledLabelKey: "false"})
		included := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-included", nil)
		w.nsWatchers["prod"] = newTestNsWatcher("ns-profile", gvkr, optedOut, included)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		require.Len(t, refs["ns-profile"], 1)
		assert.Equal(t, "web-included", refs["ns-profile"][0].Name)
	})

	t.Run("Skips unsynced namespace watchers", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		dep := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		nw := newTestNsWatcher("high-cpu", gvkr, dep)
		nw.informers[0].synced = func() bool { return false }
		w.nsWatchers["prod"] = nw

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		assert.Empty(t, refs)
	})
}

func TestSyncNsWatchers(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Creates watcher for new labeled namespace", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")

		w := newTestWorkloadWatcher(profileStore)
		w.nsLister = &fakeNamespaceLister{namespaces: map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "high-cpu"},
		}}

		w.syncNsWatchers()

		require.Contains(t, w.nsWatchers, "prod")
		assert.Equal(t, "high-cpu", w.nsWatchers["prod"].profileName)
		require.Len(t, w.nsWatchers["prod"].informers, 1)
		assert.Equal(t, gvkr, w.nsWatchers["prod"].informers[0].gvkr)
	})

	t.Run("Removes watcher when namespace label is removed", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")

		w := newTestWorkloadWatcher(profileStore)
		w.nsWatchers["prod"] = newTestNsWatcher("high-cpu", gvkr)

		w.syncNsWatchers()

		assert.NotContains(t, w.nsWatchers, "prod")
	})

	t.Run("Updates profile name when namespace label changes", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("old-profile", validTestProfile("old-profile"), "test")
		profileStore.Set("new-profile", validTestProfile("new-profile"), "test")

		w := newTestWorkloadWatcher(profileStore)
		w.nsWatchers["prod"] = newTestNsWatcher("old-profile", gvkr)
		w.nsLister = &fakeNamespaceLister{namespaces: map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "new-profile"},
		}}

		w.syncNsWatchers()

		require.Contains(t, w.nsWatchers, "prod")
		assert.Equal(t, "new-profile", w.nsWatchers["prod"].profileName)
	})

	t.Run("Namespace with unknown profile does not create watcher", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)
		w.nsLister = &fakeNamespaceLister{namespaces: map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "nonexistent"},
		}}

		w.syncNsWatchers()

		assert.Empty(t, w.nsWatchers)
	})
}

func TestWorkloadWatcherNamespaceLevelReconcile(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Namespace label discovers all workloads in namespace", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")

		w := newTestWorkloadWatcher(profileStore)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR)},
		}
		w.nsLister = &fakeNamespaceLister{namespaces: map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "high-cpu"},
		}}

		dep1 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		dep2 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-2", nil)
		w.nsWatchers["prod"] = newTestNsWatcher("high-cpu", gvkr, dep1, dep2)

		w.reconcile()

		pi, ok := profileStore.Get("high-cpu")
		require.True(t, ok)
		require.Len(t, pi.Workloads(), 2)
		var names []string
		for _, ref := range pi.Workloads() {
			names = append(names, ref.Name)
			assert.Equal(t, "prod", ref.Namespace)
			assert.Equal(t, "Deployment", ref.Kind)
		}
		assert.ElementsMatch(t, []string{"web-1", "web-2"}, names)
	})

	t.Run("Workload label takes precedence over namespace label", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("ns-profile", validTestProfile("ns-profile"), "test")
		profileStore.Set("wl-profile", validTestProfile("wl-profile"), "test")

		labeledDep := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-labeled",
			map[string]string{model.ProfileLabelKey: "wl-profile"})
		unlabeledDep := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-unlabeled", nil)

		w := newTestWorkloadWatcher(profileStore)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, labeledDep)},
		}
		w.nsLister = &fakeNamespaceLister{namespaces: map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "ns-profile"},
		}}
		w.nsWatchers["prod"] = newTestNsWatcher("ns-profile", gvkr, labeledDep, unlabeledDep)

		w.reconcile()

		wlPI, ok := profileStore.Get("wl-profile")
		require.True(t, ok)
		require.Len(t, wlPI.Workloads(), 1)
		for _, ref := range wlPI.Workloads() {
			assert.Equal(t, "web-labeled", ref.Name)
		}

		nsPI, ok := profileStore.Get("ns-profile")
		require.True(t, ok)
		require.Len(t, nsPI.Workloads(), 1)
		for _, ref := range nsPI.Workloads() {
			assert.Equal(t, "web-unlabeled", ref.Name)
		}
	})

	t.Run("Multiple namespaces with different profiles", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("prof-a", validTestProfile("prof-a"), "test")
		profileStore.Set("prof-b", validTestProfile("prof-b"), "test")

		depA := newUnstructuredWorkload("Deployment", "apps/v1", "ns-a", "app-a", nil)
		depB := newUnstructuredWorkload("Deployment", "apps/v1", "ns-b", "app-b", nil)

		w := newTestWorkloadWatcher(profileStore)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR)},
		}
		w.nsLister = &fakeNamespaceLister{namespaces: map[string]map[string]string{
			"ns-a": {model.ProfileLabelKey: "prof-a"},
			"ns-b": {model.ProfileLabelKey: "prof-b"},
		}}
		w.nsWatchers["ns-a"] = newTestNsWatcher("prof-a", gvkr, depA)
		w.nsWatchers["ns-b"] = newTestNsWatcher("prof-b", gvkr, depB)

		w.reconcile()

		piA, ok := profileStore.Get("prof-a")
		require.True(t, ok)
		require.Len(t, piA.Workloads(), 1)
		for _, ref := range piA.Workloads() {
			assert.Equal(t, "app-a", ref.Name)
			assert.Equal(t, "ns-a", ref.Namespace)
		}

		piB, ok := profileStore.Get("prof-b")
		require.True(t, ok)
		require.Len(t, piB.Workloads(), 1)
		for _, ref := range piB.Workloads() {
			assert.Equal(t, "app-b", ref.Name)
			assert.Equal(t, "ns-b", ref.Namespace)
		}
	})

	t.Run("Namespace label removed clears workload refs", func(t *testing.T) {
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
		assert.Empty(t, pi.Workloads())
	})
}

func newTestLister(gr schema.GroupResource, objs ...*unstructured.Unstructured) cache.GenericLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, obj := range objs {
		_ = indexer.Add(obj)
	}
	return cache.NewGenericLister(indexer, gr)
}
