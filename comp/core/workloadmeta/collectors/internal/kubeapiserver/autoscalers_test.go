// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// notifyRecorder captures the events the index pushes into workloadmeta. Only
// Notify is exercised, so the embedded interface is never dereferenced.
type notifyRecorder struct {
	workloadmeta.Component

	mu     sync.Mutex
	events []workloadmeta.CollectorEvent
}

func (n *notifyRecorder) Notify(events []workloadmeta.CollectorEvent) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, events...)
}

// drain returns the events recorded since the last call.
func (n *notifyRecorder) drain() []workloadmeta.CollectorEvent {
	n.mu.Lock()
	defer n.mu.Unlock()
	events := n.events
	n.events = nil
	return events
}

// deploymentKinds reduces recorded events to the autoscaler kinds set on each
// deployment entity, so assertions read as "what would the tagger see".
func deploymentKinds(events []workloadmeta.CollectorEvent) map[string][]string {
	out := map[string][]string{}
	for _, event := range events {
		deployment, ok := event.Entity.(*workloadmeta.KubernetesDeployment)
		if !ok {
			continue
		}
		if event.Type == workloadmeta.EventTypeUnset {
			out[deployment.ID] = nil
			continue
		}
		out[deployment.ID] = sets.List(deployment.AutoscalerKinds)
	}
	return out
}

func newTestIndex() (*autoscalerIndex, *notifyRecorder) {
	recorder := &notifyRecorder{}
	return newAutoscalerIndex(recorder), recorder
}

type autoscalerOpts struct {
	ownerKind string
	labels    map[string]string
}

// newAutoscalerObj builds an unstructured autoscaler pointing at targetKind/targetName.
func newAutoscalerObj(namespace, name, targetRefField, targetKind, targetName string, opts autoscalerOpts) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
		"spec": map[string]interface{}{
			targetRefField: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       targetKind,
				"name":       targetName,
			},
		},
	}}
	if opts.ownerKind != "" {
		u.SetOwnerReferences([]metav1.OwnerReference{{Kind: opts.ownerKind, Name: "owner", APIVersion: "keda.sh/v1alpha1"}})
	}
	if opts.labels != nil {
		u.SetLabels(opts.labels)
	}
	return u
}

func newTestStore(index *autoscalerIndex, spec autoscalerSpec, resource string) *autoscalerStore {
	return &autoscalerStore{
		index: index,
		spec:  spec,
		gvr:   schema.GroupVersionResource{Resource: resource},
	}
}

func hpaSpec() autoscalerSpec {
	return autoscalerSpec{kind: kubernetes.AutoscalerKindHPA, targetRefPath: []string{"spec", "scaleTargetRef"}, kedaAware: true}
}

func vpaSpec() autoscalerSpec {
	return autoscalerSpec{kind: kubernetes.AutoscalerKindVPA, targetRefPath: []string{"spec", "targetRef"}}
}

func dpaSpec() autoscalerSpec {
	return autoscalerSpec{kind: kubernetes.AutoscalerKindDPA, targetRefPath: []string{"spec", "targetRef"}}
}

func TestAutoscalerStoreParse(t *testing.T) {
	tests := []struct {
		name         string
		spec         autoscalerSpec
		obj          *unstructured.Unstructured
		expectOK     bool
		expectKind   string
		expectTarget kubernetes.WorkloadTarget
	}{
		{
			name:         "hpa reads scaleTargetRef",
			spec:         hpaSpec(),
			obj:          newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{}),
			expectOK:     true,
			expectKind:   kubernetes.AutoscalerKindHPA,
			expectTarget: kubernetes.WorkloadTarget{Kind: "Deployment", Namespace: "ns", Name: "my-app"},
		},
		{
			name:         "vpa reads targetRef",
			spec:         vpaSpec(),
			obj:          newAutoscalerObj("ns", "my-vpa", "targetRef", "Deployment", "my-app", autoscalerOpts{}),
			expectOK:     true,
			expectKind:   kubernetes.AutoscalerKindVPA,
			expectTarget: kubernetes.WorkloadTarget{Kind: "Deployment", Namespace: "ns", Name: "my-app"},
		},
		{
			name:       "keda detected through owner reference",
			spec:       hpaSpec(),
			obj:        newAutoscalerObj("ns", "keda-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{ownerKind: kubernetes.KedaScaledObjectKind}),
			expectOK:   true,
			expectKind: kubernetes.AutoscalerKindKeda,
		},
		{
			name: "keda detected through managed-by label",
			spec: hpaSpec(),
			obj: newAutoscalerObj("ns", "keda-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{
				labels: map[string]string{kubernetes.ManagedByLabelKey: kubernetes.KedaManagedByLabelValue},
			}),
			expectOK:   true,
			expectKind: kubernetes.AutoscalerKindKeda,
		},
		{
			name: "unrelated managed-by label is not keda",
			spec: hpaSpec(),
			obj: newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{
				labels: map[string]string{kubernetes.ManagedByLabelKey: "helm"},
			}),
			expectOK:   true,
			expectKind: kubernetes.AutoscalerKindHPA,
		},
		{
			// A DPA is never attributed to KEDA: KEDA does not generate them.
			name:       "dpa owned by a scaledobject stays a dpa",
			spec:       dpaSpec(),
			obj:        newAutoscalerObj("ns", "my-dpa", "targetRef", "Deployment", "my-app", autoscalerOpts{ownerKind: kubernetes.KedaScaledObjectKind}),
			expectOK:   true,
			expectKind: kubernetes.AutoscalerKindDPA,
		},
		{
			name:     "missing target ref is not indexed",
			spec:     hpaSpec(),
			obj:      &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{"namespace": "ns", "name": "broken"}}},
			expectOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index, _ := newTestIndex()
			store := newTestStore(index, test.spec, "someresource")

			_, entry, ok := store.parse(test.obj)
			require.Equal(t, test.expectOK, ok)
			if !test.expectOK {
				return
			}
			assert.Equal(t, test.expectKind, entry.kind)
			if test.expectTarget != (kubernetes.WorkloadTarget{}) {
				assert.Equal(t, test.expectTarget, entry.target)
			}
		})
	}
}

func TestAutoscalerIndexLifecycle(t *testing.T) {
	index, recorder := newTestIndex()
	hpaStore := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)
	vpaStore := newTestStore(index, vpaSpec(), kubernetes.VerticalPodAutoscalerResourceName)

	// An HPA appears.
	require.NoError(t, hpaStore.Add(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})))
	assert.Equal(t, map[string][]string{"ns/my-app": {"hpa"}}, deploymentKinds(recorder.drain()))

	// A VPA on the same workload adds a second value rather than replacing.
	require.NoError(t, vpaStore.Add(newAutoscalerObj("ns", "my-vpa", "targetRef", "Deployment", "my-app", autoscalerOpts{})))
	assert.Equal(t, map[string][]string{"ns/my-app": {"hpa", "vpa"}}, deploymentKinds(recorder.drain()))

	// Re-adding the same HPA is a no-op: a resync must not churn the tagger.
	require.NoError(t, hpaStore.Add(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})))
	assert.Empty(t, recorder.drain())

	// Retargeting the HPA removes it from the old workload and adds it to the new one.
	require.NoError(t, hpaStore.Update(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "other-app", autoscalerOpts{})))
	assert.Equal(t, map[string][]string{
		"ns/my-app":    {"vpa"},
		"ns/other-app": {"hpa"},
	}, deploymentKinds(recorder.drain()))

	// Deleting the last autoscaler clears the kinds with a Set carrying an empty
	// set, then an Unset. The order is load-bearing: a bare Unset leaves
	// subscribers with the pre-delete state (see
	// TestAutoscalerTagsClearThroughRealStore for why).
	require.NoError(t, vpaStore.Delete(newAutoscalerObj("ns", "my-vpa", "targetRef", "Deployment", "my-app", autoscalerOpts{})))
	events := recorder.drain()
	require.Len(t, events, 2)
	assert.Equal(t, workloadmeta.EventTypeSet, events[0].Type)
	assert.Empty(t, sets.List(events[0].Entity.(*workloadmeta.KubernetesDeployment).AutoscalerKinds))
	assert.Equal(t, workloadmeta.EventTypeUnset, events[1].Type)
	for _, event := range events {
		assert.Equal(t, "ns/my-app", event.Entity.GetID().ID)
		assert.Equal(t, workloadmeta.SourceKubeAutoscalers, event.Source)
	}
}

func TestAutoscalerIndexIgnoresUnsupportedTargets(t *testing.T) {
	index, recorder := newTestIndex()
	store := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)

	// StatefulSets and Rollouts have no workloadmeta entity to carry the tag
	// yet, so nothing is emitted for them.
	require.NoError(t, store.Add(newAutoscalerObj("ns", "sts-hpa", "scaleTargetRef", "StatefulSet", "my-sts", autoscalerOpts{})))
	require.NoError(t, store.Add(newAutoscalerObj("ns", "ro-hpa", "scaleTargetRef", "Rollout", "my-rollout", autoscalerOpts{})))
	assert.Empty(t, recorder.drain())

	// They are still tracked in the index, so adding their entities later is
	// purely additive.
	assert.Len(t, index.byAutoscaler, 2)
}

func TestAutoscalerStoreReplace(t *testing.T) {
	index, recorder := newTestIndex()
	hpaStore := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)
	vpaStore := newTestStore(index, vpaSpec(), kubernetes.VerticalPodAutoscalerResourceName)

	require.NoError(t, hpaStore.Add(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})))
	require.NoError(t, vpaStore.Add(newAutoscalerObj("ns", "my-vpa", "targetRef", "Deployment", "my-app", autoscalerOpts{})))
	recorder.drain()

	// A relist of HPAs that no longer contains my-hpa must drop it, but must
	// leave the VPA entry alone: each reflector only owns its own resource.
	require.NoError(t, hpaStore.Replace(nil, ""))
	assert.Equal(t, map[string][]string{"ns/my-app": {"vpa"}}, deploymentKinds(recorder.drain()))

	// A relist with unchanged content emits nothing.
	require.NoError(t, vpaStore.Replace([]interface{}{
		newAutoscalerObj("ns", "my-vpa", "targetRef", "Deployment", "my-app", autoscalerOpts{}),
	}, ""))
	assert.Empty(t, recorder.drain())

	assert.True(t, hpaStore.HasSynced())
	assert.True(t, vpaStore.HasSynced())
}

func TestAutoscalerIndexPodStamping(t *testing.T) {
	index, recorder := newTestIndex()
	hpaStore := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-app-7d9f8b6c5d-abcde",
			Namespace: "ns",
		},
		Owners: []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "my-app-7d9f8b6c5d"}},
	}

	// Pod first, autoscaler second: the autoscaler change must reach the pod.
	index.handlePodEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: pod}},
	})
	assert.Empty(t, recorder.drain(), "an unautoscaled pod should not be tagged")

	require.NoError(t, hpaStore.Add(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})))
	events := recorder.drain()
	require.Len(t, events, 2, "expected one deployment event and one pod event")
	var podEvents int
	for _, event := range events {
		if p, ok := event.Entity.(*workloadmeta.KubernetesPod); ok {
			podEvents++
			assert.Equal(t, "pod-uid", p.ID)
			assert.Equal(t, []string{"hpa"}, sets.List(p.AutoscalerKinds))
		}
	}
	assert.Equal(t, 1, podEvents)

	// Autoscaler first, pod second: the pod must be tagged as soon as it is
	// discovered, without waiting for another autoscaler change.
	newPod := &workloadmeta.KubernetesPod{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid-2"},
		EntityMeta: workloadmeta.EntityMeta{Name: "my-app-7d9f8b6c5d-fghij", Namespace: "ns"},
		Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "my-app-7d9f8b6c5d"}},
	}
	index.handlePodEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: newPod}},
	})
	events = recorder.drain()
	require.Len(t, events, 1)
	assert.Equal(t, []string{"hpa"}, sets.List(events[0].Entity.(*workloadmeta.KubernetesPod).AutoscalerKinds))

	// Once a pod is gone it must not be re-stamped.
	index.handlePodEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeUnset, Entity: pod}},
	})
	recorder.drain()
	require.NoError(t, hpaStore.Delete(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})))
	for _, event := range recorder.drain() {
		if p, ok := event.Entity.(*workloadmeta.KubernetesPod); ok {
			assert.NotEqual(t, "pod-uid", p.ID, "deleted pod should not receive events")
		}
	}
}

// A pod owned by an Argo Rollout also hangs off a ReplicaSet whose name parses
// like a Deployment's. It must not be attributed to a same-named Deployment's
// autoscaler.
func TestAutoscalerIndexRolloutPodIsNotADeployment(t *testing.T) {
	index, recorder := newTestIndex()
	hpaStore := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)

	require.NoError(t, hpaStore.Add(newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})))
	recorder.drain()

	index.handlePodEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "rollout-pod"},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "my-app-7d9f8b6c5d-abcde",
				Namespace: "ns",
				Labels:    map[string]string{kubernetes.ArgoRolloutLabelKey: "7d9f8b6c5d"},
			},
			Owners: []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "my-app-7d9f8b6c5d"}},
		}}},
	})

	assert.Empty(t, recorder.drain(), "a Rollout pod must not inherit a Deployment's autoscalers")
}

// A cluster without the WPA, DPA or VPA CRDs must still watch the built-in HPA.
// Crucially, the absent resources produce no store at all, so they can never
// end up in the collector's readiness set and block startup.
func TestStartAutoscalerStoresSkipsMissingCRDs(t *testing.T) {
	client := fakeclientset.NewSimpleClientset()
	fakeDiscovery, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok)

	fakeDiscovery.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "autoscaling/v2",
			APIResources: []metav1.APIResource{
				{Name: kubernetes.HorizontalPodAutoscalerResourceName, Namespaced: true, Kind: "HorizontalPodAutoscaler"},
			},
		},
	}

	index, _ := newTestIndex()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The reflector for a discovered resource really starts listing, so the
	// fake dynamic client needs the list kind registered.
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "autoscaling", Version: "v2", Resource: kubernetes.HorizontalPodAutoscalerResourceName}: "HorizontalPodAutoscalerList",
		},
	)

	stores := startAutoscalerStores(ctx, index, fakeDiscovery, dynamicClient)
	assert.Len(t, stores, 1, "only the discovered HPA resource should produce a store")
}

// eventCollector records every event a workloadmeta subscriber receives, in
// order. It appends before acknowledging, so once Notify returns on the
// synchronous mock store, the events are already recorded.
type eventCollector struct {
	mu     sync.Mutex
	events []workloadmeta.Event
}

func collectEvents(t *testing.T, store workloadmeta.Component, filter *workloadmeta.Filter) *eventCollector {
	t.Helper()
	c := &eventCollector{}
	ch := store.Subscribe("autoscaler-tags-test", workloadmeta.NormalPriority, filter)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for bundle := range ch {
			c.mu.Lock()
			c.events = append(c.events, bundle.Events...)
			c.mu.Unlock()
			bundle.Acknowledge()
		}
	}()
	t.Cleanup(func() {
		store.Unsubscribe(ch)
		<-done
	})
	return c
}

// last returns the most recent event delivered for the given entity ID: what a
// subscriber such as the tagger is left believing.
func (c *eventCollector) last(t *testing.T, id string) workloadmeta.Event {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.events) - 1; i >= 0; i-- {
		if c.events[i].Entity.GetID().ID == id {
			return c.events[i]
		}
	}
	require.FailNowf(t, "no event delivered", "entity %s", id)
	return workloadmeta.Event{}
}

// Removing the last autoscaler must clear the tag for subscribers, not just in
// the store.
//
// This goes through the real workloadmeta store because the failure mode lives
// there. When one source of a multi-source entity is unset, the store
// deliberately notifies subscribers with the *pre-delete* merged state (so
// container tags don't flap when collectors report a delete at different
// times). A bare Unset therefore leaves the tagger holding the old kinds
// forever, even though the store itself is correct. The event recorder used by
// the other tests cannot see that.
func TestAutoscalerTagsClearThroughRealStore(t *testing.T) {
	store := mockedWorkloadmeta(t)
	events := collectEvents(t, store, workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindKubernetesDeployment).Build())

	// The Deployment as the deployment reflector reports it: the second source
	// on the same entity is what makes the unset partial.
	store.Notify([]workloadmeta.CollectorEvent{{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceKubeAPIServer,
		Entity: &workloadmeta.KubernetesDeployment{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: "ns/my-app"},
			EntityMeta: workloadmeta.EntityMeta{Name: "my-app", Namespace: "ns"},
		},
	}})

	index := newAutoscalerIndex(store)
	hpaStore := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)
	hpa := newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "my-app", autoscalerOpts{})

	require.NoError(t, hpaStore.Add(hpa))
	got := events.last(t, "ns/my-app").Entity.(*workloadmeta.KubernetesDeployment)
	require.Equal(t, []string{"hpa"}, sets.List(got.AutoscalerKinds))

	require.NoError(t, hpaStore.Delete(hpa))

	// What the tagger sees.
	got = events.last(t, "ns/my-app").Entity.(*workloadmeta.KubernetesDeployment)
	assert.Empty(t, sets.List(got.AutoscalerKinds), "subscribers must see the kinds cleared")

	// The store is right too, and the Deployment itself is untouched.
	stored, err := store.GetKubernetesDeployment("ns/my-app")
	require.NoError(t, err, "the deployment reflector's entity must survive")
	assert.Empty(t, sets.List(stored.AutoscalerKinds))
	assert.Equal(t, "my-app", stored.Name)
}

// An autoscaler can point at a workload that does not exist, in which case the
// autoscaler source is the only one on the entity. Clearing must remove the
// entity outright rather than leave an empty one behind.
func TestAutoscalerTagsClearRemovesSoleSourceEntity(t *testing.T) {
	store := mockedWorkloadmeta(t)
	events := collectEvents(t, store, workloadmeta.NewFilterBuilder().AddKind(workloadmeta.KindKubernetesDeployment).Build())

	index := newAutoscalerIndex(store)
	hpaStore := newTestStore(index, hpaSpec(), kubernetes.HorizontalPodAutoscalerResourceName)
	hpa := newAutoscalerObj("ns", "my-hpa", "scaleTargetRef", "Deployment", "ghost", autoscalerOpts{})

	require.NoError(t, hpaStore.Add(hpa))
	require.NoError(t, hpaStore.Delete(hpa))

	_, err := store.GetKubernetesDeployment("ns/ghost")
	assert.Error(t, err, "an entity only the autoscaler source knew about must be removed")

	last := events.last(t, "ns/ghost")
	assert.Equal(t, workloadmeta.EventTypeUnset, last.Type)
	assert.Empty(t, sets.List(last.Entity.(*workloadmeta.KubernetesDeployment).AutoscalerKinds))
}
