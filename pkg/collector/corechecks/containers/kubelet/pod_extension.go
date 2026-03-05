// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"slices"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetafilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/util/workloadmeta"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var includeContainerStateReason = map[string][]string{
	"waiting": {
		"errimagepull",
		"imagepullbackoff",
		"crashloopbackoff",
		"containercreating",
		"createcontainererror",
		"invalidimagename",
		"createcontainerconfigerror",
	},
	"terminated": {"oomkilled", "containercannotrun", "error"},
}

const kubeNamespaceTag = tags.KubeNamespace
const kubePodConditionResizePending = "PodResizePending"

// podExtension implements generic.ProcessorExtension and replaces the pod provider.
// It handles pod-level and container-status metrics that the generic processor's
// container iteration cannot cover (resource requests/limits, restart counts,
// container state reasons, running aggregation, pod termination, pod resize).
//
// Most work is done in PreProcess (which iterates pods independently) because
// the generic processor iterates runtime containers, while this extension needs
// to iterate pod container statuses (including init and ephemeral containers).
type podExtension struct {
	store           workloadmeta.Component
	containerFilter workloadfilter.FilterBundle
	config          *common.KubeletConfig
	podUtils        *common.PodUtils
	tagger          tagger.Component
	now             func() time.Time

	aggSender         sender.Sender
	runningAggregator *runningAggregator
}

func newPodExtension(
	filterStore workloadfilter.Component,
	store workloadmeta.Component,
	config *common.KubeletConfig,
	podUtils *common.PodUtils,
	tagger tagger.Component,
) *podExtension {
	return &podExtension{
		containerFilter: filterStore.GetContainerSharedMetricFilters(),
		store:           store,
		config:          config,
		podUtils:        podUtils,
		tagger:          tagger,
		now:             time.Now,
	}
}

// PreProcess iterates all pods and their container statuses to emit pod-level
// and container-status metrics. This runs before the generic processor's
// per-container iteration.
func (e *podExtension) PreProcess(_ generic.SenderFunc, aggSender sender.Sender) {
	e.aggSender = aggSender
	e.runningAggregator = newRunningAggregator()

	if kubeletMetrics, err := e.store.GetKubeletMetrics(); err == nil && kubeletMetrics != nil {
		aggSender.Gauge(common.KubeletMetricsPrefix+"pods.expired", float64(kubeletMetrics.ExpiredPodCount), "", e.config.Tags)
	}

	for _, pod := range e.store.ListKubernetesPods() {
		e.processWorkloadmetaPod(pod)
	}
}

// Process is a no-op. The pod extension's per-container work is done in
// PreProcess because it needs to iterate container statuses (including init
// and ephemeral containers) which the generic processor does not provide.
func (e *podExtension) Process(_ []string, _ *workloadmeta.Container, _ metrics.Collector, _ time.Duration) {
}

// PostProcess emits the aggregated running pod/container counts.
func (e *podExtension) PostProcess(_ tagger.Component) {
	if e.runningAggregator != nil && e.aggSender != nil {
		e.runningAggregator.generateRunningAggregatorMetrics(e.aggSender)
	}
}

func (e *podExtension) processWorkloadmetaPod(pod *workloadmeta.KubernetesPod) {
	e.podUtils.PopulateForPod(pod)

	allContainerStatuses := make([]workloadmeta.KubernetesContainerStatus, 0,
		len(pod.InitContainerStatuses)+len(pod.ContainerStatuses)+len(pod.EphemeralContainerStatuses))
	allContainerStatuses = append(allContainerStatuses, pod.InitContainerStatuses...)
	allContainerStatuses = append(allContainerStatuses, pod.ContainerStatuses...)
	allContainerStatuses = append(allContainerStatuses, pod.EphemeralContainerStatuses...)

	allContainerSpecs := make([]workloadmeta.OrchestratorContainer, 0,
		len(pod.InitContainers)+len(pod.Containers)+len(pod.EphemeralContainers))
	allContainerSpecs = append(allContainerSpecs, pod.InitContainers...)
	allContainerSpecs = append(allContainerSpecs, pod.Containers...)
	allContainerSpecs = append(allContainerSpecs, pod.EphemeralContainers...)

	for _, cStatus := range allContainerStatuses {
		if cStatus.ContainerID == "" {
			continue
		}

		cID, err := kubelet.KubeContainerIDToTaggerEntityID(cStatus.ContainerID)
		if err != nil {
			continue
		}

		var containerSpec *workloadmeta.OrchestratorContainer
		for i := range allContainerSpecs {
			if cStatus.Name == allContainerSpecs[i].Name {
				containerSpec = &allContainerSpecs[i]
				break
			}
		}

		e.runningAggregator.recordContainer(e, pod, &cStatus, cID)

		if containerSpec == nil {
			continue
		}

		filterableContainer := workloadmetafilter.CreateContainerFromOrch(containerSpec, workloadmetafilter.CreatePod(pod))
		if e.containerFilter.IsExcluded(filterableContainer) {
			continue
		}

		e.generateContainerSpecMetrics(pod, &cStatus, cID)
		e.generateContainerStatusMetrics(pod, &cStatus, cID)
	}

	e.generatePodTerminationMetric(pod)
	e.generatePodResizeMetric(pod)

	e.runningAggregator.recordPod(e, pod)
}

func (e *podExtension) generateContainerSpecMetrics(pod *workloadmeta.KubernetesPod, cStatus *workloadmeta.KubernetesContainerStatus, containerID types.EntityID) {
	if pod.Phase != "Running" && pod.Phase != "Pending" {
		return
	}

	if cStatus.State.Terminated != nil && cStatus.State.Terminated.Reason == "Completed" {
		return
	}

	tagList, _ := e.tagger.Tag(containerID, types.HighCardinality)
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	tagList = utils.ConcatenateTags(tagList, e.config.Tags)

	containerEntity, err := e.store.GetContainer(containerID.GetID())
	if err != nil || containerEntity == nil {
		return
	}

	tagList = common.AppendKubeStaticCPUsTag(e.store, pod.QOSClass, containerID, tagList)

	for r, value := range containerEntity.Resources.RawRequests {
		quantity, err := resource.ParseQuantity(value)
		if err != nil {
			log.Warnf("Failed to parse resource quantity %s: %s", value, err)
			continue
		}
		e.aggSender.Gauge(common.KubeletMetricsPrefix+string(r)+".requests", quantity.AsApproximateFloat64(), "", tagList)
	}

	for r, value := range containerEntity.Resources.RawLimits {
		quantity, err := resource.ParseQuantity(value)
		if err != nil {
			log.Warnf("Failed to parse resource quantity %s: %s", value, err)
			continue
		}
		e.aggSender.Gauge(common.KubeletMetricsPrefix+string(r)+".limits", quantity.AsApproximateFloat64(), "", tagList)
	}
}

func (e *podExtension) generateContainerStatusMetrics(pod *workloadmeta.KubernetesPod, cStatus *workloadmeta.KubernetesContainerStatus, containerID types.EntityID) {
	if pod.ID == "" || pod.Name == "" {
		return
	}

	tagList, _ := e.tagger.Tag(containerID, types.OrchestratorCardinality)
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	tagList = utils.ConcatenateTags(tagList, e.config.Tags)

	e.aggSender.Gauge(common.KubeletMetricsPrefix+"containers.restarts", float64(cStatus.RestartCount), "", tagList)

	for key, state := range map[string]workloadmeta.KubernetesContainerState{"state": cStatus.State, "last_state": cStatus.LastTerminationState} {
		if state.Terminated != nil && slices.Contains(includeContainerStateReason["terminated"], strings.ToLower(state.Terminated.Reason)) {
			termTags := utils.ConcatenateStringTags(tagList, "reason:"+strings.ToLower(state.Terminated.Reason))
			e.aggSender.Gauge(common.KubeletMetricsPrefix+"containers."+key+".terminated", 1, "", termTags)
		}
		if state.Waiting != nil && slices.Contains(includeContainerStateReason["waiting"], strings.ToLower(state.Waiting.Reason)) {
			waitTags := utils.ConcatenateStringTags(tagList, "reason:"+strings.ToLower(state.Waiting.Reason))
			e.aggSender.Gauge(common.KubeletMetricsPrefix+"containers."+key+".waiting", 1, "", waitTags)
		}
	}
}

func (e *podExtension) generatePodTerminationMetric(pod *workloadmeta.KubernetesPod) {
	if pod.DeletionTimestamp == nil {
		return
	}

	dur := e.now().Sub(*pod.DeletionTimestamp)
	if dur < 0 {
		return
	}

	podID := pod.ID
	if podID == "" {
		log.Debugf("skipping pod with no uid for termination metric, duration: %f", dur.Seconds())
		return
	}
	entityID := types.NewEntityID(types.KubernetesPodUID, podID)
	tagList, _ := e.tagger.Tag(entityID, types.LowCardinality)
	tagList = utils.ConcatenateTags(tagList, e.config.Tags)

	e.aggSender.Gauge(common.KubeletMetricsPrefix+"pod.terminating.duration", float64(dur.Seconds()), "", tagList)
}

func (e *podExtension) generatePodResizeMetric(pod *workloadmeta.KubernetesPod) {
	var cond *workloadmeta.KubernetesPodCondition
	for _, c := range pod.Conditions {
		if c.Type == kubePodConditionResizePending {
			cond = &c
			break
		}
	}

	if cond == nil {
		return
	}

	podID := pod.ID
	if podID == "" {
		log.Debug("skipping pod with no uid for pod resize metric")
		return
	}
	entityID := types.NewEntityID(types.KubernetesPodUID, podID)
	tagList, _ := e.tagger.Tag(entityID, types.HighCardinality)
	tagList = utils.ConcatenateTags(tagList, e.config.Tags)

	tagList = utils.ConcatenateStringTags(tagList, "reason:"+strings.ToLower(cond.Reason))

	e.aggSender.Gauge(common.KubeletMetricsPrefix+"pod.resize.pending", 1.0, "", tagList)
}

type runningAggregator struct {
	runningPodsCounter       map[string]float64
	runningPodsTags          map[string][]string
	runningContainersCounter map[string]float64
	runningContainersTags    map[string][]string
	podHasRunningContainers  map[string]bool
}

func newRunningAggregator() *runningAggregator {
	return &runningAggregator{
		runningPodsCounter:       make(map[string]float64),
		runningPodsTags:          make(map[string][]string),
		runningContainersCounter: make(map[string]float64),
		runningContainersTags:    make(map[string][]string),
		podHasRunningContainers:  make(map[string]bool),
	}
}

func (r *runningAggregator) recordContainer(e *podExtension, pod *workloadmeta.KubernetesPod, cStatus *workloadmeta.KubernetesContainerStatus, containerID types.EntityID) {
	if cStatus.State.Running == nil || cStatus.State.Running.StartedAt.IsZero() {
		return
	}
	r.podHasRunningContainers[pod.ID] = true
	tagList, _ := e.tagger.Tag(containerID, types.LowCardinality)
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	hashTags := generateTagHash(tagList)
	r.runningContainersCounter[hashTags]++
	if _, ok := r.runningContainersTags[hashTags]; !ok {
		r.runningContainersTags[hashTags] = utils.ConcatenateTags(tagList, e.config.Tags)
	}
}

func (r *runningAggregator) recordPod(e *podExtension, pod *workloadmeta.KubernetesPod) {
	if !r.podHasRunningContainers[pod.ID] {
		return
	}
	podID := pod.ID
	if podID == "" {
		log.Debug("skipping pod with no uid")
		return
	}
	entityID := types.NewEntityID(types.KubernetesPodUID, podID)
	tagList, _ := e.tagger.Tag(entityID, types.LowCardinality)
	if len(tagList) == 0 {
		return
	}
	hashTags := generateTagHash(tagList)
	r.runningPodsCounter[hashTags]++
	if _, ok := r.runningPodsTags[hashTags]; !ok {
		r.runningPodsTags[hashTags] = utils.ConcatenateTags(tagList, e.config.Tags)
	}
}

func (r *runningAggregator) generateRunningAggregatorMetrics(sender sender.Sender) {
	for hash, count := range r.runningContainersCounter {
		sender.Gauge(common.KubeletMetricsPrefix+"containers.running", count, "", r.runningContainersTags[hash])
	}
	for hash, count := range r.runningPodsCounter {
		sender.Gauge(common.KubeletMetricsPrefix+"pods.running", count, "", r.runningPodsTags[hash])
	}
}

func generateTagHash(tagSlice []string) string {
	sortedTags := make([]string, len(tagSlice))
	copy(sortedTags, tagSlice)
	sort.Strings(sortedTags)
	return strings.Join(sortedTags, ",")
}

func isTagKeyPresent(key string, tagSlice []string) bool {
	for _, tag := range tagSlice {
		if strings.HasPrefix(tag, key+":") {
			return true
		}
	}
	return false
}
