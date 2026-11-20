// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestDependencyIndex(t *testing.T) {
	pod := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod"}
	workload := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueWorkload, ID: "default/job"}
	flavor := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueResourceFlavor, ID: "a100"}

	d := newDependencyIndex()

	d.replace(pod, map[workloadmeta.EntityID]struct{}{workload: {}, flavor: {}})
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(workload))
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(flavor))

	d.replace(pod, map[workloadmeta.EntityID]struct{}{flavor: {}})
	assert.Empty(t, d.readersOf(workload))
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(flavor))

	d.forget(pod)
	assert.Empty(t, d.readersOf(flavor))
	assert.Empty(t, d.readers)
	assert.Empty(t, d.reads)
}

func TestTrackingStoreRecordsLookupsOnlyWhileTracking(t *testing.T) {
	s := &trackingStore{Component: newDependencyTestStore(t)}

	_, _ = s.GetKubernetesKAIPodGroup("default/untracked")

	s.startTracking()
	// The entity is not in the store. Lookups that find nothing are recorded
	// too, so that the entity arriving later re-tags the reader.
	_, _ = s.GetKubernetesKAIPodGroup("default/pg")
	reads := s.stopTracking()

	assert.Equal(t, map[workloadmeta.EntityID]struct{}{
		{Kind: workloadmeta.KindKubernetesKAIPodGroup, ID: "default/pg"}: {},
	}, reads)

	_, _ = s.GetKubernetesKAIPodGroup("default/untracked")
	assert.Nil(t, s.stopTracking())
}

// When one source of a pod is removed while another remains, workloadmeta
// sends a set event with the pod as it was before the removal, while the
// store already dropped that source's data. A pod tagged from that event is
// re-tagged from that event when a dependency changes, so it keeps the tags
// that workloadmeta preserves.
func TestReaderTaggedFromPreRemovalEventIsRetaggedFromIt(t *testing.T) {
	h := newDependencyTestHarness(t)
	podID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-a"}
	// The PodGroup annotation only comes from the kubelet.
	fromKubelet := &workloadmeta.KubernetesPod{
		EntityID: podID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:        "pod-a",
			Namespace:   "default",
			Annotations: map[string]string{kubernetes.KAIPodGroupAnnotationKey: "pg"},
		},
	}
	fromCluster := &workloadmeta.KubernetesPod{
		EntityID:   podID,
		EntityMeta: workloadmeta.EntityMeta{Name: "pod-a", Namespace: "default"},
	}
	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceClusterOrchestrator, Entity: fromCluster},
	})
	h.add(newKAIPodGroup("pg", "queue-a"))
	beforeRemoval, err := h.store.GetKubernetesPod(podID.ID)
	require.NoError(t, err)
	got := h.process(setEvent(beforeRemoval))
	require.Contains(t, kaiTags(tagInfoFor(got, podTaggerID("pod-a"))), "kai_queue:queue-a")

	// The kubelet removes the pod first, and the PodGroup changes in the same
	// bundle. The pre-removal copy carries the annotation; the store's pod
	// no longer does.
	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeUnset, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
	})
	preRemovalCopy := beforeRemoval.DeepCopy()
	updated := newKAIPodGroup("pg", "queue-b")
	h.store.Set(updated)
	got = h.process(setEvent(preRemovalCopy), setEvent(updated))

	podTags := tagInfoFor(got, podTaggerID("pod-a"))
	require.NotNil(t, podTags)
	assert.Contains(t, kaiTags(podTags), "kai_queue:queue-b")
}

// A pod tagged from its pre-removal event keeps being re-tagged from it in
// later bundles, by its dependencies and by autoscaler changes, which match
// the pod's owner as the event had it.
func TestPodTaggedFromPreRemovalEventIsRetaggedFromItLater(t *testing.T) {
	h := newDependencyTestHarness(t)
	podID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-a"}
	// The PodGroup annotation and the owner only come from the kubelet.
	fromKubelet := &workloadmeta.KubernetesPod{
		EntityID: podID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:        "pod-a",
			Namespace:   "default",
			Annotations: map[string]string{kubernetes.KAIPodGroupAnnotationKey: "pg"},
		},
		Owners: []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.DeploymentKind, Name: "web"}},
	}
	fromCluster := &workloadmeta.KubernetesPod{
		EntityID:   podID,
		EntityMeta: workloadmeta.EntityMeta{Name: "pod-a", Namespace: "default"},
	}
	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceClusterOrchestrator, Entity: fromCluster},
	})
	h.add(newKAIPodGroup("pg", "queue-a"))
	beforeRemoval, err := h.store.GetKubernetesPod(podID.ID)
	require.NoError(t, err)
	h.process(setEvent(beforeRemoval))

	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeUnset, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
	})
	h.process(setEvent(beforeRemoval.DeepCopy()))

	podTags := mustTagInfo(t, h.add(newKAIPodGroup("pg", "queue-b")), podTaggerID("pod-a"))
	assert.Contains(t, kaiTags(podTags), "kai_queue:queue-b")

	hpa := newTestAutoscaler(workloadmeta.AutoscalerKindHPA, "default", "hpa", workloadmeta.AutoscalerTargetDeployment, "web")
	podTags = mustTagInfo(t, h.add(hpa), podTaggerID("pod-a"))
	assert.Contains(t, append(podTags.LowCardTags, podTags.OrchestratorCardTags...), "kube_autoscaler_kind:horizontalpodautoscaler")
	assert.Contains(t, kaiTags(podTags), "kai_queue:queue-b")

	h.store.Unset(hpa)
	podTags = mustTagInfo(t, h.process(unsetEvent(hpa)), podTaggerID("pod-a"))
	assert.NotContains(t, append(podTags.LowCardTags, podTags.OrchestratorCardTags...), "kube_autoscaler_kind:horizontalpodautoscaler")
}

type dependencyTestHarness struct {
	store     workloadmetamock.Mock
	collector *WorkloadMetaCollector
}

func newDependencyTestHarness(t *testing.T) *dependencyTestHarness {
	store := newDependencyTestStore(t)
	return &dependencyTestHarness{
		store:     store,
		collector: NewWorkloadMetaCollector(context.Background(), configmock.New(t), store, nil),
	}
}

func newDependencyTestStore(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

// add sets the entities in the store and processes their set events as one
// bundle, as workloadmeta would for a single Notify.
func (h *dependencyTestHarness) add(entities ...workloadmeta.Entity) []*types.TagInfo {
	events := make([]workloadmeta.Event, 0, len(entities))
	for _, entity := range entities {
		h.store.Set(entity)
		events = append(events, setEvent(entity))
	}
	return h.process(events...)
}

// process runs one bundle through the collector and returns the tags it
// produced.
func (h *dependencyTestHarness) process(events ...workloadmeta.Event) []*types.TagInfo {
	var got []*types.TagInfo
	h.collector.tagProcessor = processorFunc(func(tagInfos []*types.TagInfo) {
		got = append(got, tagInfos...)
	})
	h.collector.processEvents(workloadmeta.EventBundle{Events: events, Ch: make(chan struct{})})
	return got
}

type processorFunc func([]*types.TagInfo)

func (f processorFunc) ProcessTagInfo(tagInfos []*types.TagInfo) {
	f(tagInfos)
}

// mustTagInfo is tagInfoFor for tag infos the test requires.
func mustTagInfo(t *testing.T, tagInfos []*types.TagInfo, id types.EntityID) *types.TagInfo {
	t.Helper()
	tagInfo := tagInfoFor(tagInfos, id)
	require.NotNil(t, tagInfo, "no tags for %s", id)
	return tagInfo
}

func tagInfoFor(tagInfos []*types.TagInfo, id types.EntityID) *types.TagInfo {
	var found *types.TagInfo
	for _, tagInfo := range tagInfos {
		if tagInfo.EntityID == id && !tagInfo.DeleteEntity {
			found = tagInfo
		}
	}
	return found
}

func setEvent(entity workloadmeta.Entity) workloadmeta.Event {
	return workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: entity, IsComplete: true}
}

func unsetEvent(entity workloadmeta.Entity) workloadmeta.Event {
	return workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: entity, IsComplete: true}
}

func podTaggerID(uid string) types.EntityID {
	return types.NewEntityID(types.KubernetesPodUID, uid)
}

func newKAIPodGroup(name, queue string) *workloadmeta.KubernetesKAIPodGroup {
	return &workloadmeta.KubernetesKAIPodGroup{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKAIPodGroup,
			ID:   workloadmeta.GenerateKAIPodGroupEntityID("default", name),
		},
		EntityMeta: workloadmeta.EntityMeta{Name: name, Namespace: "default"},
		QueueName:  queue,
	}
}

func kaiTags(tagInfo *types.TagInfo) []string {
	if tagInfo == nil {
		return nil
	}
	var kai []string
	for _, tag := range append(tagInfo.LowCardTags, tagInfo.OrchestratorCardTags...) {
		if strings.HasPrefix(tag, "kai_") {
			kai = append(kai, tag)
		}
	}
	return kai
}
