// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

/*
Package kubelet exposes a metric provider to handle metrics exposed by the main /metrics kubelet endpoint
*/
package kubelet

import (
	"net/url"

	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/prometheus"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

var (
	defaultGauges = map[string]string{
		"rest_client_requests_total":     "rest.client.requests",
		"go_threads":                     "go_threads",
		"go_goroutines":                  "go_goroutines",
		"kubelet_pleg_last_seen_seconds": "kubelet.pleg.last_seen",
	}

	deprecatedGauges = map[string]string{
		"kubelet_runtime_operations":        "kubelet.runtime.operations",
		"kubelet_runtime_operations_errors": "kubelet.runtime.errors",
		"kubelet_docker_operations":         "kubelet.docker.operations",
		"kubelet_docker_operations_errors":  "kubelet.docker.errors",
	}

	new114Gauges = map[string]string{
		"kubelet_runtime_operations_total":        "kubelet.runtime.operations",
		"kubelet_runtime_operations_errors_total": "kubelet.runtime.errors",
	}

	defaultHistograms = map[string]string{
		"apiserver_client_certificate_expiration_seconds": "apiserver.certificate.expiration",
		"kubelet_pleg_relist_duration_seconds":            "kubelet.pleg.relist_duration",
		"kubelet_pleg_relist_interval_seconds":            "kubelet.pleg.relist_interval",
	}

	deprecatedSummaries = map[string]string{
		"kubelet_network_plugin_operations_latency_microseconds": "kubelet.network_plugin.latency",
		"kubelet_pod_start_latency_microseconds":                 "kubelet.pod.start.duration",
		"kubelet_pod_worker_latency_microseconds":                "kubelet.pod.worker.duration",
		"kubelet_pod_worker_start_latency_microseconds":          "kubelet.pod.worker.start.duration",
		"kubelet_runtime_operations_latency_microseconds":        "kubelet.runtime.operations.duration",
		"kubelet_docker_operations_latency_microseconds":         "kubelet.docker.operations.duration",
	}

	new114Summaries = map[string]string{}

	transformValuesHistogram = map[string]string{
		"kubelet_network_plugin_operations_duration_seconds": "kubelet.network_plugin.latency",
		"kubelet_pod_start_duration_seconds":                 "kubelet.pod.start.duration",
		"kubelet_pod_worker_duration_seconds":                "kubelet.pod.worker.duration",
		"kubelet_pod_worker_start_duration_seconds":          "kubelet.pod.worker.start.duration",
		"kubelet_runtime_operations_duration_seconds":        "kubelet.runtime.operations.duration",
	}

	counterMetrics = map[string]string{
		"kubelet_evictions":                          "kubelet.evictions",
		"kubelet_pleg_discard_events":                "kubelet.pleg.discard_events",
		"kubelet_cpu_manager_pinning_errors_total":   "kubelet.cpu_manager.pinning_errors_total",
		"kubelet_cpu_manager_pinning_requests_total": "kubelet.cpu_manager.pinning_requests_total",
	}

	volumeMetrics = map[string]string{
		"kubelet_volume_stats_available_bytes": "kubelet.volume.stats.available_bytes",
		"kubelet_volume_stats_capacity_bytes":  "kubelet.volume.stats.capacity_bytes",
		"kubelet_volume_stats_used_bytes":      "kubelet.volume.stats.used_bytes",
		"kubelet_volume_stats_inodes":          "kubelet.volume.stats.inodes",
		"kubelet_volume_stats_inodes_free":     "kubelet.volume.stats.inodes_free",
		"kubelet_volume_stats_inodes_used":     "kubelet.volume.stats.inodes_used",
	}
)

// Provider provides the metrics related to data collected from the `/metrics` Kubelet endpoint
type Provider struct {
	filter   *containers.Filter
	store    workloadmeta.Component
	podUtils *common.PodUtils
	tagger   tagger.Component
	prometheus.Provider
}

// NewProvider creates and returns a new Provider, configured based on the values passed in.
func NewProvider(filter *containers.Filter, config *common.KubeletConfig, store workloadmeta.Component, podUtils *common.PodUtils, tagger tagger.Component) (*Provider, error) {
	// clone instance configuration so we can set our default metrics
	kubeletConfig := *config

	kubeletConfig.Metrics = []interface{}{
		defaultGauges,
		deprecatedGauges,
		new114Gauges,
		defaultHistograms,
		deprecatedSummaries,
		new114Summaries,
	}

	provider := &Provider{
		filter:   filter,
		store:    store,
		podUtils: podUtils,
		tagger:   tagger,
	}

	transformers := prometheus.Transformers{
		"kubelet_container_log_filesystem_used_bytes": provider.kubeletContainerLogFilesystemUsedBytes,
		"rest_client_request_latency_seconds":         provider.restClientLatency,
		"rest_client_request_duration_seconds":        provider.restClientLatency,
	}
	for k := range counterMetrics {
		transformers[k] = provider.sendAlwaysCounter
	}
	for k, v := range transformValuesHistogram {
		transformers[k] = provider.HistogramFromSecondsToMicroseconds(v)
	}
	for k := range volumeMetrics {
		transformers[k] = provider.appendPodTagsToVolumeMetrics
	}

	scraperConfig := &prometheus.ScraperConfig{
		Path: "/metrics",
	}

	promProvider, err := prometheus.NewProvider(&kubeletConfig, transformers, scraperConfig)
	if err != nil {
		return nil, err
	}
	provider.Provider = promProvider
	return provider, nil
}

func (p *Provider) sendAlwaysCounter(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := metricFam.Name
	nameWithNamespace := common.KubeletMetricsPrefix + counterMetrics[metricName]

	for _, metric := range metricFam.Samples {
		tags := p.MetricTags(metric)
		sender.MonotonicCount(nameWithNamespace, float64(metric.Value), "", tags)
	}
}

func (p *Provider) appendPodTagsToVolumeMetrics(metricFam *prom.MetricFamily, sender sender.Sender) {
	// Store PV -> pod UID in cache for some amount of time in /pods provider
	// Get pod UID from cache based on PV
	// Compute tags based on pod UID (maybe these should be cached? they are cached in the python version)

	metricName := metricFam.Name
	metricNameWithNamespace := common.KubeletMetricsPrefix + volumeMetrics[metricName]
	for _, metric := range metricFam.Samples {
		pvcName := metric.Metric["persistentvolumeclaim"]
		namespace := metric.Metric["namespace"]
		if pvcName == "" || namespace == "" || p.filter.IsExcluded(nil, "", "", string(namespace)) {
			continue
		}
		tags := p.MetricTags(metric)
		if podTags := p.podUtils.GetPodTagsByPVC(string(namespace), string(pvcName)); len(podTags) > 0 {
			tags = utils.ConcatenateTags(tags, podTags)
		}
		sender.Gauge(metricNameWithNamespace, float64(metric.Value), "", tags)
	}
}

func (p *Provider) kubeletContainerLogFilesystemUsedBytes(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := common.KubeletMetricsPrefix + "kubelet.container.log_filesystem.used_bytes"
	for _, metric := range metricFam.Samples {
		cID, err := common.GetContainerID(p.store, metric.Metric, p.filter)

		if err == common.ErrContainerExcluded {
			log.Debugf("Skipping excluded container: %s/%s/%s:%s", metric.Metric["namespace"], metric.Metric["pod"], metric.Metric["container"], cID)
			continue
		}

		tags, _ := p.tagger.Tag(types.NewEntityID(types.ContainerID, cID), types.HighCardinality)
		if len(tags) == 0 {
			log.Debugf("Tags not found for container: %s/%s/%s:%s", metric.Metric["namespace"], metric.Metric["pod"], metric.Metric["container"], cID)
		}
		tags = utils.ConcatenateTags(tags, p.Config.Tags)

		sender.Gauge(metricName, float64(metric.Value), "", tags)
	}
}

func (p *Provider) restClientLatency(metricFam *prom.MetricFamily, sender sender.Sender) {
	metricName := metricFam.Name
	for _, metric := range metricFam.Samples {
		if u, ok := metric.Metric["url"]; ok {
			parsed, err := url.Parse(string(u))
			if err != nil {
				log.Errorf("Unable to parse URL %s for given metric %s: %s", u, metricName, err)
			} else if parsed != nil {
				metric.Metric["url"] = model.LabelValue(parsed.Path)
			}
		}
	}
	p.SubmitMetric(metricFam, "rest.client.latency", sender)
}
