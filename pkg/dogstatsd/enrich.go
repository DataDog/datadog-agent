package dogstatsd

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

var (
	hostTagPrefix       = "host:"
	entityIDTagPrefix   = "dd.internal.entity_id:"
	entityIDIgnoreValue = "none"
)

func extractTagsMetadata(tags []string, defaultHostname string, originTags string, entityIDPrecedenceEnabled bool) ([]string, string, string, string) {
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

	origin := ""
	// We use the UDS socket origin if no origin ID was specify in the tags
	// or 'dogstatsd_entity_id_precedence' is set to False (default false).
	if entityIDValue == "" || !entityIDPrecedenceEnabled {
		// Add origin tags only if the entity id tags is not provided
		origin = originTags
	}

	k8sOrigin := ""
	// We set k8sOriginID if the metrics contain a 'dd.internal.entity_id' tag different from 'none'.
	if entityIDValue != "" && entityIDValue != entityIDIgnoreValue {
		// Check if the value is not "none" in order to avoid calling
		// the tagger for entity that doesn't exist.

		// currently only supported for pods
		k8sOrigin = kubelet.KubePodTaggerEntityPrefix + entityIDValue
	}

	return tags, host, origin, k8sOrigin
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
	defaultHostname string, origin string, entityIDPrecedenceEnabled bool, serverlessMode bool) []metrics.MetricSample {
	metricName := ddSample.name
	tags, hostnameFromTags, originID, k8sOriginID := extractTagsMetadata(ddSample.tags, defaultHostname, origin, entityIDPrecedenceEnabled)

	if !isBlacklisted(metricName, namespace, namespaceBlacklist) {
		metricName = namespace + metricName
	}

	if serverlessMode { // we don't want to set the host while running in serverless mode
		hostnameFromTags = ""
	}

	mtype := enrichMetricType(ddSample.metricType)

	// if 'ddSample.values' contains values we're enriching a multi-value
	// dogstatsd message and will create a MetricSample per value. If not
	// we will use 'ddSample.value'and return a single MetricSample
	if len(ddSample.values) > 0 {
		for idx := range ddSample.values {
			metricSamples = append(metricSamples,
				metrics.MetricSample{
					Host:        hostnameFromTags,
					Name:        metricName,
					Tags:        tags,
					Mtype:       mtype,
					Value:       ddSample.values[idx],
					SampleRate:  ddSample.sampleRate,
					RawValue:    ddSample.setValue,
					OriginID:    originID,
					K8sOriginID: k8sOriginID,
				})
		}
		return metricSamples
	}

	// only one value contained, simple append it
	return append(metricSamples, metrics.MetricSample{
		Host:        hostnameFromTags,
		Name:        metricName,
		Tags:        tags,
		Mtype:       mtype,
		Value:       ddSample.value,
		SampleRate:  ddSample.sampleRate,
		RawValue:    ddSample.setValue,
		OriginID:    originID,
		K8sOriginID: k8sOriginID,
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

func enrichEvent(event dogstatsdEvent, defaultHostname string, origin string, entityIDPrecedenceEnabled bool) *metrics.Event {
	tags, hostnameFromTags, originID, k8sOriginID := extractTagsMetadata(event.tags, defaultHostname, origin, entityIDPrecedenceEnabled)

	enrichedEvent := &metrics.Event{
		Title:          event.title,
		Text:           event.text,
		Ts:             event.timestamp,
		Priority:       enrichEventPriority(event.priority),
		Tags:           tags,
		AlertType:      enrichEventAlertType(event.alertType),
		AggregationKey: event.aggregationKey,
		SourceTypeName: event.sourceType,
		OriginID:       originID,
		K8sOriginID:    k8sOriginID,
	}

	if len(event.hostname) != 0 {
		enrichedEvent.Host = event.hostname
	} else {
		enrichedEvent.Host = hostnameFromTags
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

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, defaultHostname string, origin string, entityIDPrecedenceEnabled bool) *metrics.ServiceCheck {
	tags, hostnameFromTags, originID, k8sOriginID := extractTagsMetadata(serviceCheck.tags, defaultHostname, origin, entityIDPrecedenceEnabled)

	enrichedServiceCheck := &metrics.ServiceCheck{
		CheckName:   serviceCheck.name,
		Ts:          serviceCheck.timestamp,
		Status:      enrichServiceCheckStatus(serviceCheck.status),
		Message:     serviceCheck.message,
		Tags:        tags,
		OriginID:    originID,
		K8sOriginID: k8sOriginID,
	}

	if len(serviceCheck.hostname) != 0 {
		enrichedServiceCheck.Host = serviceCheck.hostname
	} else {
		enrichedServiceCheck.Host = hostnameFromTags
	}
	return enrichedServiceCheck
}
