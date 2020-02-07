package dogstatsd

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type tagRetriever func(entity string, cardinality collectors.TagCardinality) ([]string, error)

var (
	hostTagPrefix       = "host:"
	entityIDTagPrefix   = "dd.internal.entity_id:"
	entityIDIgnoreValue = "none"

	getTags tagRetriever = tagger.Tag
)

func enrichTags(tags []string, defaultHostname, originID string) ([]string, string) {
	if len(tags) == 0 {
		return nil, defaultHostname
	}

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
	if entityIDValue == "" {
		// Add origin tags only if the entity id tags is not provided
		tags = append(tags, findOriginTags(originID)...)
	} else if entityIDValue != entityIDIgnoreValue {
		// currently only supported for pods
		entity := kubelet.KubePodTaggerEntityPrefix + entityIDValue
		entityTags, err := getTags(entity, tagger.DogstatsdCardinality)
		if err != nil {
			log.Tracef("Cannot get tags for entity %s: %s", entity, err)
		} else {
			tags = append(tags, entityTags...)
		}
	}
	return tags, host
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

func enrichMetricSample(metricSample dogstatsdMetricSample, namespace string, namespaceBlacklist []string, defaultHostname string, originID string) metrics.MetricSample {
	metricName := metricSample.name
	tags, hostname := enrichTags(metricSample.tags, defaultHostname, originID)

	if !isBlacklisted(metricName, namespace, namespaceBlacklist) {
		metricName = namespace + metricName
	}

	return metrics.MetricSample{
		Host:       hostname,
		Name:       metricName,
		Tags:       tags,
		Mtype:      enrichMetricType(metricSample.metricType),
		Value:      metricSample.value,
		SampleRate: metricSample.sampleRate,
		RawValue:   metricSample.setValue,
	}
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

func enrichEvent(event dogstatsdEvent, defaultHostname string, originID string) *metrics.Event {
	tags, hostFromTags := enrichTags(event.tags, defaultHostname, originID)

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

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, defaultHostname string, originID string) *metrics.ServiceCheck {
	tags, hostFromTags := enrichTags(serviceCheck.tags, defaultHostname, originID)

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

func findOriginTags(origin string) []string {
	var tags []string
	if origin != listeners.NoOrigin {
		originTags, err := tagger.Tag(origin, tagger.DogstatsdCardinality)
		if err != nil {
			log.Errorf(err.Error())
		} else {
			tags = append(tags, originTags...)
		}
	}
	return tags
}
