// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

/*
Package cadvisor exposes a metric provider to handle metrics exposed by the /metrics/cadvisor kubelet endpoint
*/
package cadvisor

import (
	"fmt"
	"math"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/prometheus"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

var (
	ignoreMetrics = []string{
		"container_fs_inodes_free",
		"container_fs_inodes_total",
		"container_fs_io_current",
		"container_fs_io_time_seconds_total",
		"container_fs_io_time_weighted_seconds_total",
		"container_fs_read_seconds_total",
		"container_fs_reads_merged_total",
		"container_fs_reads_total",
		"container_fs_sector_reads_total",
		"container_fs_sector_writes_total",
		"container_fs_write_seconds_total",
		"container_fs_writes_merged_total",
		"container_fs_writes_total",
		"container_last_seen",
		"container_start_time_seconds",
		"container_scrape_error",
	}

	metricTypes = map[string]struct{}{
		"COUNTER": {},
		"GAUGE":   {},
		"SUMMARY": {},
	}

	// container-specific metrics should have all these labels
	pre116ContainerLabels  = []string{"namespace", "name", "image", "id", "container_name", "pod_name"}
	post116ContainerLabels = []string{"namespace", "name", "image", "id", "container", "pod"}

	maxMemoryRss = math.Pow(2, 63)
)

type uidFromLabelsFunc func(model.Metric) string

type processCache struct {
	value float64
	tags  []string
}

// Provider provides the metrics related to data collected from the `/metrics/cadvisor` Kubelet endpoint
type Provider struct {
	filter         *containers.Filter
	store          workloadmeta.Component
	tagger         tagger.Component
	podUtils       *common.PodUtils
	fsUsageBytes   map[string]*processCache
	memUsageBytes  map[string]*processCache
	swapUsageBytes map[string]*processCache
	prometheus.Provider
}

// NewProvider creates and returns a new Provider, configured based on the values passed in.
func NewProvider(filter *containers.Filter, config *common.KubeletConfig, store workloadmeta.Component, podUtils *common.PodUtils, tagger tagger.Component) (*Provider, error) {
	// clone instance configuration so we can set our default metrics
	cadvisorConfig := *config

	cadvisorConfig.IgnoreMetrics = ignoreMetrics

	provider := &Provider{
		filter:         filter,
		store:          store,
		tagger:         tagger,
		podUtils:       podUtils,
		fsUsageBytes:   map[string]*processCache{},
		memUsageBytes:  map[string]*processCache{},
		swapUsageBytes: map[string]*processCache{},
	}

	transformers := prometheus.Transformers{
		"container_cpu_usage_seconds_total":                provider.containerCPUUsageSecondsTotal,
		"container_cpu_load_average_10s":                   provider.containerCPULoadAverage10s,
		"container_cpu_system_seconds_total":               provider.containerCPUSystemSecondsTotal,
		"container_cpu_user_seconds_total":                 provider.containerCPUUserSecondsTotal,
		"container_cpu_cfs_periods_total":                  provider.containerCPUCfsPeriodsTotal,
		"container_cpu_cfs_throttled_periods_total":        provider.containerCPUCfsThrottledPeriodsTotal,
		"container_cpu_cfs_throttled_seconds_total":        provider.containerCPUCfsThrottledSecondsTotal,
		"container_fs_reads_bytes_total":                   provider.containerFsReadsBytesTotal,
		"container_fs_writes_bytes_total":                  provider.containerFsWritesBytesTotal,
		"container_network_receive_bytes_total":            provider.containerNetworkReceiveBytesTotal,
		"container_network_transmit_bytes_total":           provider.containerNetworkTransmitBytesTotal,
		"container_network_receive_errors_total":           provider.containerNetworkReceiveErrorsTotal,
		"container_network_transmit_errors_total":          provider.containerNetworkTransmitErrorsTotal,
		"container_network_transmit_packets_dropped_total": provider.containerNetworkTransmitPacketsDroppedTotal,
		"container_network_receive_packets_dropped_total":  provider.containerNetworkReceivePacketsDroppedTotal,
		"container_fs_usage_bytes":                         provider.containerFsUsageBytes,
		"container_fs_limit_bytes":                         provider.containerFsLimitBytes,
		"container_memory_usage_bytes":                     provider.containerMemoryUsageBytes,
		"container_memory_working_set_bytes":               provider.containerMemoryWorkingSetBytes,
		"container_memory_cache":                           provider.containerMemoryCache,
		"container_memory_rss":                             provider.containerMemoryRss,
		"container_memory_swap":                            provider.containerMemorySwap,
		"container_spec_memory_limit_bytes":                provider.containerSpecMemoryLimitBytes,
		"container_spec_memory_swap_limit_bytes":           provider.containerSpecMemorySwapLimitBytes,
	}

	scraperConfig := &prometheus.ScraperConfig{
		Path:                "/metrics/cadvisor",
		TextFilterBlacklist: []string{"pod_name=\"\"", "pod=\"\""},
	}

	promProvider, err := prometheus.NewProvider(&cadvisorConfig, transformers, scraperConfig)
	if err != nil {
		return nil, err
	}
	provider.Provider = promProvider
	return provider, nil
}

func (p *Provider) processContainerMetric(metricType, metricName string, metricFam *prom.MetricFamily, labels []string, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}

	samples := p.latestValueByContext(metricFam, p.getEntityIDIfContainerMetric)
	for containerID, sample := range samples {
		var tags []string

		// TODO we are forced to do that because the Kubelet PodList isn't updated
		//      for static pods, see https://github.com/kubernetes/kubernetes/pull/59948
		pod := p.getPodByMetricLabel(sample.Metric)
		if pod != nil && p.podUtils.IsStaticPendingPod(pod.ID) {
			podTags, _ := p.tagger.Tag(taggercommon.BuildTaggerEntityID(pod.GetID()), types.HighCardinality)
			if len(podTags) == 0 {
				continue
			}
			containerName := p.getKubeContainerNameTag(sample.Metric)
			if containerName != "" {
				podTags = append(podTags, containerName)
			}
			tags = podTags
		} else {
			cID, _ := kubelet.KubeContainerIDToTaggerEntityID(containerID)
			tags, _ = p.tagger.Tag(cID, types.HighCardinality)
		}

		if len(tags) == 0 {
			continue
		}
		tags = utils.ConcatenateTags(tags, p.Config.Tags)

		for _, label := range labels {
			if value, ok := sample.Metric[model.LabelName(label)]; ok {
				tags = append(tags, fmt.Sprintf("%s:%s", label, value))
			}
		}

		switch metricType {
		case "rate":
			sender.Rate(metricName, float64(sample.Value), "", tags)
		case "gauge":
			sender.Gauge(metricName, float64(sample.Value), "", tags)
		default:
			log.Debugf("Unsupported metric type %s for metric %s", metricType, metricName)
		}
	}
}

func (p *Provider) processPodRate(metricName string, metricFam *prom.MetricFamily, labels []string, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}

	samples := p.sumValuesByContext(metricFam, p.getPodUIDIfPodMetric)
	for podUID, sample := range samples {
		pod := p.getPodByUID(podUID)
		if pod == nil {
			continue
		}
		namespace := pod.Namespace
		if p.filter.IsExcluded(pod.Annotations, "", "", namespace) {
			continue
		}
		if strings.Contains(metricName, ".network.") && p.podUtils.IsHostNetworkedPod(podUID) {
			continue
		}
		entityID := taggercommon.BuildTaggerEntityID(pod.GetID())
		tags, _ := p.tagger.Tag(entityID, types.HighCardinality)
		if len(tags) == 0 {
			continue
		}
		tags = utils.ConcatenateTags(tags, p.Config.Tags)

		for _, label := range labels {
			if value, ok := sample.Metric[model.LabelName(label)]; ok {
				tags = append(tags, fmt.Sprintf("%s:%s", label, value))
			}
		}

		sender.Rate(metricName, float64(sample.Value), "", tags)
	}
}

func (p *Provider) processUsageMetric(metricName string, metricFam *prom.MetricFamily, cache map[string]*processCache, labels []string, sender sender.Sender) {
	// track containers that still exist in the cache
	seenKeys := map[string]bool{}
	for k := range cache {
		seenKeys[k] = false
	}

	samples := p.sumValuesByContext(metricFam, p.getEntityIDIfContainerMetric)
	for containerID, sample := range samples {
		containerName := string(sample.Metric["name"])
		if containerName == "" {
			continue
		}

		cID, _ := kubelet.KubeContainerIDToTaggerEntityID(containerID)
		tags, _ := p.tagger.Tag(cID, types.HighCardinality)
		if len(tags) == 0 {
			continue
		}
		tags = utils.ConcatenateTags(tags, p.Config.Tags)

		// TODO we are forced to do that because the Kubelet PodList isn"t updated
		//      for static pods, see https://github.com/kubernetes/kubernetes/pull/59948
		pod := p.getPodByMetricLabel(sample.Metric)
		if pod != nil && p.podUtils.IsStaticPendingPod(pod.ID) {
			entityID := taggercommon.BuildTaggerEntityID(pod.EntityID)
			podTags, _ := p.tagger.Tag(entityID, types.HighCardinality)
			if len(podTags) == 0 {
				continue
			}
			containerNameTag := p.getKubeContainerNameTag(sample.Metric)
			if containerNameTag != "" {
				podTags = append(podTags, containerNameTag)
			}
			tags = utils.ConcatenateTags(tags, podTags)
		}

		for _, label := range labels {
			if value, ok := sample.Metric[model.LabelName(label)]; ok {
				tags = append(tags, fmt.Sprintf("%s:%s", label, value))
			}
		}

		cache[containerName] = &processCache{
			value: float64(sample.Value),
			tags:  tags,
		}
		seenKeys[containerName] = true

		sender.Gauge(metricName, float64(sample.Value), "", tags)
	}

	for k, seen := range seenKeys {
		if !seen {
			delete(cache, k)
		}
	}
}

func (p *Provider) processLimitMetric(metricName string, metricFam *prom.MetricFamily, cache map[string]*processCache, pctMetricName string, sender sender.Sender) {
	samples := p.latestValueByContext(metricFam, p.getEntityIDIfContainerMetric)
	for containerID, sample := range samples {
		cID, _ := kubelet.KubeContainerIDToTaggerEntityID(containerID)
		tags, _ := p.tagger.Tag(cID, types.HighCardinality)
		if len(tags) == 0 {
			continue
		}
		tags = utils.ConcatenateTags(tags, p.Config.Tags)

		if metricName != "" {
			sender.Gauge(metricName, float64(sample.Value), "", tags)
		}

		if pctMetricName != "" && sample.Value > 0 {
			containerName := string(sample.Metric["name"])
			if containerName == "" {
				continue
			}

			if cached, ok := cache[containerName]; ok {
				sender.Gauge(pctMetricName, cached.value/float64(sample.Value), "", cached.tags)
			} else {
				log.Debugf("No corresponding usage found for metric %s and container %s, skipping usage_pct for now.", pctMetricName, containerName)
			}
		}
	}
}

func (p *Provider) sumValuesByContext(metricFam *prom.MetricFamily, uidFromLabelsFunc uidFromLabelsFunc) map[string]*model.Sample {
	seen := map[string]*model.Sample{}
	for _, sample := range metricFam.Samples {
		uid := uidFromLabelsFunc(sample.Metric)
		if uid == "" {
			continue
		}
		// Sum the counter value across all contexts
		if _, ok := seen[uid]; !ok {
			seen[uid] = sample
		} else {
			seen[uid].Value += sample.Value
		}
	}

	return seen
}

func (p *Provider) latestValueByContext(metricFam *prom.MetricFamily, uidFromLabelsFunc uidFromLabelsFunc) map[string]*model.Sample {
	seen := map[string]*model.Sample{}
	for _, sample := range metricFam.Samples {
		uid := uidFromLabelsFunc(sample.Metric)
		if uid == "" {
			continue
		}
		seen[uid] = sample
	}

	return seen
}

func (p *Provider) getEntityIDIfContainerMetric(labels model.Metric) string {
	if isContainerMetric(labels) {
		pod := p.getPodByMetricLabel(labels)
		if pod != nil && p.podUtils.IsStaticPendingPod(pod.ID) {
			// If the pod is static, ContainerStatus is unavailable.
			// Return the pod UID so that we can collect metrics from it later on.
			return p.getPodUID(labels)
		}
		cID, _ := common.GetContainerID(p.store, labels, p.filter)
		return types.NewEntityID(types.ContainerID, cID).String()
	}
	return ""
}

func (p *Provider) getPodUIDIfPodMetric(labels model.Metric) string {
	if isPodMetric(labels) {
		return p.getPodUID(labels)
	}
	return ""
}

func (p *Provider) getPodUID(labels model.Metric) string {
	if pod := p.getPodByMetricLabel(labels); pod != nil {
		return pod.ID
	}
	return ""
}

func (p *Provider) getPodByMetricLabel(labels model.Metric) *workloadmeta.KubernetesPod {
	namespace := labels["namespace"]
	podName, ok := labels["pod"]
	if !ok {
		podName = labels["pod_name"]
	}
	if pod, err := p.store.GetKubernetesPodByName(string(podName), string(namespace)); err == nil {
		if !p.filter.IsExcluded(pod.EntityMeta.Annotations, "", "", pod.Namespace) {
			return pod
		}
	}
	return nil
}

func (p *Provider) getContainerName(labels model.Metric) string {
	containerName := labels["container"]
	if containerName == "" {
		containerName = labels["container_name"]
	}
	return string(containerName)
}

func (p *Provider) getKubeContainerNameTag(labels model.Metric) string {
	containerName := p.getContainerName(labels)
	if containerName != "" {
		return fmt.Sprintf("kube_container_name:%s", containerName)
	}
	return ""
}

func (p *Provider) containerCPUUsageSecondsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.usage.total"
	for i := range metricFam.Samples {
		// Replace sample value to convert cores to nano cores
		metricFam.Samples[i].Value *= model.SampleValue(math.Pow10(9))
	}
	p.processContainerMetric("rate", metricName, metricFam, nil, sender)
}

func (p *Provider) containerCPULoadAverage10s(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.load.10s.avg"
	p.processContainerMetric("gauge", metricName, metricFam, nil, sender)
}

func (p *Provider) containerCPUSystemSecondsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.system.total"
	p.processContainerMetric("rate", metricName, metricFam, nil, sender)
}

func (p *Provider) containerCPUUserSecondsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.user.total"
	p.processContainerMetric("rate", metricName, metricFam, nil, sender)
}

func (p *Provider) containerCPUCfsPeriodsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.cfs.periods"
	p.processContainerMetric("rate", metricName, metricFam, nil, sender)
}

func (p *Provider) containerCPUCfsThrottledPeriodsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.cfs.throttled.periods"
	p.processContainerMetric("rate", metricName, metricFam, nil, sender)
}

func (p *Provider) containerCPUCfsThrottledSecondsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "cpu.cfs.throttled.seconds"
	p.processContainerMetric("rate", metricName, metricFam, nil, sender)
}

func (p *Provider) containerFsReadsBytesTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "io.read_bytes"
	labels := []string{"device"}
	p.processContainerMetric("rate", metricName, metricFam, labels, sender)
}

func (p *Provider) containerFsWritesBytesTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "io.write_bytes"
	labels := []string{"device"}
	p.processContainerMetric("rate", metricName, metricFam, labels, sender)
}

func (p *Provider) containerNetworkReceiveBytesTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "network.rx_bytes"
	labels := []string{"interface"}
	p.processPodRate(metricName, metricFam, labels, sender)
}

func (p *Provider) containerNetworkTransmitBytesTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "network.tx_bytes"
	labels := []string{"interface"}
	p.processPodRate(metricName, metricFam, labels, sender)
}

func (p *Provider) containerNetworkReceiveErrorsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "network.rx_errors"
	labels := []string{"interface"}
	p.processPodRate(metricName, metricFam, labels, sender)
}

func (p *Provider) containerNetworkTransmitErrorsTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "network.tx_errors"
	labels := []string{"interface"}
	p.processPodRate(metricName, metricFam, labels, sender)
}

func (p *Provider) containerNetworkTransmitPacketsDroppedTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "network.tx_dropped"
	labels := []string{"interface"}
	p.processPodRate(metricName, metricFam, labels, sender)
}

func (p *Provider) containerNetworkReceivePacketsDroppedTotal(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "network.rx_dropped"
	labels := []string{"interface"}
	p.processPodRate(metricName, metricFam, labels, sender)
}

func (p *Provider) containerFsUsageBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}
	metricName := common.KubeletMetricsPrefix + "filesystem.usage"
	labels := []string{"device"}
	p.processUsageMetric(metricName, metricFam, p.fsUsageBytes, labels, sender)
}

func (p *Provider) containerFsLimitBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}
	pctName := common.KubeletMetricsPrefix + "filesystem.usage_pct"
	p.processLimitMetric("", metricFam, p.fsUsageBytes, pctName, sender)
}

func (p *Provider) containerMemoryUsageBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}
	metricName := common.KubeletMetricsPrefix + "memory.usage"
	p.processUsageMetric(metricName, metricFam, p.memUsageBytes, nil, sender)
}

func (p *Provider) containerMemoryWorkingSetBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "memory.working_set"
	p.processContainerMetric("gauge", metricName, metricFam, nil, sender)
}

func (p *Provider) containerMemoryCache(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "memory.cache"
	p.processContainerMetric("gauge", metricName, metricFam, nil, sender)
}

func (p *Provider) containerMemoryRss(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "memory.rss"
	// Filter out aberrant values
	filteredSamples := model.Vector{}
	for i := range metricFam.Samples {
		if metricFam.Samples[i].Value < model.SampleValue(maxMemoryRss) {
			filteredSamples = append(filteredSamples, metricFam.Samples[i])
		}
	}
	metricFam.Samples = filteredSamples
	p.processContainerMetric("gauge", metricName, metricFam, nil, sender)
}

func (p *Provider) containerMemorySwap(metricFam *prom.MetricFamily, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}
	metricName := common.KubeletMetricsPrefix + "memory.swap"
	p.processUsageMetric(metricName, metricFam, p.swapUsageBytes, nil, sender)
}

func (p *Provider) containerSpecMemoryLimitBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}
	metricName := common.KubeletMetricsPrefix + "memory.limits"
	pctName := common.KubeletMetricsPrefix + "memory.usage_pct"
	p.processLimitMetric(metricName, metricFam, p.memUsageBytes, pctName, sender)
}

func (p *Provider) containerSpecMemorySwapLimitBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	if _, ok := metricTypes[metricFam.Type]; !ok {
		log.Errorf("Metric type %s unsupported for metric %s", metricFam.Type, metricFam.Name)
		return
	}
	metricName := common.KubeletMetricsPrefix + "memory.sw_limit"
	pctName := common.KubeletMetricsPrefix + "memory.sw_in_use"
	p.processLimitMetric(metricName, metricFam, p.swapUsageBytes, pctName, sender)
}

func (p *Provider) getPodByUID(podUID string) *workloadmeta.KubernetesPod {
	if pod, err := p.store.GetKubernetesPod(podUID); err == nil {
		return pod
	}
	return nil
}

func isContainerMetric(labels model.Metric) bool {
	if metricContainsLabels(labels, post116ContainerLabels) {
		return labels["container"] != "" && labels["container"] != "POD"
	} else if metricContainsLabels(labels, pre116ContainerLabels) {
		return labels["container_name"] != "" && labels["container_name"] != "POD"
	}
	return false
}

func isPodMetric(labels model.Metric) bool {
	return labels["container"] == "POD" ||
		labels["container_name"] == "POD" ||
		(labels["container"] == "" && labels["pod"] != "") ||
		(labels["container_name"] == "" && labels["pod_name"] != "")
}

func metricContainsLabels(metric model.Metric, labels []string) bool {
	for _, label := range labels {
		if _, ok := metric[model.LabelName(label)]; !ok {
			return false
		}
	}
	return true
}
