// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package slis is responsible for emitting the Kubelet check metrics that are
// collected from the `/metrics/slis` endpoint.
package slis

import (
	"strings"

	"github.com/samber/lo"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/provider/prometheus"
	prom "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// Provider provides the metrics related to data collected from the `/metrics/slis` Kubelet endpoint
type Provider struct {
	store workloadmeta.Component
	prometheus.Provider
}

// NewProvider returns a new Provider
func NewProvider(config *common.KubeletConfig, store workloadmeta.Component) (*Provider, error) {
	provider := &Provider{
		store: store,
	}
	transformers := prometheus.Transformers{
		"kubernetes_healthcheck":        provider.sliHealthCheck,
		"kubernetes_healthchecks_total": provider.sliHealthCheck,
	}

	scraperConfig := &prometheus.ScraperConfig{AllowNotFound: true, ShouldDisable: true}
	if config.SlisMetricsEndpoint == nil || *config.SlisMetricsEndpoint != "" {
		scraperConfig.Path = "/metrics/slis"
	}

	sliProvider, err := prometheus.NewProvider(config, transformers, scraperConfig)
	if err != nil {
		return nil, err
	}
	provider.Provider = sliProvider
	return provider, nil
}

func (p *Provider) sliHealthCheck(metricFam *prom.MetricFamily, sender sender.Sender) {
	for i := range metricFam.Samples {
		metric := &metricFam.Samples[i]
		metricSuffix := metric.Metric["__name__"]
		tags := p.MetricTags(metric)
		for i, tag := range tags {
			if strings.HasPrefix(tag, "name:") {
				tags[i] = strings.Replace(tag, "name:", "sli_name:", 1)
			}
		}

		tags = lo.Filter(tags, func(x string, _ int) bool {
			return !strings.HasPrefix(x, "type")
		})

		switch metricSuffix {
		case "kubernetes_healthchecks_total":
			sender.Count(common.KubeletMetricsPrefix+"slis."+metricSuffix, metric.Value, "", tags)
		case "kubernetes_healthcheck":
			sender.Gauge(common.KubeletMetricsPrefix+"slis."+metricSuffix, metric.Value, "", tags)
		}
	}
}
