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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func newTestWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
	objects ...runtime.Object,
) *WorkloadWatcher {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	nsGR := schema.GroupResource{Resource: "namespaces"}
	return &WorkloadWatcher{
		profileStore:      profileStore,
		isLeader:          func() bool { return true },
		dynamicClient:     fakeClient,
		workloadResources: []GroupVersionKindResource{deploymentGVKR()},
		nsLister:          newTestLister(nsGR),
	}
}

func newUnstructuredNamespace(name string, lbls map[string]string) *unstructured.Unstructured {
	metadata := map[string]any{"name": name}
	if len(lbls) > 0 {
		labelsMap := make(map[string]any, len(lbls))
		for k, v := range lbls {
			labelsMap[k] = v
		}
		metadata["labels"] = labelsMap
	}
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   metadata,
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

func newTestLister(gr schema.GroupResource, objs ...*unstructured.Unstructured) cache.GenericLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, obj := range objs {
		_ = indexer.Add(obj)
	}
	return cache.NewGenericLister(indexer, gr)
}

func TestScanNsWorkloads(t *testing.T) {
	gvkr := deploymentGVKR()
	nsGR := schema.GroupResource{Resource: "namespaces"}

	t.Run("Discovers workloads in labeled namespace", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")

		dep1 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		dep2 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-2", nil)

		w := newTestWorkloadWatcher(profileStore, dep1, dep2)
		w.workloadResources = []GroupVersionKindResource{gvkr}

		ns := newUnstructuredNamespace("prod", map[string]string{model.ProfileLabelKey: "high-cpu"})
		w.nsLister = newTestLister(nsGR, ns)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		require.Len(t, refs["high-cpu"], 2)
		var names []string
		for _, ref := range refs["high-cpu"] {
			names = append(names, ref.Name)
			assert.Equal(t, "prod", ref.Namespace)
			assert.Equal(t, "Deployment", ref.Kind)
		}
		assert.ElementsMatch(t, []string{"web-1", "web-2"}, names)
	})

	t.Run("Skips workloads with profile label (precedence)", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("ns-profile", validTestProfile("ns-profile"), "test")

		labeled := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-labeled",
			map[string]string{model.ProfileLabelKey: "other-profile"})
		unlabeled := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-unlabeled", nil)

		w := newTestWorkloadWatcher(profileStore, labeled, unlabeled)
		w.workloadResources = []GroupVersionKindResource{gvkr}

		ns := newUnstructuredNamespace("prod", map[string]string{model.ProfileLabelKey: "ns-profile"})
		w.nsLister = newTestLister(nsGR, ns)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		require.Len(t, refs["ns-profile"], 1)
		assert.Equal(t, "web-unlabeled", refs["ns-profile"][0].Name)
	})

	t.Run("Skips workloads with profile-disabled label", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("ns-profile", validTestProfile("ns-profile"), "test")

		optedOut := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-opted-out",
			map[string]string{model.ProfileDisabledLabelKey: "true"})
		included := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-included", nil)

		w := newTestWorkloadWatcher(profileStore, optedOut, included)
		w.workloadResources = []GroupVersionKindResource{gvkr}

		ns := newUnstructuredNamespace("prod", map[string]string{model.ProfileLabelKey: "ns-profile"})
		w.nsLister = newTestLister(nsGR, ns)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		require.Len(t, refs["ns-profile"], 1)
		assert.Equal(t, "web-included", refs["ns-profile"][0].Name)
	})

	t.Run("Namespace with unknown profile is skipped", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		ns := newUnstructuredNamespace("prod", map[string]string{model.ProfileLabelKey: "nonexistent"})
		w.nsLister = newTestLister(nsGR, ns)

		refs := make(map[string][]model.NamespacedObjectReference)
		w.scanNsWorkloads(refs)

		assert.Empty(t, refs)
	})
}

func TestWorkloadWatcherNamespaceLevelReconcile(t *testing.T) {
	gvkr := deploymentGVKR()
	nsGR := schema.GroupResource{Resource: "namespaces"}

	t.Run("Namespace label discovers all workloads in namespace", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")

		dep1 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		dep2 := newUnstructuredWorkload("Deployment", "apps/v1", "prod", "web-2", nil)

		w := newTestWorkloadWatcher(profileStore, dep1, dep2)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR)},
		}

		ns := newUnstructuredNamespace("prod", map[string]string{model.ProfileLabelKey: "high-cpu"})
		w.nsLister = newTestLister(nsGR, ns)

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

		w := newTestWorkloadWatcher(profileStore, labeledDep, unlabeledDep)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, labeledDep)},
		}

		ns := newUnstructuredNamespace("prod", map[string]string{model.ProfileLabelKey: "ns-profile"})
		w.nsLister = newTestLister(nsGR, ns)

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

		w := newTestWorkloadWatcher(profileStore, depA, depB)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR)},
		}

		nsA := newUnstructuredNamespace("ns-a", map[string]string{model.ProfileLabelKey: "prof-a"})
		nsB := newUnstructuredNamespace("ns-b", map[string]string{model.ProfileLabelKey: "prof-b"})
		w.nsLister = newTestLister(nsGR, nsA, nsB)

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
