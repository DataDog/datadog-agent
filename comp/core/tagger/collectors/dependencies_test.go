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

	d.replace(pod, map[dependency]struct{}{workload: {}, flavor: {}})
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(workload))
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(flavor))

	d.replace(pod, map[dependency]struct{}{flavor: {}})
	assert.Empty(t, d.readersOf(workload))
	assert.ElementsMatch(t, []workloadmeta.EntityID{pod}, d.readersOf(flavor))

	d.forget(pod)
	assert.Empty(t, d.readersOf(flavor))
	assert.Empty(t, d.readers)
	assert.Empty(t, d.reads)
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
	require.Contains(t, mustTagInfo(t, got, podTaggerID("pod-a")).OrchestratorCardTags, "kueue_workload:job-a")

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
	require.Contains(t, mustTagInfo(t, got, podTaggerID("pod-a")).LowCardTags, "gpu_architecture:ampere")

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

func TestReaderIsRetaggedOncePerBundle(t *testing.T) {
	h := newDependencyTestHarness(t)
	h.add(newContainer("ctr-a"), newKueuePod("pod-a", "ctr-a", "job-a"), newKueueWorkload("job-a", "a100"), newKueueResourceFlavor("a100", "ampere"))

	// Both entities that the pod reads change in the same bundle.
	got := h.add(newKueueWorkload("job-a", "h100"), newKueueResourceFlavor("a100", "hopper"))

	count := 0
	for _, tagInfo := range got {
		if tagInfo.EntityID == podTaggerID("pod-a") {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

// A pod tagged before its container in a bundle computes the container's tags
// with the container's previous completeness. When a dependency of the pod
// changes in the same bundle, its re-tag runs after the whole bundle, with the
// container's new completeness.
func TestReaderRetagUsesStateUpdatedLaterInTheBundle(t *testing.T) {
	h := newDependencyTestHarness(t)
	container := newContainer("ctr-a")
	pod := newKueuePod("pod-a", "ctr-a", "job-a")
	h.store.Set(container)
	h.store.Set(pod)
	workload := newKueueWorkload("job-a", "a100")
	h.store.Set(workload)

	got := h.process(
		setEvent(pod),
		setEvent(container),
		setEvent(workload),
	)

	var fromPod *types.TagInfo
	for _, tagInfo := range got {
		if tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromPod = tagInfo
		}
	}
	require.NotNil(t, fromPod, "the pod didn't tag its container")
	assert.True(t, fromPod.IsComplete, "the container's tags from the pod use its completeness from later in the bundle")
	assert.Contains(t, fromPod.OrchestratorCardTags, "kueue_workload:job-a")
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

// An autoscaler change re-tags a pod through the same tracked path as its
// dependencies, so a container missing at that point re-tags the pod when it
// arrives, and gets the autoscaler's tags.
func TestContainerMissingOnAutoscalerRetagRetagsThePodOnArrival(t *testing.T) {
	h := newDependencyTestHarness(t)
	pod := &workloadmeta.KubernetesPod{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-a"},
		EntityMeta: workloadmeta.EntityMeta{Name: "pod-a", Namespace: "default"},
		Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.DeploymentKind, Name: "web"}},
		Containers: []workloadmeta.OrchestratorContainer{{ID: "ctr-a", Name: "ctr-a"}},
	}
	container := newContainer("ctr-a")
	h.add(container, pod)

	h.store.Unset(container)
	h.process(unsetEvent(container))
	h.add(newTestAutoscaler(workloadmeta.AutoscalerKindHPA, "default", "hpa", workloadmeta.AutoscalerTargetDeployment, "web"))

	got := h.add(container)
	var fromPod *types.TagInfo
	for _, tagInfo := range got {
		if tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromPod = tagInfo
		}
	}
	require.NotNil(t, fromPod, "the container's arrival didn't re-tag the pod")
	assert.Contains(t, append(fromPod.LowCardTags, fromPod.OrchestratorCardTags...), "kube_autoscaler_kind:horizontalpodautoscaler")
}

// The global tags of ECS tasks share a source, so a re-tag of a task after a
// dependency change must not replace them with the task's: only task events do.
func TestECSTaskRetagLeavesGlobalTagsAlone(t *testing.T) {
	h := newDependencyTestHarness(t)
	newTask := func(id, clusterARN, containerID string) *workloadmeta.ECSTask {
		return &workloadmeta.ECSTask{
			EntityID:    workloadmeta.EntityID{Kind: workloadmeta.KindECSTask, ID: id},
			EntityMeta:  workloadmeta.EntityMeta{Name: id},
			LaunchType:  workloadmeta.ECSLaunchTypeEC2,
			ClusterName: "cluster",
			ClusterARN:  clusterARN,
			Containers:  []workloadmeta.OrchestratorContainer{{ID: containerID, Name: containerID}},
		}
	}
	h.add(newTask("task-a", "", "ctr-a"))
	h.add(newTask("task-b", "arn:cluster", "ctr-b"))

	got := h.add(newContainer("ctr-a"))
	var fromTask *types.TagInfo
	for _, tagInfo := range got {
		assert.NotEqual(t, types.GetGlobalEntityID(), tagInfo.EntityID, "the re-tag of task-a replaced the global tags")
		if tagInfo.Source == taskSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromTask = tagInfo
		}
	}
	require.NotNil(t, fromTask, "the container's arrival didn't re-tag task-a")
}

// A process re-tags its container, but not before the container's own event:
// the pod tagged in between must still see the container's completeness as
// unknown, so that the container's event re-tags it.
func TestProcessBeforeItsContainerLeavesItsCompletenessUnknown(t *testing.T) {
	h := newDependencyTestHarness(t)
	h.collector.promoteProcessTags = true
	process := newProcessWithTags(1, "ctr-a", "training_run_id:run-1")
	pod := newKueuePod("pod-a", "ctr-a", "")
	container := newContainer("ctr-a")
	h.store.Set(container)
	h.store.Set(process)
	h.store.Set(pod)

	got := h.process(setEvent(process), setEvent(pod), setEvent(container))

	var fromPod *types.TagInfo
	for _, tagInfo := range got {
		if tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromPod = tagInfo
		}
	}
	require.NotNil(t, fromPod)
	assert.True(t, fromPod.IsComplete)
}

// A container's first event in the bundle of a partial unset of its pod
// re-tags the pod from the pod's event, not from the store, which lost the
// removed source's data.
func TestContainerArrivalKeepsPreRemovalPodTags(t *testing.T) {
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
		Containers: []workloadmeta.OrchestratorContainer{{ID: "ctr-a", Name: "ctr-a"}},
	}
	fromCluster := &workloadmeta.KubernetesPod{
		EntityID:   podID,
		EntityMeta: workloadmeta.EntityMeta{Name: "pod-a", Namespace: "default"},
		Containers: []workloadmeta.OrchestratorContainer{{ID: "ctr-a", Name: "ctr-a"}},
	}
	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceClusterOrchestrator, Entity: fromCluster},
	})
	h.add(newKAIPodGroup("pg", "queue-a"))
	beforeRemoval, err := h.store.GetKubernetesPod(podID.ID)
	require.NoError(t, err)

	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeUnset, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
	})
	container := newContainer("ctr-a")
	h.store.Set(container)
	got := h.process(setEvent(beforeRemoval), setEvent(container))

	assert.Contains(t, kaiTags(mustTagInfo(t, got, podTaggerID("pod-a"))), "kai_queue:queue-a")
	var fromPod *types.TagInfo
	for _, tagInfo := range got {
		if tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromPod = tagInfo
		}
	}
	require.NotNil(t, fromPod)
	assert.True(t, fromPod.IsComplete)
}

// A pod tagged before its container is in workloadmeta can't tag it. The
// container's arrival re-tags the pod, so the container gets the pod's tags.
func TestContainerArrivingAfterItsPodRetagsThePod(t *testing.T) {
	h := newDependencyTestHarness(t)
	h.add(newKueueWorkload("job-a", "a100"))

	got := h.add(newKueuePod("pod-a", "ctr-a", "job-a"))
	require.Nil(t, tagInfoFor(got, containerTaggerID("ctr-a")), "the container isn't in workloadmeta yet")

	got = h.add(newContainer("ctr-a"))
	var fromPod *types.TagInfo
	for _, tagInfo := range got {
		if tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromPod = tagInfo
		}
	}
	require.NotNil(t, fromPod, "the pod wasn't re-tagged when its container arrived")
	assert.Contains(t, fromPod.OrchestratorCardTags, "kueue_workload:job-a")

	// Once found, later changes to the container don't re-tag the pod.
	assert.Empty(t, h.collector.dependencies.readersOf(entityDependency(workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "ctr-a"})))
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

// A pod tagged before its container's event in a bundle computes the
// container's tags before the container's completeness is known. The
// container's event re-tags the pod, even without a scheduler event.
func TestPodBeforeItsContainerInABundleTagsItComplete(t *testing.T) {
	h := newDependencyTestHarness(t)
	container := newContainer("ctr-a")
	pod := newKueuePod("pod-a", "ctr-a", "")
	h.store.Set(container)
	h.store.Set(pod)

	got := h.process(setEvent(pod), setEvent(container))

	var fromPod *types.TagInfo
	for _, tagInfo := range got {
		if tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && !tagInfo.DeleteEntity {
			fromPod = tagInfo
		}
	}
	require.NotNil(t, fromPod)
	assert.True(t, fromPod.IsComplete)
	assert.Empty(t, h.collector.dependencies.readersOf(entityDependency(workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "ctr-a"})))
}

// After a partial unset, the store's pod can lack the containers that
// workloadmeta's pre-removal event still has. A re-tag after a dependency
// change must not expire the container tags that event keeps.
func TestDependencyRetagKeepsChildrenAfterPartialUnset(t *testing.T) {
	h := newDependencyTestHarness(t)
	podID := workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-a"}
	// The containers and the Workload annotation only come from the kubelet.
	fromKubelet := newKueuePod("pod-a", "ctr-a", "job-a")
	fromCluster := &workloadmeta.KubernetesPod{
		EntityID:   podID,
		EntityMeta: workloadmeta.EntityMeta{Name: "pod-a", Namespace: "default"},
	}
	h.add(newContainer("ctr-a"), newKueueWorkload("job-a", "a100"))
	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceClusterOrchestrator, Entity: fromCluster},
	})
	beforeRemoval, err := h.store.GetKubernetesPod(podID.ID)
	require.NoError(t, err)
	h.process(setEvent(beforeRemoval))

	h.store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeUnset, Source: workloadmeta.SourceNodeOrchestrator, Entity: fromKubelet},
	})
	updated := newKueueWorkload("job-a", "h100")
	h.store.Set(updated)
	got := h.process(setEvent(beforeRemoval.DeepCopy()), setEvent(updated))

	for _, tagInfo := range got {
		assert.False(t, tagInfo.Source == podSource && tagInfo.EntityID == containerTaggerID("ctr-a") && tagInfo.DeleteEntity,
			"the container's tags from the pod were expired")
	}
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
