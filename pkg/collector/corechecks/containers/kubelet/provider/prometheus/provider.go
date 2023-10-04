// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

// Package prometheus implements data collection from a prometheus Kubelet
// endpoint.
package prometheus

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// TransformerFunc outlines the function signature for any transformers which will be used with the prometheus Provider
type TransformerFunc func(*model.Sample, sender.Sender)

// Transformers is a mapping of metric names to their desired TransformerFunc
type Transformers map[string]TransformerFunc

// Provider provides the metrics related to data collected from a prometheus Kubelet endpoint.
//
// It is based on the python openmetrics mixin which is defined in the integrations-core repo, however in its current
// form it has been trimmed down to focus on the functionality used by the kubelet check specifically. Do not assume that
// this is a complete implementation of that.
type Provider struct {
	Config              *common.KubeletConfig
	ScraperConfig       *ScraperConfig
	transformers        Transformers
	metricMapping       map[string]string
	wildcardRegex       *regexp.Regexp
	ignoredMetrics      map[string]bool
	ignoredMetricsRegex *regexp.Regexp
}

// ScraperConfig contains the configuration of the Prometheus scraper.
type ScraperConfig struct {
	Path string
	// AllowNotFound determines whether the check should error out or just return nothing when a 404 status code is encountered
	AllowNotFound bool
}

// NewProvider returns a new Provider.
func NewProvider(config *common.KubeletConfig, transformers Transformers, scraperConfig *ScraperConfig) (Provider, error) {
	if config == nil {
		config = &common.KubeletConfig{}
	}

	if scraperConfig == nil {
		scraperConfig = &ScraperConfig{}
	}

	var wildcardMetrics []string
	var wildcardRegex, ignoredRegex *regexp.Regexp
	var err error
	// Build metric include list
	metricMappings := map[string]string{}
	for _, v := range config.Metrics {
		switch val := v.(type) {
		case string:
			metricMappings[val] = val
			if strings.Contains(val, "*") {
				wildcardMetrics = append(wildcardMetrics, val)
			}
		case map[interface{}]interface{}:
			for k1, v1 := range val {
				if _, ok := k1.(string); !ok {
					continue
				}
				if _, ok := v1.(string); !ok {
					continue
				}
				metricMappings[k1.(string)] = v1.(string)
			}
		}
	}
	// TODO translate properly (python supports regex or glob format, and converts glob to regex)
	if len(wildcardMetrics) > 0 {
		wildcardRegex, err = regexp.Compile(strings.Join(wildcardMetrics, "|"))
		if err != nil {
			return Provider{}, err
		}
	}

	// Build metric exclude list
	var ignoredMetricsWildcard []string
	ignoredMetrics := map[string]bool{}
	for _, v := range config.IgnoreMetrics {
		ignoredMetrics[v] = true

		if strings.Contains(v, "*") {
			ignoredMetricsWildcard = append(ignoredMetricsWildcard, v)
		}
	}
	// TODO translate properly (python supports regex or glob format, and converts glob to regex)
	if len(ignoredMetricsWildcard) > 0 {
		ignoredRegex, err = regexp.Compile(strings.Join(ignoredMetricsWildcard, "|"))
		if err != nil {
			return Provider{}, err
		}
	}

	return Provider{
		Config:              config,
		ScraperConfig:       scraperConfig,
		transformers:        transformers,
		metricMapping:       metricMappings,
		wildcardRegex:       wildcardRegex,
		ignoredMetrics:      ignoredMetrics,
		ignoredMetricsRegex: ignoredRegex,
	}, nil
}

// Provide sends the metrics collected.
func (p *Provider) Provide(kc kubelet.KubeUtilInterface, sender sender.Sender) error {
	// Collect raw data
	data, status, err := kc.QueryKubelet(context.TODO(), p.ScraperConfig.Path)
	if err != nil {
		log.Debugf("Unable to collect query probes endpoint: %s", err)
		return err
	}
	if status == 404 && p.ScraperConfig.AllowNotFound {
		return nil
	}

	metrics, err := prometheus.ParseMetrics(data)
	if err != nil {
		return err
	}

	// Report metrics
	for _, metric := range metrics {
		// Handle a Prometheus metric according to the following flow:
		// - search `p.Config.metricMapping` for a prometheus.metric to datadog.metric mapping
		// - call check method with the same name as the metric
		// - log info if none of the above worked
		if metric == nil {
			continue
		}
		metricName := string(metric.Metric["__name__"])
		// check metric name in ignore_metrics (or if it matches an ignored regex)
		if _, ok := p.ignoredMetrics[metricName]; ok {
			continue
		}

		if p.ignoredMetricsRegex != nil && p.ignoredMetricsRegex.MatchString(metricName) {
			continue
		}
		// finally, flow listed above
		if mName, ok := p.metricMapping[metricName]; ok {
			p.submitMetric(metric, mName, sender)
			continue
		}

		if transformer, ok := p.transformers[metricName]; ok {
			transformer(metric, sender)
			continue
		}

		if p.wildcardRegex != nil && p.wildcardRegex.MatchString(metricName) {
			p.submitMetric(metric, metricName, sender)
		}

		log.Debugf("Skipping metric `%s` as it is not defined in the metrics mapper, has no transformer function, nor does it match any wildcards.", metricName)
	}
	return nil
}

func (p *Provider) submitMetric(metric *model.Sample, metricName string, sender sender.Sender) {
	nameWithNamespace := metricName
	if p.Config.Namespace != "" {
		nameWithNamespace = fmt.Sprintf("%s.%s", strings.TrimSuffix(p.Config.Namespace, "."), metricName)
	}

	tags := p.Config.Tags

	sender.Gauge(nameWithNamespace, float64(metric.Value), "", tags)
}
