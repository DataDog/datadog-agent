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
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// Suffixes per:
	// https://github.com/kubernetes/kubernetes/blob/8fd414537b5143ab039cb910590237cabf4af783/pkg/api/resource/suffix.go#L108
	factors = map[string]float64{
		"n":  float64(1) / (1000 * 1000 * 1000),
		"u":  float64(1) / (1000 * 1000),
		"m":  float64(1) / 1000,
		"k":  1000,
		"M":  1000 * 1000,
		"G":  1000 * 1000 * 1000,
		"T":  1000 * 1000 * 1000 * 1000,
		"P":  1000 * 1000 * 1000 * 1000 * 1000,
		"E":  1000 * 1000 * 1000 * 1000 * 1000 * 1000,
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"Pi": 1024 * 1024 * 1024 * 1024 * 1024,
		"Ei": 1024 * 1024 * 1024 * 1024 * 1024 * 1024,
	}

	whitelistedContainerStateReason = map[string][]string{
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

func (p *Provider) Collect(kc kubelet.KubeUtilInterface) (interface{}, error) {
	return kc.GetLocalPodListWithMetadata(context.TODO())
}

func (p *Provider) Transform(podList interface{}, sender aggregator.Sender) error {
	pods := podList.(*kubelet.PodList)
	if pods == nil {
		return nil
	}

	runningAggregator := newRunningAggregator()

	sender.Gauge(common.KubeletMetricsPrefix+"pods.expired", float64(pods.ExpiredCount), "", p.config.Tags)

	for _, pod := range pods.Items {
		//for _, container := range pod.Spec.Containers {
		for _, cStatus := range pod.Status.Containers {
			if cStatus.ID == "" {
				// no container ID means we could not find the matching container status for this container, which will make fetching tags difficult.
				continue
			}
			cId, err := kubelet.KubeContainerIDToTaggerEntityID(cStatus.ID)
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
			runningAggregator.recordContainer(p, pod, &cStatus, cId)

			// don't exclude filtered containers from aggregation, but filter them out from other reported metrics
			if p.filter.IsExcluded(pod.Metadata.Annotations, cStatus.Name, cStatus.Image, pod.Metadata.Namespace) {
				continue
			}

			p.generateContainerSpecMetrics(sender, pod, &container, &cStatus, cId)
			p.generateContainerStatusMetrics(sender, pod, &container, &cStatus, cId)
		}
		runningAggregator.recordPod(p, pod)
	}
	runningAggregator.generateRunningAggregatorMetrics(sender)

	return nil
}

func (p *Provider) generateContainerSpecMetrics(sender aggregator.Sender, pod *kubelet.Pod, container *kubelet.ContainerSpec, cStatus *kubelet.ContainerStatus, containerId string) {
	if pod.Status.Phase != "Running" && pod.Status.Phase != "Pending" {
		return
	}

	tags, _ := tagger.Tag(containerId, collectors.HighCardinality)
	if len(tags) == 0 {
		return
	}
	tags = utils.ConcatenateTags(tags, p.config.Tags)

	for resource, value := range container.Resources.Requests {
		if v, err := parseQuantity(value); err == nil {
			sender.Gauge(common.KubeletMetricsPrefix+resource+".requests", v, "", tags)
		}
	}
	for resource, value := range container.Resources.Limits {
		if v, err := parseQuantity(value); err == nil {
			sender.Gauge(common.KubeletMetricsPrefix+resource+".limits", v, "", tags)
		}
	}
}

func (p *Provider) generateContainerStatusMetrics(sender aggregator.Sender, pod *kubelet.Pod, container *kubelet.ContainerSpec, cStatus *kubelet.ContainerStatus, containerId string) {
	if pod.Metadata.UID == "" || pod.Metadata.Name == "" {
		return
	}

	tags, _ := tagger.Tag(containerId, collectors.OrchestratorCardinality)
	if len(tags) == 0 {
		return
	}
	tags = utils.ConcatenateTags(tags, p.config.Tags)

	sender.Gauge(common.KubeletMetricsPrefix+"containers.restarts", float64(cStatus.RestartCount), "", tags)

	for key, state := range map[string]kubelet.ContainerState{"state": cStatus.State, "last_state": cStatus.LastState} {
		if state.Terminated != nil && slices.Contains(whitelistedContainerStateReason["terminated"], strings.ToLower(state.Terminated.Reason)) {
			termTags := utils.ConcatenateStringTags(tags, "reason:"+strings.ToLower(state.Terminated.Reason))
			sender.Gauge(common.KubeletMetricsPrefix+"containers."+key+".terminated", 1, "", termTags)
		}
		if state.Waiting != nil && slices.Contains(whitelistedContainerStateReason["waiting"], strings.ToLower(state.Waiting.Reason)) {
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

func (r *runningAggregator) recordContainer(p *Provider, pod *kubelet.Pod, cStatus *kubelet.ContainerStatus, containerId string) {
	if cStatus.State.Running == nil || time.Time.IsZero(cStatus.State.Running.StartedAt) {
		return
	}
	r.podHasRunningContainers[pod.Metadata.UID] = true
	tags, _ := tagger.Tag(containerId, collectors.LowCardinality)
	if len(tags) == 0 {
		return
	}
	hashTags := generateTagHash(tags)
	r.runningContainersCounter[hashTags] += 1
	if _, ok := r.runningContainersTags[hashTags]; !ok {
		r.runningContainersTags[hashTags] = utils.ConcatenateTags(tags, p.config.Tags)
	}
}

func (r *runningAggregator) recordPod(p *Provider, pod *kubelet.Pod) {
	if !r.podHasRunningContainers[pod.Metadata.UID] {
		return
	}
	podId := pod.Metadata.UID
	if podId == "" {
		log.Debug("skipping pod with no uid")
		return
	}
	tags, _ := tagger.Tag(fmt.Sprintf("kubernetes_pod_uid://%s", podId), collectors.LowCardinality)
	if len(tags) == 0 {
		return
	}
	hashTags := generateTagHash(tags)
	r.runningPodsCounter[hashTags] += 1
	if _, ok := r.runningPodsTags[hashTags]; !ok {
		r.runningPodsTags[hashTags] = utils.ConcatenateTags(tags, p.config.Tags)
	}
}

func (r *runningAggregator) generateRunningAggregatorMetrics(sender aggregator.Sender) {
	for hash, count := range r.runningContainersCounter {
		sender.Gauge(common.KubeletMetricsPrefix+"containers.running", count, "", r.runningContainersTags[hash])
	}
	for hash, count := range r.runningPodsCounter {
		sender.Gauge(common.KubeletMetricsPrefix+"pods.running", count, "", r.runningPodsTags[hash])
	}
}

func parseQuantity(value string) (float64, error) {
	var number, unit string
	for _, char := range value {
		if unicode.IsDigit(char) || char == '.' {
			number += string(char)
		} else {
			unit += string(char)
		}
	}

	convertedNumber, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, err
	}

	if factor, ok := factors[unit]; ok {
		return convertedNumber * factor, nil
	}
	return convertedNumber, nil
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
