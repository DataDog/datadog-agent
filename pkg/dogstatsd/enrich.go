package dogstatsd

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type tagRetriever func(entity string, cardinality collectors.TagCardinality) ([]string, error)

var (
	hostTagPrefix       = "host:"
	entityIDTagPrefix   = "dd.internal.entity_id:"
	entityIDIgnoreValue = "none"

	getTags tagRetriever = tagger.Tag

	tlmUDPOriginDetectionError = telemetry.NewCounter("dogstatsd", "udp_origin_detection_error",
		nil, "Dogstatsd UDP origin detection error count")
)

func enrichTags(tags []string, defaultHostname string, originTagsFunc func() []string, entityIDPrecedenceEnabled bool) ([]string, string) {
	var originTags, entityTags []string
	host := defaultHostname

	n := 0
	entityIDValue := ""
	for _, tag := range tags {
		if strings.HasPrefix(tag, hostTagPrefix) {
			host = tag[len(hostTagPrefix):]
		} else if strings.HasPrefix(tag, entityIDTagPrefix) {
			entityIDValue = tag[len(entityIDTagPrefix):]
		} else {
			tags[n] = tag
			n++
		}
	}
	tags = tags[:n]

	if entityIDValue == "" || !entityIDPrecedenceEnabled {
		// Add origin tags only if the entity id tags is not provided
		originTags = originTagsFunc()
	}
	if entityIDValue != "" && entityIDValue != entityIDIgnoreValue {
		// Check if the value is not "none" in order to avoid calling
		// the tagger for entity that doesn't exist.

		// currently only supported for pods
		entity := kubelet.KubePodTaggerEntityPrefix + entityIDValue
		var err error
		entityTags, err = getTags(entity, tagger.DogstatsdCardinality)
		if err != nil {
			log.Tracef("Cannot get tags for entity %s: %s", entity, err)
			tlmUDPOriginDetectionError.Inc()
		}
	}

	var finalTags []string
	if cap(tags) >= len(tags)+len(originTags)+len(entityTags) {
		tags = append(tags, originTags...)
		tags = append(tags, entityTags...)
		finalTags = tags
	} else {
		finalTags = make([]string, 0, len(tags)+len(originTags)+len(entityTags))
		finalTags = append(finalTags, tags...)
		finalTags = append(finalTags, originTags...)
		finalTags = append(finalTags, entityTags...)
	}

	return finalTags, host
}

func enrichMetricType(dogstatsdMetricType metricType) metrics.MetricType {
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

func isBlacklisted(metricName, namespace string, namespaceBlacklist []string) bool {
	if namespace != "" {
		for _, prefix := range namespaceBlacklist {
			if strings.HasPrefix(metricName, prefix) {
				return true
			}
		}
	}
	return false
}

func enrichMetricSample(metricSamples []metrics.MetricSample, ddSample dogstatsdMetricSample, namespace string, namespaceBlacklist []string,
	defaultHostname string, originTagsFunc func() []string, entityIDPrecedenceEnabled bool, serverlessMode bool) []metrics.MetricSample {
	metricName := ddSample.name
	tags, hostname := enrichTags(ddSample.tags, defaultHostname, originTagsFunc, entityIDPrecedenceEnabled)

	if !isBlacklisted(metricName, namespace, namespaceBlacklist) {
		metricName = namespace + metricName
	}

	if serverlessMode { // we don't want to set the host while running in serverless mode
		hostname = ""
	}

	mtype := enrichMetricType(ddSample.metricType)

	// if 'ddSample.values' contains values we're enriching a multi-value
	// dogstatsd message and will create a MetricSample per value. If not
	// we will use 'ddSample.value'and return a single MetricSample
	if len(ddSample.values) > 0 {
		for idx := range ddSample.values {
			metricSamples = append(metricSamples,
				metrics.MetricSample{
					Host:       hostname,
					Name:       metricName,
					Tags:       tags,
					Mtype:      mtype,
					Value:      ddSample.values[idx],
					SampleRate: ddSample.sampleRate,
					RawValue:   ddSample.setValue,
				})
		}
		return metricSamples
	}

	// only one value contained, simple append it
	return append(metricSamples, metrics.MetricSample{
		Host:       hostname,
		Name:       metricName,
		Tags:       tags,
		Mtype:      mtype,
		Value:      ddSample.value,
		SampleRate: ddSample.sampleRate,
		RawValue:   ddSample.setValue,
	})
}

func enrichEventPriority(priority eventPriority) metrics.EventPriority {
	switch priority {
	case priorityNormal:
		return metrics.EventPriorityNormal
	case priorityLow:
		return metrics.EventPriorityLow
	}
	return metrics.EventPriorityNormal
}

func enrichEventAlertType(dogstatsdAlertType alertType) metrics.EventAlertType {
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

func enrichEvent(event dogstatsdEvent, defaultHostname string, originTagsFunc func() []string, entityIDPrecedenceEnabled bool) *metrics.Event {
	tags, hostFromTags := enrichTags(event.tags, defaultHostname, originTagsFunc, entityIDPrecedenceEnabled)

	enrichedEvent := &metrics.Event{
		Title:          event.title,
		Text:           event.text,
		Ts:             event.timestamp,
		Priority:       enrichEventPriority(event.priority),
		Tags:           tags,
		AlertType:      enrichEventAlertType(event.alertType),
		AggregationKey: event.aggregationKey,
		SourceTypeName: event.sourceType,
	}

	if len(event.hostname) != 0 {
		enrichedEvent.Host = event.hostname
	} else {
		enrichedEvent.Host = hostFromTags
	}
	return enrichedEvent
}

func enrichServiceCheckStatus(status serviceCheckStatus) metrics.ServiceCheckStatus {
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

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, defaultHostname string, originTagsFunc func() []string, entityIDPrecedenceEnabled bool) *metrics.ServiceCheck {
	tags, hostFromTags := enrichTags(serviceCheck.tags, defaultHostname, originTagsFunc, entityIDPrecedenceEnabled)

	enrichedServiceCheck := &metrics.ServiceCheck{
		CheckName: serviceCheck.name,
		Ts:        serviceCheck.timestamp,
		Status:    enrichServiceCheckStatus(serviceCheck.status),
		Message:   serviceCheck.message,
		Tags:      tags,
	}

	if len(serviceCheck.hostname) != 0 {
		enrichedServiceCheck.Host = serviceCheck.hostname
	} else {
		enrichedServiceCheck.Host = hostFromTags
	}
	return enrichedServiceCheck
}
