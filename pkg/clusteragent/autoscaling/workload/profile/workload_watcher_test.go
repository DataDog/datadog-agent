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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata/metadatalister"
	"k8s.io/client-go/tools/cache"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

var namespaceGR = schema.GroupResource{Group: "", Resource: "namespaces"}

func newTestNSLister(namespaces map[string]map[string]string) cache.GenericLister {
	objs := make([]*metav1.PartialObjectMetadata, 0, len(namespaces))
	for name, lbls := range namespaces {
		objs = append(objs, &metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls},
		})
	}
	return newTestLister(namespaceGR, objs...)
}

func newTestWorkloadWatcher(
	profileStore *autoscaling.Store[model.PodAutoscalerProfileInternal],
) *WorkloadWatcher {
	return &WorkloadWatcher{
		profileStore: profileStore,
		isLeader:     func() bool { return true },
		nsLister:     newTestNSLister(nil),
	}
}

func validTestProfile(name string) model.PodAutoscalerProfileInternal {
	pi, _ := model.NewPodAutoscalerProfileInternal(&datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1},
		Spec:       datadoghq.DatadogPodAutoscalerProfileSpec{Template: validTemplate()},
	})
	return pi
}

func newPartialWorkload(kind, apiVersion, namespace, name string, lbls map[string]string) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{Kind: kind, APIVersion: apiVersion},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    lbls,
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
	noLabeledNs := map[string]string{}

	t.Run("Labelled Deployment with existing profile", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		deployment := newPartialWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, deployment), noLabeledNs)

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

		deployment := newPartialWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "nonexistent"})
		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, deployment), noLabeledNs)

		assert.Empty(t, refs)
	})

	t.Run("No label - skipped", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)

		deployment := newPartialWorkload("Deployment", "apps/v1", "prod", "web-app", nil)
		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, deployment), noLabeledNs)

		assert.Empty(t, refs)
	})

	t.Run("Multiple workloads one profile", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		dep1 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-1",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		dep2 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-2",
			map[string]string{model.ProfileLabelKey: "high-cpu"})

		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, dep1, dep2), noLabeledNs)

		require.Len(t, refs["high-cpu"], 2)
	})

	t.Run("Multiple profiles", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("foo", validTestProfile("foo"), "test")
		profileStore.Set("bar", validTestProfile("bar"), "test")
		w := newTestWorkloadWatcher(profileStore)

		dep1 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-1",
			map[string]string{model.ProfileLabelKey: "foo"})
		dep2 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-2",
			map[string]string{model.ProfileLabelKey: "bar"})

		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, dep1, dep2), noLabeledNs)

		require.Len(t, refs["foo"], 1)
		require.Len(t, refs["bar"], 1)
	})

	t.Run("Namespace-level: discovers workloads in labeled namespace", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		dep1 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		dep2 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-2", nil)

		labeledNs := map[string]string{"prod": "high-cpu"}
		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, dep1, dep2), labeledNs)

		require.Len(t, refs["high-cpu"], 2)
	})

	t.Run("Namespace-level: skips workloads with profile label (precedence)", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("ns-profile", validTestProfile("ns-profile"), "test")
		profileStore.Set("wl-profile", validTestProfile("wl-profile"), "test")
		w := newTestWorkloadWatcher(profileStore)

		labeled := newPartialWorkload("Deployment", "apps/v1", "prod", "web-labeled",
			map[string]string{model.ProfileLabelKey: "wl-profile"})
		unlabeled := newPartialWorkload("Deployment", "apps/v1", "prod", "web-unlabeled", nil)

		labeledNs := map[string]string{"prod": "ns-profile"}
		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, labeled, unlabeled), labeledNs)

		require.Len(t, refs["wl-profile"], 1)
		assert.Equal(t, "web-labeled", refs["wl-profile"][0].Name)
		require.Len(t, refs["ns-profile"], 1)
		assert.Equal(t, "web-unlabeled", refs["ns-profile"][0].Name)
	})

	t.Run("Namespace-level: skips workloads with profile=excluded label", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("ns-profile", validTestProfile("ns-profile"), "test")
		w := newTestWorkloadWatcher(profileStore)

		optedOut := newPartialWorkload("Deployment", "apps/v1", "prod", "web-opted-out",
			map[string]string{model.ProfileLabelKey: model.ProfileExcludedValue})
		included := newPartialWorkload("Deployment", "apps/v1", "prod", "web-included", nil)

		labeledNs := map[string]string{"prod": "ns-profile"}
		refs := w.scanWorkloads(gvkr, newTestLister(deploymentGR, optedOut, included), labeledNs)

		require.Len(t, refs["ns-profile"], 1)
		assert.Equal(t, "web-included", refs["ns-profile"][0].Name)
	})

	t.Run("Namespace-level: namespace with unknown profile is filtered by buildLabeledNamespaces", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		w := newTestWorkloadWatcher(profileStore)
		w.nsLister = newTestNSLister(map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "nonexistent"},
		})

		labeledNs := w.buildLabeledNamespaces()
		assert.Empty(t, labeledNs)
	})
}

func TestWorkloadWatcherReconcile(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Sets workload refs on profile", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")
		w := newTestWorkloadWatcher(profileStore)

		deployment := newPartialWorkload("Deployment", "apps/v1", "prod", "web-app",
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
		deployment := newPartialWorkload("Deployment", "apps/v1", "prod", "web-app",
			map[string]string{model.ProfileLabelKey: "high-cpu"})
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, deployment)},
		}

		w.reconcile()

		assert.False(t, updated, "Should not trigger store update when refs unchanged")
	})
}

func TestWorkloadWatcherNamespaceLevelReconcile(t *testing.T) {
	gvkr := deploymentGVKR()

	t.Run("Namespace label discovers all workloads in namespace", func(t *testing.T) {
		profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
		profileStore.Set("high-cpu", validTestProfile("high-cpu"), "test")

		dep1 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-1", nil)
		dep2 := newPartialWorkload("Deployment", "apps/v1", "prod", "web-2", nil)

		w := newTestWorkloadWatcher(profileStore)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, dep1, dep2)},
		}
		w.nsLister = newTestNSLister(map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "high-cpu"},
		})

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

		labeledDep := newPartialWorkload("Deployment", "apps/v1", "prod", "web-labeled",
			map[string]string{model.ProfileLabelKey: "wl-profile"})
		unlabeledDep := newPartialWorkload("Deployment", "apps/v1", "prod", "web-unlabeled", nil)

		w := newTestWorkloadWatcher(profileStore)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, labeledDep, unlabeledDep)},
		}
		w.nsLister = newTestNSLister(map[string]map[string]string{
			"prod": {model.ProfileLabelKey: "ns-profile"},
		})

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

		depA := newPartialWorkload("Deployment", "apps/v1", "ns-a", "app-a", nil)
		depB := newPartialWorkload("Deployment", "apps/v1", "ns-b", "app-b", nil)

		w := newTestWorkloadWatcher(profileStore)
		w.informers = []workloadInformer{
			{gvkr: gvkr, lister: newTestLister(deploymentGR, depA, depB)},
		}
		w.nsLister = newTestNSLister(map[string]map[string]string{
			"ns-a": {model.ProfileLabelKey: "prof-a"},
			"ns-b": {model.ProfileLabelKey: "prof-b"},
		})

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

func newTestLister(gr schema.GroupResource, objs ...*metav1.PartialObjectMetadata) cache.GenericLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, obj := range objs {
		_ = indexer.Add(obj)
	}
	gvr := schema.GroupVersionResource{Group: gr.Group, Resource: gr.Resource}
	return metadatalister.NewRuntimeObjectShim(metadatalister.New(indexer, gvr))
}
