// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package pod is responsible for emitting the Kubelet check metrics that are
// collected from the pod endpoints.
package pod

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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

// Provider provides the metrics related to data collected from the `/pods` Kubelet endpoint
type Provider struct {
	filter   *containers.Filter
	config   *common.KubeletConfig
	podUtils *common.PodUtils
	tagger   tagger.Component
}

// NewProvider returns a new Provider
func NewProvider(filter *containers.Filter, config *common.KubeletConfig, podUtils *common.PodUtils, tagger tagger.Component) *Provider {
	return &Provider{
		filter:   filter,
		config:   config,
		podUtils: podUtils,
		tagger:   tagger,
	}
}

// Provide provides the metrics related to a Kubelet pods
func (p *Provider) Provide(kc kubelet.KubeUtilInterface, sender sender.Sender) error {
	// Collect raw data
	pods, err := kc.GetLocalPodListWithMetadata(context.TODO())
	if err != nil {
		return err
	}
	if pods == nil {
		return nil
	}

	// Report metrics
	runningAggregator := newRunningAggregator()

	sender.Gauge(common.KubeletMetricsPrefix+"pods.expired", float64(pods.ExpiredCount), "", p.config.Tags)

	for _, pod := range pods.Items {
		p.podUtils.PopulateForPod(pod)
		// Combine regular containers with init containers for easier iteration
		allContainers := make([]kubelet.ContainerSpec, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
		allContainers = append(allContainers, pod.Spec.InitContainers...)
		allContainers = append(allContainers, pod.Spec.Containers...)

		for _, cStatus := range pod.Status.AllContainers {
			if cStatus.ID == "" {
				// no container ID means we could not find the matching container status for this container, which will make fetching tags difficult.
				continue
			}
			cID, err := kubelet.KubeContainerIDToTaggerEntityID(cStatus.ID)
			if err != nil {
				// could not correctly parse container ID
				continue
			}

			var container kubelet.ContainerSpec
			for _, c := range allContainers {
				if cStatus.Name == c.Name {
					container = c
					break
				}
			}
			runningAggregator.recordContainer(p, pod, &cStatus, cID)

			// don't exclude filtered containers from aggregation, but filter them out from other reported metrics
			if p.filter.IsExcluded(pod.Metadata.Annotations, cStatus.Name, cStatus.Image, pod.Metadata.Namespace) {
				continue
			}

			p.generateContainerSpecMetrics(sender, pod, &container, &cStatus, cID)
			p.generateContainerStatusMetrics(sender, pod, &container, &cStatus, cID)
		}
		runningAggregator.recordPod(p, pod)
	}
	runningAggregator.generateRunningAggregatorMetrics(sender)

	return nil
}

func (p *Provider) generateContainerSpecMetrics(sender sender.Sender, pod *kubelet.Pod, container *kubelet.ContainerSpec, cStatus *kubelet.ContainerStatus, containerID types.EntityID) {
	if pod.Status.Phase != "Running" && pod.Status.Phase != "Pending" {
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

	for r, value := range container.Resources.Requests {
		sender.Gauge(common.KubeletMetricsPrefix+string(r)+".requests", value.AsApproximateFloat64(), "", tagList)
	}
	for r, value := range container.Resources.Limits {
		sender.Gauge(common.KubeletMetricsPrefix+string(r)+".limits", value.AsApproximateFloat64(), "", tagList)
	}
}

func (p *Provider) generateContainerStatusMetrics(sender sender.Sender, pod *kubelet.Pod, _ *kubelet.ContainerSpec, cStatus *kubelet.ContainerStatus, containerID types.EntityID) {
	if pod.Metadata.UID == "" || pod.Metadata.Name == "" {
		return
	}

	tagList, _ := p.tagger.Tag(containerID, types.OrchestratorCardinality)
	// Skip recording containers without kubelet information in tagger or if there are no tags
	if !isTagKeyPresent(kubeNamespaceTag, tagList) || len(tagList) == 0 {
		return
	}
	tagList = utils.ConcatenateTags(tagList, p.config.Tags)

	sender.Gauge(common.KubeletMetricsPrefix+"containers.restarts", float64(cStatus.RestartCount), "", tagList)

	for key, state := range map[string]kubelet.ContainerState{"state": cStatus.State, "last_state": cStatus.LastState} {
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

func (r *runningAggregator) recordContainer(p *Provider, pod *kubelet.Pod, cStatus *kubelet.ContainerStatus, containerID types.EntityID) {
	if cStatus.State.Running == nil || time.Time.IsZero(cStatus.State.Running.StartedAt) {
		return
	}
	r.podHasRunningContainers[pod.Metadata.UID] = true
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

func (r *runningAggregator) recordPod(p *Provider, pod *kubelet.Pod) {
	if !r.podHasRunningContainers[pod.Metadata.UID] {
		return
	}
	podID := pod.Metadata.UID
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
