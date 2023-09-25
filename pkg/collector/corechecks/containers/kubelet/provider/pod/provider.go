// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package pod

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	includeContainerStateReason = map[string][]string{
		"waiting": {
			"errimagepull",
			"imagepullbackoff",
			"crashloopbackoff",
			"containercreating",
			"createcontainererror",
			"invalidimagename",
		},
		"terminated": {"oomkilled", "containercannotrun", "error"},
	}
)

// Provider provides the metrics related to data collected from the `/pods` Kubelet endpoint
type Provider struct {
	filter *containers.Filter
	config *common.KubeletConfig
}

func NewProvider(filter *containers.Filter, config *common.KubeletConfig) *Provider {
	return &Provider{
		filter: filter,
		config: config,
	}
}

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
		//for _, container := range pod.Spec.Containers {
		for _, cStatus := range pod.Status.Containers {
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
			//for _, status := range pod.Status.Containers {
			for _, c := range pod.Spec.Containers {
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

func (p *Provider) generateContainerSpecMetrics(sender sender.Sender, pod *kubelet.Pod, container *kubelet.ContainerSpec, cStatus *kubelet.ContainerStatus, containerID string) {
	if pod.Status.Phase != "Running" && pod.Status.Phase != "Pending" {
		return
	}

	tags, _ := tagger.Tag(containerID, collectors.HighCardinality)
	if len(tags) == 0 {
		return
	}
	tags = utils.ConcatenateTags(tags, p.config.Tags)

	for r, value := range container.Resources.Requests {
		if v, err := resource.ParseQuantity(value); err == nil {
			sender.Gauge(common.KubeletMetricsPrefix+r+".requests", v.AsApproximateFloat64(), "", tags)
		}
	}
	for r, value := range container.Resources.Limits {
		if v, err := resource.ParseQuantity(value); err == nil {
			sender.Gauge(common.KubeletMetricsPrefix+r+".limits", v.AsApproximateFloat64(), "", tags)
		}
	}
}

func (p *Provider) generateContainerStatusMetrics(sender sender.Sender, pod *kubelet.Pod, container *kubelet.ContainerSpec, cStatus *kubelet.ContainerStatus, containerID string) {
	if pod.Metadata.UID == "" || pod.Metadata.Name == "" {
		return
	}

	tags, _ := tagger.Tag(containerID, collectors.OrchestratorCardinality)
	if len(tags) == 0 {
		return
	}
	tags = utils.ConcatenateTags(tags, p.config.Tags)

	sender.Gauge(common.KubeletMetricsPrefix+"containers.restarts", float64(cStatus.RestartCount), "", tags)

	for key, state := range map[string]kubelet.ContainerState{"state": cStatus.State, "last_state": cStatus.LastState} {
		if state.Terminated != nil && slices.Contains(includeContainerStateReason["terminated"], strings.ToLower(state.Terminated.Reason)) {
			termTags := utils.ConcatenateStringTags(tags, "reason:"+strings.ToLower(state.Terminated.Reason))
			sender.Gauge(common.KubeletMetricsPrefix+"containers."+key+".terminated", 1, "", termTags)
		}
		if state.Waiting != nil && slices.Contains(includeContainerStateReason["waiting"], strings.ToLower(state.Waiting.Reason)) {
			waitTags := utils.ConcatenateStringTags(tags, "reason:"+strings.ToLower(state.Waiting.Reason))
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

func (r *runningAggregator) recordContainer(p *Provider, pod *kubelet.Pod, cStatus *kubelet.ContainerStatus, containerID string) {
	if cStatus.State.Running == nil || time.Time.IsZero(cStatus.State.Running.StartedAt) {
		return
	}
	r.podHasRunningContainers[pod.Metadata.UID] = true
	tags, _ := tagger.Tag(containerID, collectors.LowCardinality)
	if len(tags) == 0 {
		return
	}
	hashTags := generateTagHash(tags)
	r.runningContainersCounter[hashTags]++
	if _, ok := r.runningContainersTags[hashTags]; !ok {
		r.runningContainersTags[hashTags] = utils.ConcatenateTags(tags, p.config.Tags)
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
	tags, _ := tagger.Tag(fmt.Sprintf("kubernetes_pod_uid://%s", podID), collectors.LowCardinality)
	if len(tags) == 0 {
		return
	}
	hashTags := generateTagHash(tags)
	r.runningPodsCounter[hashTags]++
	if _, ok := r.runningPodsTags[hashTags]; !ok {
		r.runningPodsTags[hashTags] = utils.ConcatenateTags(tags, p.config.Tags)
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
