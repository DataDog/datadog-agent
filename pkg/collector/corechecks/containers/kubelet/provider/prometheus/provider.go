// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	TypeLabel   = "__type__"
	MICROS_IN_S = 1000000
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

type ScraperConfig struct {
	Path string
	// AllowNotFound determines whether the check should error out or just return nothing when a 404 status code is encountered
	AllowNotFound bool
}

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

	metrics, err := ParseMetrics(data)
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

		// The parsing library we are using appends some suffixes to the metric name for samples in a histogram or summary,
		// To ensure backwards compatability, we will remove these
		if metric.Metric[TypeLabel] == "SUMMARY" || metric.Metric[TypeLabel] == "HISTOGRAM" {
			if strings.HasSuffix(metricName, "_bucket") {
				metricName = strings.TrimSuffix(metricName, "_bucket")
			} else if strings.HasSuffix(metricName, "_count") {
				metricName = strings.TrimSuffix(metricName, "_count")
			} else if strings.HasSuffix(metricName, "_sum") {
				metricName = strings.TrimSuffix(metricName, "_sum")
			}
		}

		// check metric name in ignore_metrics (or if it matches an ignored regex)
		if _, ok := p.ignoredMetrics[metricName]; ok {
			continue
		}

		if p.ignoredMetricsRegex != nil && p.ignoredMetricsRegex.MatchString(metricName) {
			continue
		}
		// finally, flow listed above
		if mName, ok := p.metricMapping[metricName]; ok {
			p.SubmitMetric(metric, mName, sender)
			continue
		}

		if transformer, ok := p.transformers[metricName]; ok {
			transformer(metric, sender)
			continue
		}

		if p.wildcardRegex != nil && p.wildcardRegex.MatchString(metricName) {
			p.SubmitMetric(metric, metricName, sender)
		}

		log.Debugf("Skipping metric `%s` as it is not defined in the metrics mapper, has no transformer function, nor does it match any wildcards.", metricName)
	}
	return nil
}

func (p *Provider) SubmitMetric(metric *model.Sample, metricName string, sender sender.Sender) {
	metricType := metric.Metric[TypeLabel]

	if p.ignoreMetricByLabel(metric, metricName) {
		return
	}

	// TODO constants
	// TODO switch statement
	if metricType == "HISTOGRAM" {
		p.submitHistogram(metric, metricName, sender)
	} else if metricType == "SUMMARY" {
		p.submitSummary(metric, metricName, sender)
	} else if metricType == "GAUGE" || metricType == "COUNTER" {
		nameWithNamespace := p.metricNameWithNamespace(metricName)

		tags := p.MetricTags(metric)
		if metricType == "COUNTER" && p.Config.MonotonicCounter != nil && *p.Config.MonotonicCounter {
			// TODO flush_first codepath
			sender.MonotonicCount(nameWithNamespace, float64(metric.Value), "", tags)
		} else {
			sender.Gauge(nameWithNamespace, float64(metric.Value), "", tags)

			// Metric is a "counter" but legacy behavior has "send_as_monotonic" defaulted to False
			// Submit metric as monotonic_count with appended name
			if metricName == "COUNTER" && p.Config.MonotonicWithGauge {
				sender.MonotonicCount(nameWithNamespace+".total", float64(metric.Value), "", tags)
			}
		}
	} else {
		log.Errorf("Metric type %s unsupported for metric %s.", metricType, metricName)
	}
}

func (p *Provider) submitHistogram(metric *model.Sample, metricName string, sender sender.Sender) {
	// TODO non_cumulative_buckets
	sampleName := string(metric.Metric["__name__"])
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
			// TODO
		} else if !strings.Contains(string(metric.Metric["le"]), "Inf") {
			p.sendDistributionCount(nameWithNamespace+".count", float64(metric.Value), "", tags, p.Config.DistributionCountsAsMonotonic, sender)
		}
	}
}

func (p *Provider) submitSummary(metric *model.Sample, metricName string, sender sender.Sender) {
	sampleName := string(metric.Metric["__name__"])
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

func (p *Provider) getHostname(metric *model.Sample) string {
	if hName, ok := metric.Metric[model.LabelName(p.Config.LabelToHostname)]; p.Config.LabelToHostname != "" && ok {
		// TODO label_to_hostname_suffix
		return string(hName)
	}
	return ""
}

func (p *Provider) MetricTags(metric *model.Sample) []string {
	tags := p.Config.Tags
	for lName, lVal := range metric.Metric {
		shouldExclude := lName == "__name__" || lName == TypeLabel
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

		// TODO include_labels
		tagName, exists := p.Config.LabelsMapper[string(lName)]
		if !exists {
			tagName = string(lName)
		}
		tags = append(tags, tagName+":"+string(lVal))
	}
	return tags
}

func (p *Provider) histogramConvertValues(metricName string, converter func(model.SampleValue) model.SampleValue) TransformerFunc {
	return func(sample *model.Sample, s sender.Sender) {
		sampleName := string(sample.Metric["__name__"])
		if strings.HasSuffix(sampleName, "_sum") {
			sample.Value = converter(sample.Value)
		} else if strings.HasSuffix(sampleName, "_bucket") && !strings.Contains(string(sample.Metric["le"]), "Inf") {
			var le float64
			var err error
			if le, err = strconv.ParseFloat(string(sample.Metric["le"]), 64); err != nil {
				// TODO log error?
				return
			}
			le = float64(converter(model.SampleValue(le)))
			sample.Metric["le"] = model.LabelValue(fmt.Sprintf("%f", le))
		}
		p.SubmitMetric(sample, metricName, s)
	}
}

func (p *Provider) HistogramFromSecondsToMicroseconds(metricName string) TransformerFunc {
	return p.histogramConvertValues(metricName, func(value model.SampleValue) model.SampleValue {
		return value * MICROS_IN_S
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

// ParseMetrics parses prometheus-formatted metrics from the input data.
func ParseMetrics(data []byte) (model.Vector, error) {
	// the prometheus TextParser does not support windows line separators, so we need to explicitly remove them
	data = bytes.Replace(data, []byte("\r"), []byte(""), -1)

	reader := bytes.NewReader(data)
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}

	var metrics model.Vector
	for _, family := range mf {
		samples, err := expfmt.ExtractSamples(&expfmt.DecodeOptions{Timestamp: model.Now()}, family)
		if err != nil {
			return nil, err
		}
		for i := range samples {
			// explicitly set the metric type as a label, as it will help when handling the metric
			samples[i].Metric[TypeLabel] = model.LabelValue(family.Type.String())
		}
		metrics = append(metrics, samples...)
	}
	return metrics, nil
}
