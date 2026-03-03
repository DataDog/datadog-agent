// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package pod is responsible for emitting the Kubelet check metrics that are
// collected from the pod endpoints.
package pod

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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
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

// Provider provides the metrics related to data collected from the `/pods` Kubelet endpoint
type Provider struct {
	store           workloadmeta.Component
	containerFilter workloadfilter.FilterBundle
	config          *common.KubeletConfig
	podUtils        *common.PodUtils
	tagger          tagger.Component
	// now timer func is used to mock time in tests
	now func() time.Time
}

// NewProvider returns a new Provider
func NewProvider(filterStore workloadfilter.Component, store workloadmeta.Component, config *common.KubeletConfig,
	podUtils *common.PodUtils, tagger tagger.Component) *Provider {
	return &Provider{
		containerFilter: filterStore.GetContainerSharedMetricFilters(),
		store:           store,
		config:          config,
		podUtils:        podUtils,
		tagger:          tagger,
		now:             time.Now,
	}
}

// Provide provides the metrics related to Kubelet pods using workloadmeta
func (p *Provider) Provide(_ kubelet.KubeUtilInterface, sender sender.Sender) error {
	if kubeletMetrics, err := p.store.GetKubeletMetrics(); err == nil && kubeletMetrics != nil {
		sender.Gauge(common.KubeletMetricsPrefix+"pods.expired", float64(kubeletMetrics.ExpiredPodCount), "", p.config.Tags)
	}

	runningAggregator := newRunningAggregator()

	for _, pod := range p.store.ListKubernetesPods() {
		p.processWorkloadmetaPod(pod, sender, runningAggregator)
	}

	runningAggregator.generateRunningAggregatorMetrics(sender)

	return nil
}

func (p *Provider) processWorkloadmetaPod(pod *workloadmeta.KubernetesPod, sender sender.Sender, runningAggregator *runningAggregator) {
	p.podUtils.PopulateForPod(pod)

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
			// no container ID means we could not find the matching container status
			continue
		}

		cID, err := kubelet.KubeContainerIDToTaggerEntityID(cStatus.ContainerID)
		if err != nil {
			// could not correctly parse container ID
			continue
		}

		// Find the corresponding container spec
		var containerSpec *workloadmeta.OrchestratorContainer
		for i := range allContainerSpecs {
			if cStatus.Name == allContainerSpecs[i].Name {
				containerSpec = &allContainerSpecs[i]
				break
			}
		}

		runningAggregator.recordContainer(p, pod, &cStatus, cID)

		if containerSpec == nil {
			continue
		}

		// don't exclude filtered containers from aggregation, but filter them out from other reported metrics
		filterableContainer := workloadmetafilter.CreateContainerFromOrch(containerSpec, workloadmetafilter.CreatePod(pod))
		if p.containerFilter.IsExcluded(filterableContainer) {
			continue
		}

		p.generateContainerSpecMetrics(sender, pod, &cStatus, cID)
		p.generateContainerStatusMetrics(sender, pod, &cStatus, cID)
	}

	p.generatePodTerminationMetric(sender, pod)
	p.generatePodResizeMetric(sender, pod)

	runningAggregator.recordPod(p, pod)
}

func (p *Provider) generateContainerSpecMetrics(sender sender.Sender, pod *workloadmeta.KubernetesPod, cStatus *workloadmeta.KubernetesContainerStatus, containerID types.EntityID) {
	if pod.Phase != "Running" && pod.Phase != "Pending" {
		return
	}

	// Filter out containers which have completed, as their resources should be freed
	if cStatus.State.Terminated != nil && cStatus.State.Terminated.Reason == "Completed" {
		return
	}

	tagList, _ := p.tagger.Tag(containerID, types.HighCardinality)
	// Skip recording containers without kubelet information in tagger or if there are no tags
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	containerEntity, err := p.store.GetContainer(containerID.GetID())
	if err != nil || containerEntity == nil {
		// Container not found in workloadmeta, skip resource metrics
		return
	}

	tagList = common.AppendKubeStaticCPUsTag(p.store, pod.QOSClass, containerID, tagList)

	for r, value := range containerEntity.Resources.RawRequests {
		quantity, err := resource.ParseQuantity(value)
		if err != nil {
			log.Warnf("Failed to parse resource quantity %s: %s", value, err)
			continue
		}
		sender.Gauge(common.KubeletMetricsPrefix+string(r)+".requests", quantity.AsApproximateFloat64(), "", tagList)
	}

	for r, value := range containerEntity.Resources.RawLimits {
		quantity, err := resource.ParseQuantity(value)
		if err != nil {
			log.Warnf("Failed to parse resource quantity %s: %s", value, err)
			continue
		}
		sender.Gauge(common.KubeletMetricsPrefix+string(r)+".limits", quantity.AsApproximateFloat64(), "", tagList)
	}
}

func (p *Provider) generateContainerStatusMetrics(sender sender.Sender, pod *workloadmeta.KubernetesPod, cStatus *workloadmeta.KubernetesContainerStatus, containerID types.EntityID) {
	if pod.ID == "" || pod.Name == "" {
		return
	}

	tagList, _ := p.tagger.Tag(containerID, types.OrchestratorCardinality)
	// Skip recording containers without kubelet information in tagger or if there are no tags
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	sender.Gauge(common.KubeletMetricsPrefix+"containers.restarts", float64(cStatus.RestartCount), "", tagList)

	for key, state := range map[string]workloadmeta.KubernetesContainerState{"state": cStatus.State, "last_state": cStatus.LastTerminationState} {
		if state.Terminated != nil && slices.Contains(includeContainerStateReason["terminated"], strings.ToLower(state.Terminated.Reason)) {
			termTags := utils.ConcatenateStringTags(tagList, "reason:"+strings.ToLower(state.Terminated.Reason))
			sender.Gauge(common.KubeletMetricsPrefix+"containers."+key+".terminated", 1, "", termTags)
		}
		if state.Waiting != nil && slices.Contains(includeContainerStateReason["waiting"], strings.ToLower(state.Waiting.Reason)) {
			waitTags := utils.ConcatenateStringTags(tagList, "reason:"+strings.ToLower(state.Waiting.Reason))
			sender.Gauge(common.KubeletMetricsPrefix+"containers."+key+".waiting", 1, "", waitTags)
		}
	}
}

func (p *Provider) generatePodTerminationMetric(sender sender.Sender, pod *workloadmeta.KubernetesPod) {
	// This field is set by the server when a graceful deletion is requested by the user, and is not directly settable by a client.
	// If there is no DeletionTimestamp then POD is not in Termination and no metric is needed.
	if pod.DeletionTimestamp == nil {
		return
	}

	dur := p.now().Sub(*pod.DeletionTimestamp)

	// While DeletionTimestamp is in the future metric is not emitted.
	if dur < 0 {
		return
	}

	podID := pod.ID
	if podID == "" {
		log.Debugf("skipping pod with no uid for termination metric, duration: %f", dur.Seconds())
		return
	}
	entityID := types.NewEntityID(types.KubernetesPodUID, podID)
	tagList, _ := p.tagger.Tag(entityID, types.LowCardinality)
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	sender.Gauge(common.KubeletMetricsPrefix+"pod.terminating.duration", float64(dur.Seconds()), "", tagList)
}

func (p *Provider) generatePodResizeMetric(sender sender.Sender, pod *workloadmeta.KubernetesPod) {
	var cond *workloadmeta.KubernetesPodCondition
	for _, c := range pod.Conditions {
		if c.Type == kubePodConditionResizePending {
			cond = &c
			break
		}
	}

	if cond == nil {
		// nothing to report if condition is not on the pod
		return
	}

	podID := pod.ID
	if podID == "" {
		log.Debug("skipping pod with no uid for pod resize metric")
		return
	}
	entityID := types.NewEntityID(types.KubernetesPodUID, podID)
	tagList, _ := p.tagger.Tag(entityID, types.HighCardinality)
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	// reason could be Infeasible or Deferred
	// See: https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/#pod-resize-status
	tagList = utils.ConcatenateStringTags(tagList, "reason:"+strings.ToLower(cond.Reason))

	sender.Gauge(common.KubeletMetricsPrefix+"pod.resize.pending", 1.0, "", tagList)
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

func (r *runningAggregator) recordContainer(p *Provider, pod *workloadmeta.KubernetesPod, cStatus *workloadmeta.KubernetesContainerStatus, containerID types.EntityID) {
	if cStatus.State.Running == nil || cStatus.State.Running.StartedAt.IsZero() {
		return
	}
	r.podHasRunningContainers[pod.ID] = true
	tagList, _ := p.tagger.Tag(containerID, types.LowCardinality)
	// Skip recording containers without kubelet information in tagger or if there are no tags
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	hashTags := generateTagHash(tagList)
	r.runningContainersCounter[hashTags]++
	if _, ok := r.runningContainersTags[hashTags]; !ok {
		r.runningContainersTags[hashTags] = utils.ConcatenateTags(tagList, p.config.Tags)
	}
}

func (r *runningAggregator) recordPod(p *Provider, pod *workloadmeta.KubernetesPod) {
	if !r.podHasRunningContainers[pod.ID] {
		return
	}
	podID := pod.ID
	if podID == "" {
		log.Debug("skipping pod with no uid")
		return
	}
	entityID := types.NewEntityID(types.KubernetesPodUID, podID)
	tagList, _ := p.tagger.Tag(entityID, types.LowCardinality)
	if len(tagList) == 0 {
		return
	}
	hashTags := generateTagHash(tagList)
	r.runningPodsCounter[hashTags]++
	if _, ok := r.runningPodsTags[hashTags]; !ok {
		r.runningPodsTags[hashTags] = utils.ConcatenateTags(tagList, p.config.Tags)
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

// generateTagHash creates a sorted string representation of the tags supplied. The resulting string that it returns is
// the concatenated key-value pairs of tags, sorted, which should reduce the likelihood of a hash collision that is
// possible using tagger.GetEntityHash.
//
// This is mainly used for aggregation purposes, and is here because go does not support using a slice as a map key.
// It is intended to keep the existing functionality in place after migrating this check from python.
func generateTagHash(tags []string) string {
	sortedTags := make([]string, len(tags))
	copy(sortedTags, tags)
	sort.Strings(sortedTags)
	return strings.Join(sortedTags, ",")
}

func isTagKeyPresent(key string, tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, key+":") {
			return true
		}
	}
	return false
}
