// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"net/url"

	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/prometheus"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var (
	DEFAULT_GAUGES = map[string]string{
		"rest_client_requests_total":     "rest.client.requests",
		"go_threads":                     "go_threads",
		"go_goroutines":                  "go_goroutines",
		"kubelet_pleg_last_seen_seconds": "kubelet.pleg.last_seen",
	}

	DEPRECATED_GAUGES = map[string]string{
		"kubelet_runtime_operations":        "kubelet.runtime.operations",
		"kubelet_runtime_operations_errors": "kubelet.runtime.errors",
		"kubelet_docker_operations":         "kubelet.docker.operations",
		"kubelet_docker_operations_errors":  "kubelet.docker.errors",
	}

	NEW_1_14_GAUGES = map[string]string{
		"kubelet_runtime_operations_total":        "kubelet.runtime.operations",
		"kubelet_runtime_operations_errors_total": "kubelet.runtime.errors",
	}

	DEFAULT_HISTOGRAMS = map[string]string{
		"apiserver_client_certificate_expiration_seconds": "apiserver.certificate.expiration",
		"kubelet_pleg_relist_duration_seconds":            "kubelet.pleg.relist_duration",
		"kubelet_pleg_relist_interval_seconds":            "kubelet.pleg.relist_interval",
	}

	DEFAULT_SUMMARIES = map[string]string{}

	DEPRECATED_SUMMARIES = map[string]string{
		"kubelet_network_plugin_operations_latency_microseconds": "kubelet.network_plugin.latency",
		"kubelet_pod_start_latency_microseconds":                 "kubelet.pod.start.duration",
		"kubelet_pod_worker_latency_microseconds":                "kubelet.pod.worker.duration",
		"kubelet_pod_worker_start_latency_microseconds":          "kubelet.pod.worker.start.duration",
		"kubelet_runtime_operations_latency_microseconds":        "kubelet.runtime.operations.duration",
		"kubelet_docker_operations_latency_microseconds":         "kubelet.docker.operations.duration",
	}

	NEW_1_14_SUMMARIES = map[string]string{}

	TRANSFORM_VALUE_HISTOGRAMS = map[string]string{
		"kubelet_network_plugin_operations_duration_seconds": "kubelet.network_plugin.latency",
		"kubelet_pod_start_duration_seconds":                 "kubelet.pod.start.duration",
		"kubelet_pod_worker_duration_seconds":                "kubelet.pod.worker.duration",
		"kubelet_pod_worker_start_duration_seconds":          "kubelet.pod.worker.start.duration",
		"kubelet_runtime_operations_duration_seconds":        "kubelet.runtime.operations.duration",
	}

	DEFAULT_METRIC_LIMIT = 0

	COUNTER_METRICS = map[string]string{
		"kubelet_evictions":           "kubelet.evictions",
		"kubelet_pleg_discard_events": "kubelet.pleg.discard_events",
	}

	VOLUME_METRICS = map[string]string{
		"kubelet_volume_stats_available_bytes": "kubelet.volume.stats.available_bytes",
		"kubelet_volume_stats_capacity_bytes":  "kubelet.volume.stats.capacity_bytes",
		"kubelet_volume_stats_used_bytes":      "kubelet.volume.stats.used_bytes",
		"kubelet_volume_stats_inodes":          "kubelet.volume.stats.inodes",
		"kubelet_volume_stats_inodes_free":     "kubelet.volume.stats.inodes_free",
		"kubelet_volume_stats_inodes_used":     "kubelet.volume.stats.inodes_used",
	}

	VOLUME_TAG_KEYS_TO_EXCLUDE = []string{"persistentvolumeclaim", "pod_phase"}
)

// Provider provides the metrics related to data collected from the `/metrics` Kubelet endpoint
type Provider struct {
	filter *containers.Filter
	store  workloadmeta.Store
	prometheus.Provider
}

func NewProvider(filter *containers.Filter, config *common.KubeletConfig, store workloadmeta.Store) (*Provider, error) {
	// clone instance configuration so we can set our default metrics
	kubeletConfig := *config

	kubeletConfig.Metrics = []interface{}{
		DEFAULT_GAUGES,
		DEPRECATED_GAUGES,
		NEW_1_14_GAUGES,
		DEFAULT_HISTOGRAMS,
		DEFAULT_SUMMARIES,
		DEPRECATED_SUMMARIES,
		NEW_1_14_SUMMARIES,
	}

	provider := &Provider{
		filter: filter,
		store:  store,
	}

	transformers := prometheus.Transformers{
		"kubelet_container_log_filesystem_used_bytes": provider.kubeletContainerLogFilesystemUsedBytes,
		"rest_client_request_latency_seconds":         provider.restClientLatency,
		"rest_client_request_duration_seconds":        provider.restClientLatency,
	}
	for k := range COUNTER_METRICS {
		transformers[k] = provider.sendAlwaysCounter
	}
	for k, v := range TRANSFORM_VALUE_HISTOGRAMS {
		transformers[k] = provider.HistogramFromSecondsToMicroseconds(v)
	}
	for k := range VOLUME_METRICS {
		transformers[k] = provider.appendPodTagsToVolumeMetrics
	}

	scraperConfig := &prometheus.ScraperConfig{}
	// TODO probes
	if kubeletConfig.ProbesMetricsEndpoint == nil || *kubeletConfig.ProbesMetricsEndpoint != "" {
		scraperConfig.Path = "/metrics"
	}

	promProvider, err := prometheus.NewProvider(&kubeletConfig, transformers, scraperConfig)
	if err != nil {
		return nil, err
	}
	provider.Provider = promProvider
	return provider, nil
}

func (p *Provider) sendAlwaysCounter(metric *model.Sample, sender sender.Sender) {
	metricName := string(metric.Metric["__name__"])
	nameWithNamespace := p.Config.Namespace + "." + COUNTER_METRICS[metricName]

	tags := p.MetricTags(metric)
	sender.MonotonicCount(nameWithNamespace, float64(metric.Value), "", tags)
}

func (p *Provider) appendPodTagsToVolumeMetrics(metric *model.Sample, sender sender.Sender) {
	// TODO
}

func (p *Provider) kubeletContainerLogFilesystemUsedBytes(metric *model.Sample, sender sender.Sender) {
	// TODO
}

func (p *Provider) restClientLatency(metric *model.Sample, sender sender.Sender) {
	metricName := string(metric.Metric["__name__"])
	if u, ok := metric.Metric["url"]; ok {
		parsed, err := url.Parse(string(u))
		if err != nil {
			log.Errorf("Unable to parse URL %s for given metric %s: %s", u, metricName, err)
		} else if parsed != nil {
			metric.Metric["url"] = model.LabelValue(parsed.Path)
		}
	}
	p.SubmitMetric(metric, "rest.client.latency", sender)
}
