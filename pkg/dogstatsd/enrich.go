package dogstatsd

import (
	"bytes"

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

func enrichTags(tags [][]byte, defaultHostname []byte) ([][]byte, []byte) {
	if len(tags) == 0 {
		return nil, defaultHostname
	}

	newTags := tags[:0]
	var extraTags []string

	host := defaultHostname

	for _, tag := range tags {
		if bytes.HasPrefix(tag, hostTagPrefix) {
			host = tag[len(hostTagPrefix):]
		} else if bytes.HasPrefix(tag, entityIDTagPrefix) {
			// currently only supported for pods
			entity := kubelet.KubePodTaggerEntityPrefix + string(tag[len(entityIDTagPrefix):])
			entityTags, err := getTags(entity, tagger.DogstatsdCardinality)
			if err != nil {
				log.Tracef("Cannot get tags for entity %s: %s", entity, err)
				continue
			}
			extraTags = append(extraTags, entityTags...)
		} else {
			newTags = append(newTags, tag)
		}
	}

	for _, extraTag := range extraTags {
		newTags = append(newTags, []byte(extraTag))
	}
	return newTags, host
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

func enrichMetricSample(metricSample dogstatsdMetricSample, namespace []byte, namespaceBlacklist [][]byte, defaultHostname []byte) MetricSample {
	metricName := metricSample.name
	if len(namespace) != 0 {
		blacklisted := false
		for _, prefix := range namespaceBlacklist {
			if bytes.HasPrefix(metricName, prefix) {
				blacklisted = true
			}
		}
		if !blacklisted {
			metricName = make([]byte, 0, len(namespace)+len(metricSample.name))
			metricName = append(metricName, namespace...)
			metricName = append(metricName, metricSample.name...)
		}
	}

	tags, hostname := enrichTags(metricSample.tags, defaultHostname)

	return MetricSample{
		Hostname:   hostname,
		Name:       metricName,
		Tags:       tags,
		MetricType: convertMetricType(metricSample.metricType),
		Value:      metricSample.value,
		SampleRate: metricSample.sampleRate,
		SetValue:   metricSample.setValue,
	}
}

func convertEventPriority(priority eventPriority) metrics.EventPriority {
	switch priority {
	case priorityNormal:
		return metrics.EventPriorityNormal
	case priorityLow:
		return metrics.EventPriorityLow
	}
	return metrics.EventPriorityNormal
}

func convertEventAlertType(dogstatsdAlertType alertType) metrics.EventAlertType {
	switch dogstatsdAlertType {
	case alertTypeSuccess:
		return metrics.EventAlertTypeSuccess
	case alertTypeInfo:
		return metrics.EventAlertTypeInfo
	case alertTypeWarning:
		return metrics.EventAlertTypeWarning
	case alertTypeError:
		return metrics.EventAlertTypeError
	}
	return metrics.EventAlertTypeSuccess
}

func enirchEvent(event dogstatsdEvent, defaultHostname []byte) Event {
	tags, hostFromTags := enrichTags(event.tags, defaultHostname)

	convertedEvent := Event{
		Title:          event.title,
		Text:           event.text,
		Timestamp:      event.timestamp,
		Priority:       convertEventPriority(event.priority),
		Tags:           tags,
		AlertType:      convertEventAlertType(event.alertType),
		AggregationKey: event.aggregationKey,
		SourceTypeName: event.sourceType,
	}

	if len(event.hostname) != 0 {
		convertedEvent.Hostname = event.hostname
	} else {
		convertedEvent.Hostname = hostFromTags
	}
	return convertedEvent
}

func convertServiceCheckStatus(status serviceCheckStatus) metrics.ServiceCheckStatus {
	switch status {
	case serviceCheckStatusUnknown:
		return metrics.ServiceCheckUnknown
	case serviceCheckStatusOk:
		return metrics.ServiceCheckOK
	case serviceCheckStatusWarning:
		return metrics.ServiceCheckWarning
	case serviceCheckStatusCritical:
		return metrics.ServiceCheckCritical
	}
	return metrics.ServiceCheckUnknown
}

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, defaultHostname []byte) ServiceCheck {
	tags, hostFromTags := enrichTags(serviceCheck.tags, defaultHostname)

	convertedServiceCheck := ServiceCheck{
		Name:      serviceCheck.name,
		Timestamp: serviceCheck.timestamp,
		Status:    convertServiceCheckStatus(serviceCheck.status),
		Message:   serviceCheck.message,
		Tags:      tags,
	}

	if len(serviceCheck.hostname) != 0 {
		convertedServiceCheck.Hostname = serviceCheck.hostname
	} else {
		convertedServiceCheck.Hostname = hostFromTags
	}
	return convertedServiceCheck
}
