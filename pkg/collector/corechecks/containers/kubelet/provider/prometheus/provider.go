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
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	// NameLabel is the special tag which signifies the name of the metric collected from Prometheus
	NameLabel             = "__name__"
	microsecondsInSeconds = 1000000

	metricTypeHistogram = "HISTOGRAM"
	metricTypeSummary   = "SUMMARY"
	metricTypeCounter   = "COUNTER"
	metricTypeGauge     = "GAUGE"
)

var (
	// ParseMetricsWithFilterFunc allows us to override the function used for parsing the prometheus metrics. It should
	// only be overridden for testing purposes.
	ParseMetricsWithFilterFunc = prometheus.ParseMetricsWithFilter
)

// TransformerFunc outlines the function signature for any transformers which will be used with the prometheus Provider
type TransformerFunc func(*prometheus.MetricFamily, sender.Sender)

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
	AllowNotFound       bool
	TextFilterBlacklist []string
	// ShouldDisable determines if a provider should be disabled when a 404 status code is encountered
	ShouldDisable bool
	IsDisabled    bool
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
		case map[string]string:
			for k1, v1 := range val {
				metricMappings[k1] = v1
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

	if config.LabelsMapper == nil {
		config.LabelsMapper = make(map[string]string)
	}
	// Rename bucket "le" label to "upper_bound"
	config.LabelsMapper["le"] = "upper_bound"

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
	if p.ScraperConfig.IsDisabled {
		log.Debugf("Skipping collecting metrics as provider is disabled")
		return nil
	}
	data, status, err := kc.QueryKubelet(context.TODO(), p.ScraperConfig.Path)
	if err != nil {
		log.Debugf("Unable to collect query probes endpoint: %s", err)
		return err
	}
	if status == 404 && p.ScraperConfig.ShouldDisable {
		p.ScraperConfig.IsDisabled = true
		return nil
	}

	if status == 404 && p.ScraperConfig.AllowNotFound {
		return nil
	}

	metrics, err := ParseMetricsWithFilterFunc(data, p.ScraperConfig.TextFilterBlacklist)
	if err != nil {
		return err
	}

	// Report metrics
	for _, metricFam := range metrics {
		// Handle a Prometheus metric according to the following flow:
		// - search `p.Config.metricMapping` for a prometheus.metric to datadog.metric mapping
		// - call check method with the same name as the metric
		// - log info if none of the above worked
		if metricFam == nil || len(metricFam.Samples) == 0 {
			continue
		}
		metricName := metricFam.Name

		// check metric name in ignore_metrics (or if it matches an ignored regex)
		if _, ok := p.ignoredMetrics[metricName]; ok {
			continue
		}

		if p.ignoredMetricsRegex != nil && p.ignoredMetricsRegex.MatchString(metricName) {
			continue
		}
		// finally, flow listed above
		if mName, ok := p.metricMapping[metricName]; ok {
			p.SubmitMetric(metricFam, mName, sender)
			continue
		}

		if transformer, ok := p.transformers[metricName]; ok {
			transformer(metricFam, sender)
			continue
		}

		if p.wildcardRegex != nil && p.wildcardRegex.MatchString(metricName) {
			p.SubmitMetric(metricFam, metricName, sender)
		}

		log.Debugf("Skipping metric `%s` as it is not defined in the metrics mapper, has no transformer function, nor does it match any wildcards.", metricName)
	}
	return nil
}

// SubmitMetric forwards a given metric to the sender.Sender, using the passed in metricName as the name of the submitted metric.
func (p *Provider) SubmitMetric(metricFam *prometheus.MetricFamily, metricName string, sender sender.Sender) {
	metricType := metricFam.Type

	for _, metric := range metricFam.Samples {
		if p.ignoreMetricByLabel(metric, metricName) {
			continue
		}

		if math.IsNaN(float64(metric.Value)) || math.IsInf(float64(metric.Value), 0) {
			log.Debugf("Metric value is not supported for metric %s", metricName)
			continue
		}

		switch metricType {
		case metricTypeHistogram:
			p.submitHistogram(metric, metricName, sender)
		case metricTypeSummary:
			p.submitSummary(metric, metricName, sender)
		case metricTypeCounter, metricTypeGauge:
			nameWithNamespace := p.metricNameWithNamespace(metricName)

			tags := p.MetricTags(metric)
			if metricType == metricTypeCounter && p.Config.MonotonicCounter != nil && *p.Config.MonotonicCounter {
				sender.MonotonicCount(nameWithNamespace, float64(metric.Value), "", tags)
			} else {
				sender.Gauge(nameWithNamespace, float64(metric.Value), "", tags)

				// Metric is a "counter" but legacy behavior has "send_as_monotonic" defaulted to False
				// Submit metric as monotonic_count with appended name
				if metricName == metricTypeCounter && p.Config.MonotonicWithGauge {
					sender.MonotonicCount(nameWithNamespace+".total", float64(metric.Value), "", tags)
				}
			}
		default:
			log.Errorf("Metric type %s unsupported for metric %s.", metricType, metricName)
		}
	}
}

func (p *Provider) submitHistogram(metric *model.Sample, metricName string, sender sender.Sender) {
	sampleName := string(metric.Metric[NameLabel])
	tags := p.MetricTags(metric)
	nameWithNamespace := p.metricNameWithNamespace(metricName)
	if strings.HasSuffix(sampleName, "_sum") && !p.Config.DistributionBuckets {
		p.sendDistributionCount(nameWithNamespace+".sum", float64(metric.Value), "", tags, p.Config.DistributionSumsAsMonotonic, sender)
	} else if strings.HasSuffix(sampleName, "_count") && !p.Config.DistributionBuckets {
		if p.Config.SendHistogramBuckets != nil && *p.Config.SendHistogramBuckets {
			tags = append(tags, "upper_bound:none")
		}
		p.sendDistributionCount(nameWithNamespace+".count", float64(metric.Value), "", tags, p.Config.DistributionCountsAsMonotonic, sender)
	} else if p.Config.SendHistogramBuckets != nil && *p.Config.SendHistogramBuckets && strings.HasSuffix(sampleName, "_bucket") {
		if p.Config.DistributionBuckets {
			log.Debug("'send_distribution_buckets' config value found, but is not currently supported")
		} else if !strings.Contains(string(metric.Metric["le"]), "Inf") {
			p.sendDistributionCount(nameWithNamespace+".count", float64(metric.Value), "", tags, p.Config.DistributionCountsAsMonotonic, sender)
		}
	}
}

func (p *Provider) submitSummary(metric *model.Sample, metricName string, sender sender.Sender) {
	sampleName := string(metric.Metric[NameLabel])
	tags := p.MetricTags(metric)
	nameWithNamespace := p.metricNameWithNamespace(metricName)
	if strings.HasSuffix(sampleName, "_sum") {
		p.sendDistributionCount(nameWithNamespace+".sum", float64(metric.Value), "", tags, p.Config.DistributionSumsAsMonotonic, sender)
	} else if strings.HasSuffix(sampleName, "_count") {
		p.sendDistributionCount(nameWithNamespace+".count", float64(metric.Value), "", tags, p.Config.DistributionCountsAsMonotonic, sender)
	} else {
		sender.Gauge(nameWithNamespace+".quantile", float64(metric.Value), "", tags)
	}
}

func (p *Provider) sendDistributionCount(metric string, value float64, hostname string, tags []string, monotonic bool, sender sender.Sender) {
	if monotonic {
		sender.MonotonicCount(metric, value, hostname, tags)
	} else {
		sender.Gauge(metric, value, hostname, tags)
		if p.Config.MonotonicWithGauge {
			sender.MonotonicCount(metric+".total", value, hostname, tags)
		}
	}
}

// MetricTags returns the slice of tags to be submitted for a given metric, looking at the existing metric labels and
// filtering or transforming them based on config values set on the Provider.
func (p *Provider) MetricTags(metric *model.Sample) []string {
	tags := p.Config.Tags
	for lName, lVal := range metric.Metric {
		shouldExclude := lName == NameLabel
		if shouldExclude {
			continue
		}

		for i := range p.Config.ExcludeLabels {
			if string(lName) == p.Config.ExcludeLabels[i] {
				shouldExclude = true
				break
			}
		}
		if shouldExclude {
			continue
		}

		tagName, exists := p.Config.LabelsMapper[string(lName)]
		if !exists {
			tagName = string(lName)
		}
		tags = append(tags, tagName+":"+string(lVal))
	}
	return tags
}

func (p *Provider) histogramConvertValues(metricName string, converter func(model.SampleValue) model.SampleValue) TransformerFunc {
	return func(mf *prometheus.MetricFamily, s sender.Sender) {
		for _, sample := range mf.Samples {
			sampleName := string(sample.Metric[NameLabel])
			if strings.HasSuffix(sampleName, "_sum") {
				sample.Value = converter(sample.Value)
			} else if strings.HasSuffix(sampleName, "_bucket") && !strings.Contains(string(sample.Metric["le"]), "Inf") {
				var le float64
				var err error
				if le, err = strconv.ParseFloat(string(sample.Metric["le"]), 64); err != nil {
					log.Errorf("Unable to convert histogram bucket limit %v to a float for metric %s", sample.Metric["le"], sampleName)
					continue
				}
				le = float64(converter(model.SampleValue(le)))
				sample.Metric["le"] = model.LabelValue(fmt.Sprintf("%f", le))
			}
		}
		p.SubmitMetric(mf, metricName, s)
	}
}

// HistogramFromSecondsToMicroseconds is a predefined TransformerFunc which takes a value which is currently represented
// as a second value, and transforms it to a microsecond value.
func (p *Provider) HistogramFromSecondsToMicroseconds(metricName string) TransformerFunc {
	return p.histogramConvertValues(metricName, func(value model.SampleValue) model.SampleValue {
		return value * microsecondsInSeconds
	})
}

func (p *Provider) ignoreMetricByLabel(metric *model.Sample, metricName string) bool {
	for lKey, lVal := range p.Config.IgnoreMetricsByLabels {
		switch val := lVal.(type) {
		case []string:
			if len(val) == 0 {
				log.Debugf("Skipping filter label `%s` with an empty values list, did you mean to use '*' wildcard?", lKey)
			}
			for l, v := range metric.Metric {
				for i := range val {
					if string(l) == lKey {
						if val[i] == "*" || val[i] == string(v) {
							log.Debugf("Skipping metric `%s` due to label key matching: %s", metricName, lKey)
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (p *Provider) metricNameWithNamespace(metricName string) string {
	nameWithNamespace := metricName
	if p.Config.Namespace != "" {
		nameWithNamespace = fmt.Sprintf("%s.%s", strings.TrimSuffix(p.Config.Namespace, "."), metricName)
	}
	return nameWithNamespace
}
