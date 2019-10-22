package dogstatsd_tmp

import (
	"bytes"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type tagRetriever func(entity string, cardinality collectors.TagCardinality) ([]string, error)

var (
	hostTagPrefix     = []byte("host:")
	entityIDTagPrefix = []byte("dd.internal.entity_id:")

	getTags tagRetriever = tagger.Tag
)

func convertTags(tags [][]byte, defaultHostname string) ([]string, string) {
	if len(tags) == 0 {
		return nil, defaultHostname
	}

	tagsList := make([]string, 0, len(tags))
	host := defaultHostname

	for _, tag := range tags {
		if bytes.HasPrefix(tag, hostTagPrefix) {
			host = string(tag[len(hostTagPrefix):])
		} else if bytes.HasPrefix(tag, entityIDTagPrefix) {
			// currently only supported for pods
			entity := kubelet.KubePodTaggerEntityPrefix + string(tag[len(entityIDTagPrefix):])
			entityTags, err := getTags(entity, tagger.DogstatsdCardinality)
			if err != nil {
				log.Tracef("Cannot get tags for entity %s: %s", entity, err)
				continue
			}
			tagsList = append(tagsList, entityTags...)
		} else {
			tagsList = append(tagsList, string(tag))
		}
	}
	return tagsList, host
}

func convertMetricType(dogstatsdMetricType metricType) metrics.MetricType {
	switch dogstatsdMetricType {
	case gaugeType:
		return metrics.GaugeType
	case countType:
		return metrics.CounterType
	case distributionType:
		return metrics.DistributionType
	case histogramType:
		return metrics.HistogramType
	case setType:
		return metrics.SetType
	case timingType:
		return metrics.HistogramType
	}
	return metrics.GaugeType
}

func convertMetricSample(metricSample dogstatsdMetricSample, namespace string, namespaceBlacklist []string, defaultHostname string) *metrics.MetricSample {
	metricName := string(metricSample.name)
	if namespace != "" {
		blacklisted := false
		for _, prefix := range namespaceBlacklist {
			if strings.HasPrefix(metricName, prefix) {
				blacklisted = true
			}
		}
		if !blacklisted {
			metricName = namespace + metricName
		}
	}

	tags, hostname := convertTags(metricSample.tags, defaultHostname)

	return &metrics.MetricSample{
		Host:       hostname,
		Name:       metricName,
		Tags:       tags,
		Mtype:      convertMetricType(metricSample.metricType),
		Value:      metricSample.value,
		SampleRate: metricSample.sampleRate,
		RawValue:   string(metricSample.setValue),
	}
}
