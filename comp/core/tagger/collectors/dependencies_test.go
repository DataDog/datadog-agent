// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
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
	workload := entityDependency(workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueWorkload, ID: "default/job"})
	flavor := entityDependency(workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueResourceFlavor, ID: "a100"})

	d := newDependencyIndex()

	d.replace(pod, map[dependency]struct{}{workload: {}, flavor: {}}, true)
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(workload))
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(flavor))
	assert.True(t, d.isComplete(pod))

	d.replace(pod, map[dependency]struct{}{flavor: {}}, false)
	assert.Empty(t, d.readersOf(workload))
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(flavor))
	assert.False(t, d.isComplete(pod))

	d.forget(pod)
	assert.Empty(t, d.readersOf(flavor))
	assert.Empty(t, d.readers)
	assert.Empty(t, d.tagged)
}

func TestDependencyIndexChangedIncludesPreviousKeys(t *testing.T) {
	process := &workloadmeta.Process{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "1"}}
	processID := entityDependency(process.EntityID)
	d := newDependencyIndex()

	set := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: process}
	assert.ElementsMatch(t, []dependency{processID, keyDependency(workloadmeta.KindProcess, "ctr-a")}, d.changed(set, []string{"ctr-a"}))

	// The process moved to another container: readers of both are affected.
	assert.ElementsMatch(t, []dependency{
		processID,
		keyDependency(workloadmeta.KindProcess, "ctr-a"),
		keyDependency(workloadmeta.KindProcess, "ctr-b"),
	}, d.changed(set, []string{"ctr-b"}))

	// The process lost its key: readers of the old one are still affected.
	assert.ElementsMatch(t, []dependency{processID, keyDependency(workloadmeta.KindProcess, "ctr-b")}, d.changed(set, nil))
	assert.Empty(t, d.keys)

	d.changed(set, []string{"ctr-a"})
	unset := workloadmeta.Event{Type: workloadmeta.EventTypeUnset, Entity: process}
	assert.ElementsMatch(t, []dependency{processID, keyDependency(workloadmeta.KindProcess, "ctr-a")}, d.changed(unset, nil))
	assert.Empty(t, d.keys)

}

func TestTrackingStoreRecordsLookupsOnlyWhileTracking(t *testing.T) {
	s := &trackingStore{Component: newDependencyTestStore(t)}

	_, _ = s.GetKubernetesKueueWorkload("default/untracked")

	s.startTracking()
	// The entities are not in the store. Lookups that find nothing are
	// recorded too, so that the entity arriving later re-tags the reader.
	_, _ = s.GetKubernetesKueueWorkload("default/job")
	_, _ = s.GetKubernetesKueueResourceFlavor("a100")
	reads := s.stopTracking()

	assert.Equal(t, map[dependency]struct{}{
		entityDependency(workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueWorkload, ID: "default/job"}): {},
		entityDependency(workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueResourceFlavor, ID: "a100"}):  {},
	}, reads)

	_, _ = s.GetKubernetesKueueWorkload("default/untracked")
	assert.Nil(t, s.stopTracking())
}

func TestKueueWorkloadArrivingAfterPodRetagsPod(t *testing.T) {
	h := newDependencyTestHarness(t)

	got := h.add(newContainer("ctr-a"), newKueuePod("pod-a", "ctr-a", "job-a"))
	podTags := tagInfoFor(got, podTaggerID("pod-a"))
	require.NotNil(t, podTags)
	assert.NotContains(t, podTags.OrchestratorCardTags, "kueue_workload:job-a")

	got = h.add(newKueueWorkload("job-a", "a100"))

	for _, id := range []types.EntityID{podTaggerID("pod-a"), containerTaggerID("ctr-a")} {
		tagInfo := tagInfoFor(got, id)
		require.NotNil(t, tagInfo, "%s was not re-tagged", id)
		assert.Contains(t, tagInfo.OrchestratorCardTags, "kueue_workload:job-a", "tags for %s", id)
		assert.Contains(t, tagInfo.LowCardTags, "kueue_resource_flavor:a100", "tags for %s", id)
		assert.True(t, tagInfo.IsComplete, "completeness for %s", id)
	}
}

func TestKueueWorkloadRemovalRetagsPod(t *testing.T) {
	h := newDependencyTestHarness(t)
	workload := newKueueWorkload("job-a", "a100")

	got := h.add(newContainer("ctr-a"), newKueuePod("pod-a", "ctr-a", "job-a"), workload)
	require.Contains(t, tagInfoFor(got, podTaggerID("pod-a")).OrchestratorCardTags, "kueue_workload:job-a")

	h.store.Unset(workload)
	got = h.process(unsetEvent(workload))

	podTags := tagInfoFor(got, podTaggerID("pod-a"))
	require.NotNil(t, podTags, "pod was not re-tagged")
	assert.NotContains(t, podTags.OrchestratorCardTags, "kueue_workload:job-a")
}

func TestKueueResourceFlavorChangeRetagsOnlyPodsUsingIt(t *testing.T) {
	h := newDependencyTestHarness(t)

	got := h.add(
		newContainer("ctr-a"),
		newContainer("ctr-b"),
		newKueuePod("pod-a", "ctr-a", "job-a"),
		newKueuePod("pod-b", "ctr-b", "job-b"),
		newKueueWorkload("job-a", "a100"),
		newKueueWorkload("job-b", "h100"),
		newKueueResourceFlavor("a100", "ampere"),
		newKueueResourceFlavor("h100", "hopper"),
	)
	require.Contains(t, tagInfoFor(got, podTaggerID("pod-a")).LowCardTags, "gpu_architecture:ampere")

	got = h.add(newKueueResourceFlavor("a100", "blackwell"))

	podATags := tagInfoFor(got, podTaggerID("pod-a"))
	require.NotNil(t, podATags, "pod-a was not re-tagged")
	assert.Contains(t, podATags.LowCardTags, "gpu_architecture:blackwell")
	assert.NotNil(t, tagInfoFor(got, kueueWorkloadTaggerID("job-a")), "job-a was not re-tagged")

	assert.Nil(t, tagInfoFor(got, podTaggerID("pod-b")), "pod-b does not use the flavor")
	assert.Nil(t, tagInfoFor(got, kueueWorkloadTaggerID("job-b")), "job-b does not use the flavor")
}

func TestDeletedPodIsNotRetagged(t *testing.T) {
	h := newDependencyTestHarness(t)
	pod := newKueuePod("pod-a", "ctr-a", "job-a")
	workload := newKueueWorkload("job-a", "a100")
	h.add(newContainer("ctr-a"), pod, workload)

	h.store.Unset(pod)
	h.process(unsetEvent(pod))
	assert.Empty(t, h.collector.dependencies.readersOf(entityDependency(workload.EntityID)))

	got := h.add(newKueueWorkload("job-a", "h100"))
	assert.Nil(t, tagInfoFor(got, podTaggerID("pod-a")))
}

func TestEntityInBundleIsTaggedOnce(t *testing.T) {
	h := newDependencyTestHarness(t)
	pod := newKueuePod("pod-a", "ctr-a", "job-a")
	workload := newKueueWorkload("job-a", "a100")
	h.add(newContainer("ctr-a"), pod, workload)

	got := h.process(setEvent(pod), setEvent(workload))

	count := 0
	for _, tagInfo := range got {
		if tagInfo.EntityID == podTaggerID("pod-a") {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestProcessLosingPromotableTagRetagsContainer(t *testing.T) {
	h := newDependencyTestHarness(t)
	h.collector.promoteProcessTags = true

	h.add(newContainer("ctr-a"))
	got := h.add(newProcessWithTags(1, "ctr-a", "training_run_id:run-1"))
	assert.Contains(t, tagInfoFor(got, containerTaggerID("ctr-a")).LowCardTags, "training_run_id:run-1")

	// The process no longer has the tag. Its new state has no key, but the
	// container read the key the process had.
	got = h.add(newProcessWithTags(1, "ctr-a", "foo:bar"))
	containerTags := tagInfoFor(got, containerTaggerID("ctr-a"))
	require.NotNil(t, containerTags, "ctr-a was not re-tagged")
	assert.NotContains(t, containerTags.LowCardTags, "training_run_id:run-1")
}

func TestPodTaggedBeforeItsAutoscalerInTheSameBundle(t *testing.T) {
	h := newDependencyTestHarness(t)
	pod := newTestAutoscaledPod("pod-a", "my-ns", "app-a")
	hpa := newTestAutoscaler(workloadmeta.AutoscalerKindHPA, "my-ns", "my-hpa", workloadmeta.AutoscalerTargetDeployment, "app-a")
	h.store.Set(pod)

	// The pod is tagged first, and isn't re-tagged later in the bundle, so it
	// must already see the autoscaler.
	got := h.process(setEvent(pod), setEvent(hpa))

	podTags := tagInfoFor(got, podTaggerID("pod-a"))
	require.NotNil(t, podTags)
	assert.Contains(t, podTags.LowCardTags, "kube_autoscaler_kind:horizontalpodautoscaler")
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

func containerTaggerID(id string) types.EntityID {
	return types.NewEntityID(types.ContainerID, id)
}

func kueueWorkloadTaggerID(name string) types.EntityID {
	return common.BuildTaggerEntityID(workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesKueueWorkload,
		ID:   workloadmeta.GenerateKueueWorkloadEntityID("default", name),
	})
}

func newContainer(id string) *workloadmeta.Container {
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: id},
	}
}

func newKueuePod(uid, containerID, workloadName string) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: uid},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        uid,
			Namespace:   "default",
			Annotations: map[string]string{kubernetes.KueueWorkloadAnnotationKey: workloadName},
		},
		Containers: []workloadmeta.OrchestratorContainer{{ID: containerID, Name: "main"}},
	}
}

func newKueueWorkload(name, flavor string) *workloadmeta.KubernetesKueueWorkload {
	return &workloadmeta.KubernetesKueueWorkload{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueWorkload,
			ID:   workloadmeta.GenerateKueueWorkloadEntityID("default", name),
		},
		EntityMeta: workloadmeta.EntityMeta{Name: name, Namespace: "default", UID: name + "-uid"},
		PodSetAssignments: []workloadmeta.KueuePodSetAssignment{
			{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": flavor}},
		},
	}
}

func newKueueResourceFlavor(name, gpuFamily string) *workloadmeta.KubernetesKueueResourceFlavor {
	return &workloadmeta.KubernetesKueueResourceFlavor{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueResourceFlavor,
			ID:   workloadmeta.GenerateKueueResourceFlavorEntityID(name),
		},
		EntityMeta:         workloadmeta.EntityMeta{Name: name},
		NodeAffinityLabels: map[string]string{"nvidia.com/gpu.family": gpuFamily},
	}
}
