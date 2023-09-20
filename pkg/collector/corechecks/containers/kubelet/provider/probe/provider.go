// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package probe

import (
	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/prometheus"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Provider provides the metrics related to data collected from the `/metrics/probes` Kubelet endpoint
type Provider struct {
	filter *containers.Filter
	store  workloadmeta.Store
	prometheus.Provider
}

func NewProvider(filter *containers.Filter, config *common.KubeletConfig, store workloadmeta.Store) (*Provider, error) {
	provider := &Provider{
		filter: filter,
		store:  store,
	}

	transformers := prometheus.Transformers{
		"prober_probe_total": provider.proberProbeTotal,
	}

	scraperConfig := &prometheus.ScraperConfig{AllowNotFound: true}
	if config.ProbesMetricsEndpoint == nil || *config.ProbesMetricsEndpoint != "" {
		scraperConfig.Path = "/metrics/probes"
	}

	promProvider, err := prometheus.NewProvider(config, transformers, scraperConfig)
	if err != nil {
		return nil, err
	}
	provider.Provider = promProvider
	return provider, nil
}

func (p *Provider) proberProbeTotal(metric *model.Sample, sender sender.Sender) {
	metricSuffix := ""

	probeType := metric.Metric["probe_type"]
	switch probeType {
	case "Liveness":
		metricSuffix = "liveness_probe"
	case "Readiness":
		metricSuffix = "readiness_probe"
	case "Startup":
		metricSuffix = "startup_probe"
	default:
		log.Debugf("Unsupported probe type %s", probeType)
		return
	}

	result := metric.Metric["result"]
	switch result {
	case "successful":
		metricSuffix += ".success.total"
	case "failed":
		metricSuffix += ".failure.total"
	case "unknown":
		metricSuffix += ".unknown.total"
	default:
		log.Debugf("Unsupported probe result %s", result)
		return
	}

	podUid := string(metric.Metric["pod_uid"])
	containerName := string(metric.Metric["container"])
	pod, err := p.store.GetKubernetesPod(podUid)
	if err != nil {
		return
	}

	var container *workloadmeta.OrchestratorContainer
	for _, c := range pod.GetAllContainers() {
		if c.Name == containerName {
			container = &c
			break
		}
	}

	if container == nil {
		log.Debugf("container %s not found for pod with id %s", containerName, podUid)
		return
	}

	if p.filter.IsExcluded(pod.EntityMeta.Annotations, container.Name, container.Image.Name, pod.Namespace) {
		return
	}

	cId := containers.BuildTaggerEntityName(container.ID)

	tags, _ := tagger.Tag(cId, collectors.HighCardinality)
	if len(tags) == 0 {
		return
	}
	tags = utils.ConcatenateTags(tags, p.Config.Tags)

	sender.Gauge(common.KubeletMetricsPrefix+metricSuffix, float64(metric.Value), "", tags)
}
