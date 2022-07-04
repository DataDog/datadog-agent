// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

var (
	hostTagPrefix       = "host:"
	entityIDTagPrefix   = "dd.internal.entity_id:"
	entityIDIgnoreValue = "none"
	// CardinalityTagPrefix is used to set the dynamic cardinality
	CardinalityTagPrefix = "dd.internal.card:"
)

// extractTagsMetadata returns tags (client tags + host tag) and information needed to query tagger (origins, cardinality).
//
// The following tables explain how the origins are chosen.
// originFromUDS is the origin discovered via UDS origin detection (container ID).
// originFromTag is the origin sent by the client via the dd.internal.entity_id tag (non-prefixed pod uid).
// originFromMsg is the origin sent by the client via the container field (non-prefixed container ID).
// entityIDPrecedenceEnabled refers to the dogstatsd_entity_id_precedence parameter.
//
//  ---------------------------------------------------------------------------------
// | originFromUDS | originFromTag | entityIDPrecedenceEnabled || Result: udsOrigin  |
// |---------------|---------------|---------------------------||--------------------|
// | any           | any           | false                     || originFromUDS      |
// | any           | any           | true                      || empty              |
// | any           | empty         | any                       || originFromUDS      |
//  ---------------------------------------------------------------------------------
//
//  ---------------------------------------------------------------------------------
// | originFromTag          | originFromMsg   || Result: originFromClient            |
// |------------------------|-----------------||-------------------------------------|
// | not empty && not none  | any             || pod prefix + originFromTag          |
// | empty                  | empty           || empty                               |
// | none                   | empty           || empty                               |
// | empty                  | not empty       || container prefix + originFromMsg    |
// | none                   | not empty       || container prefix + originFromMsg    |
//  ---------------------------------------------------------------------------------
func extractTagsMetadata(tags []string, defaultHostname, originFromUDS string, originFromMsg []byte, entityIDPrecedenceEnabled bool) ([]string, string, string, string, string) {
	host := defaultHostname

	n := 0
	originFromTag, cardinality := "", ""
	for _, tag := range tags {
		if strings.HasPrefix(tag, hostTagPrefix) {
			host = tag[len(hostTagPrefix):]
		} else if strings.HasPrefix(tag, entityIDTagPrefix) {
			originFromTag = tag[len(entityIDTagPrefix):]
		} else if strings.HasPrefix(tag, CardinalityTagPrefix) {
			cardinality = tag[len(CardinalityTagPrefix):]
		} else {
			tags[n] = tag
			n++
		}
	}
	tags = tags[:n]

	udsOrigin := ""
	// We use the UDS socket origin if no origin ID was specify in the tags
	// or 'dogstatsd_entity_id_precedence' is set to False (default false).
	if originFromTag == "" || !entityIDPrecedenceEnabled {
		// Add origin tags only if the entity id tags is not provided
		udsOrigin = originFromUDS
	}

	// originFromClient can either be originFromTag or originFromMsg
	originFromClient := ""

	// We set originFromClient if the metrics contain a 'dd.internal.entity_id' tag different from 'none'.
	if originFromTag != "" && originFromTag != entityIDIgnoreValue {
		// Check if the value is not "none" in order to avoid calling
		// the tagger for entity that doesn't exist.

		// currently only supported for pods
		originFromClient = kubelet.KubePodTaggerEntityPrefix + originFromTag
	} else if originFromTag == "" && len(originFromMsg) > 0 {
		// originFromMsg is the container id sent by the newer clients.
		// Opt-in is handled when parsing.
		originFromClient = containers.BuildTaggerEntityName(string(originFromMsg))
	}

	return tags, host, udsOrigin, originFromClient, cardinality
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

func isExcluded(metricName, namespace string, excludedNamespaces []string) bool {
	if namespace != "" {
		for _, prefix := range excludedNamespaces {
			if strings.HasPrefix(metricName, prefix) {
				return true
			}
		}
	}
	return false
}

func isMetricBlocklisted(metricName string, metricBlocklist []string) bool {
	for _, item := range metricBlocklist {
		if metricName == item {
			return true
		}
	}
	return false
}

func tsToFloatForSamples(ts time.Time) float64 {
	// XXX(remy): we may see this call appear in profiles
	if ts.IsZero() { // avoid a conversion
		// for on-time samples, we don't want to write any value in there
		return 0.0
	}
	return float64(ts.Unix())
}

func enrichMetricSample(dest []metrics.MetricSample, ddSample dogstatsdMetricSample, namespace string, excludedNamespaces []string,
	metricBlocklist []string, defaultHostname string, origin string, entityIDPrecedenceEnabled bool, serverlessMode bool) []metrics.MetricSample {
	metricName := ddSample.name
	tags, hostnameFromTags, udsOrigin, clientOrigin, cardinality := extractTagsMetadata(ddSample.tags, defaultHostname, origin, ddSample.containerID, entityIDPrecedenceEnabled)

	if !isExcluded(metricName, namespace, excludedNamespaces) {
		metricName = namespace + metricName
	}

	if len(metricBlocklist) > 0 && isMetricBlocklisted(metricName, metricBlocklist) {
		return []metrics.MetricSample{}
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
			dest = append(dest,
				metrics.MetricSample{
					Host:             hostnameFromTags,
					Name:             metricName,
					Tags:             tags,
					Mtype:            mtype,
					Value:            ddSample.values[idx],
					SampleRate:       ddSample.sampleRate,
					RawValue:         ddSample.setValue,
					Timestamp:        tsToFloatForSamples(ddSample.ts),
					OriginFromUDS:    udsOrigin,
					OriginFromClient: clientOrigin,
					Cardinality:      cardinality,
				})
		}
		return dest
	}

	// only one value contained, simple append it
	return append(dest, metrics.MetricSample{
		Host:             hostnameFromTags,
		Name:             metricName,
		Tags:             tags,
		Mtype:            mtype,
		Value:            ddSample.value,
		SampleRate:       ddSample.sampleRate,
		RawValue:         ddSample.setValue,
		Timestamp:        tsToFloatForSamples(ddSample.ts),
		OriginFromUDS:    udsOrigin,
		OriginFromClient: clientOrigin,
		Cardinality:      cardinality,
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
	tags, hostnameFromTags, udsOrigin, clientOrigin, cardinality := extractTagsMetadata(event.tags, defaultHostname, origin, event.containerID, entityIDPrecedenceEnabled)

	enrichedEvent := &metrics.Event{
		Title:            event.title,
		Text:             event.text,
		Ts:               event.timestamp,
		Priority:         enrichEventPriority(event.priority),
		Tags:             tags,
		AlertType:        enrichEventAlertType(event.alertType),
		AggregationKey:   event.aggregationKey,
		SourceTypeName:   event.sourceType,
		OriginFromUDS:    udsOrigin,
		OriginFromClient: clientOrigin,
		Cardinality:      cardinality,
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
	tags, hostnameFromTags, udsOrigin, clientOrigin, cardinality := extractTagsMetadata(serviceCheck.tags, defaultHostname, origin, serviceCheck.containerID, entityIDPrecedenceEnabled)

	enrichedServiceCheck := &metrics.ServiceCheck{
		CheckName:        serviceCheck.name,
		Ts:               serviceCheck.timestamp,
		Status:           enrichServiceCheckStatus(serviceCheck.status),
		Message:          serviceCheck.message,
		Tags:             tags,
		OriginFromUDS:    udsOrigin,
		OriginFromClient: clientOrigin,
		Cardinality:      cardinality,
	}

	if len(serviceCheck.hostname) != 0 {
		enrichedServiceCheck.Host = serviceCheck.hostname
	} else {
		enrichedServiceCheck.Host = hostnameFromTags
	}
	return enrichedServiceCheck
}
