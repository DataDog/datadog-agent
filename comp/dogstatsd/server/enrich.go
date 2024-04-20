// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

var (
	hostTagPrefix     = "host:"
	entityIDTagPrefix = "dd.internal.entity_id:"
	//nolint:revive // TODO(AML) Fix revive linter
	CardinalityTagPrefix = constants.CardinalityTagPrefix
	jmxCheckNamePrefix   = "dd.internal.jmx_check_name:"
)

// enrichConfig contains static parameters used in various enrichment
// procedures for metrics, events and service checks.
type enrichConfig struct {
	metricPrefix              string
	metricPrefixBlacklist     []string
	metricBlocklist           blocklist
	defaultHostname           string
	entityIDPrecedenceEnabled bool
	serverlessMode            bool
}

// extractTagsMetadata returns tags (client tags + host tag) and information needed to query tagger (origins, cardinality).
func extractTagsMetadata(tags []string, originFromUDS string, originFromMsg []byte, conf enrichConfig) ([]string, string, taggertypes.OriginInfo, metrics.MetricSource) {
	host := conf.defaultHostname
	metricSource := metrics.MetricSourceDogstatsd
	origin := taggertypes.OriginInfo{
		FromUDS:       originFromUDS,
		FromMsg:       string(originFromMsg),
		ProductOrigin: taggertypes.ProductOriginDogStatsD,
	}

	n := 0
	for _, tag := range tags {
		if strings.HasPrefix(tag, hostTagPrefix) {
			host = tag[len(hostTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, entityIDTagPrefix) {
			origin.FromTag = tag[len(entityIDTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, CardinalityTagPrefix) {
			origin.Cardinality = tag[len(CardinalityTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, jmxCheckNamePrefix) {
			checkName := tag[len(jmxCheckNamePrefix):]
			metricSource = metrics.JMXCheckNameToMetricSource(checkName)
			continue
		}
		tags[n] = tag
		n++
	}

	tags = tags[:n]

	return tags, host, origin, metricSource
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

func tsToFloatForSamples(ts time.Time) float64 {
	if ts.IsZero() { // avoid a conversion
		// for on-time samples, we don't want to write any value in there
		return 0.0
	}
	return float64(ts.Unix())
}

func enrichMetricSample(dest []metrics.MetricSample, ddSample dogstatsdMetricSample, origin string, listenerID string, conf enrichConfig) []metrics.MetricSample {
	metricName := ddSample.name
	tags, hostnameFromTags, extractedOrigin, metricSource := extractTagsMetadata(ddSample.tags, origin, ddSample.containerID, conf)

	if !isExcluded(metricName, conf.metricPrefix, conf.metricPrefixBlacklist) {
		metricName = conf.metricPrefix + metricName
	}

	if conf.metricBlocklist.test(metricName) {
		return []metrics.MetricSample{}
	}

	if conf.serverlessMode { // we don't want to set the host while running in serverless mode
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
					Host:       hostnameFromTags,
					Name:       metricName,
					Tags:       tags,
					Mtype:      mtype,
					Value:      ddSample.values[idx],
					SampleRate: ddSample.sampleRate,
					RawValue:   ddSample.setValue,
					Timestamp:  tsToFloatForSamples(ddSample.ts),
					OriginInfo: extractedOrigin,
					ListenerID: listenerID,
					Source:     metricSource,
				})
		}
		return dest
	}

	// only one value contained, simple append it
	return append(dest, metrics.MetricSample{
		Host:       hostnameFromTags,
		Name:       metricName,
		Tags:       tags,
		Mtype:      mtype,
		Value:      ddSample.value,
		SampleRate: ddSample.sampleRate,
		RawValue:   ddSample.setValue,
		Timestamp:  tsToFloatForSamples(ddSample.ts),
		OriginInfo: extractedOrigin,
		ListenerID: listenerID,
		Source:     metricSource,
	})
}

func enrichEventPriority(priority eventPriority) metricsevent.EventPriority {
	switch priority {
	case priorityNormal:
		return metricsevent.EventPriorityNormal
	case priorityLow:
		return metricsevent.EventPriorityLow
	}
	return metricsevent.EventPriorityNormal
}

func enrichEventAlertType(dogstatsdAlertType alertType) metricsevent.EventAlertType {
	switch dogstatsdAlertType {
	case alertTypeSuccess:
		return metricsevent.EventAlertTypeSuccess
	case alertTypeInfo:
		return metricsevent.EventAlertTypeInfo
	case alertTypeWarning:
		return metricsevent.EventAlertTypeWarning
	case alertTypeError:
		return metricsevent.EventAlertTypeError
	}
	return metricsevent.EventAlertTypeSuccess
}

func enrichEvent(event dogstatsdEvent, origin string, conf enrichConfig) *metricsevent.Event {
	tags, hostnameFromTags, extractedOrigin, _ := extractTagsMetadata(event.tags, origin, event.containerID, conf)

	enrichedEvent := &metricsevent.Event{
		Title:          event.title,
		Text:           event.text,
		Ts:             event.timestamp,
		Priority:       enrichEventPriority(event.priority),
		Tags:           tags,
		AlertType:      enrichEventAlertType(event.alertType),
		AggregationKey: event.aggregationKey,
		SourceTypeName: event.sourceType,
		OriginInfo:     extractedOrigin,
	}

	if len(event.hostname) != 0 {
		enrichedEvent.Host = event.hostname
	} else {
		enrichedEvent.Host = hostnameFromTags
	}
	return enrichedEvent
}

func enrichServiceCheckStatus(status serviceCheckStatus) servicecheck.ServiceCheckStatus {
	switch status {
	case serviceCheckStatusUnknown:
		return servicecheck.ServiceCheckUnknown
	case serviceCheckStatusOk:
		return servicecheck.ServiceCheckOK
	case serviceCheckStatusWarning:
		return servicecheck.ServiceCheckWarning
	case serviceCheckStatusCritical:
		return servicecheck.ServiceCheckCritical
	}
	return servicecheck.ServiceCheckUnknown
}

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, origin string, conf enrichConfig) *servicecheck.ServiceCheck {
	tags, hostnameFromTags, extractedOrigin, _ := extractTagsMetadata(serviceCheck.tags, origin, serviceCheck.containerID, conf)

	enrichedServiceCheck := &servicecheck.ServiceCheck{
		CheckName:  serviceCheck.name,
		Ts:         serviceCheck.timestamp,
		Status:     enrichServiceCheckStatus(serviceCheck.status),
		Message:    serviceCheck.message,
		Tags:       tags,
		OriginInfo: extractedOrigin,
	}

	if len(serviceCheck.hostname) != 0 {
		enrichedServiceCheck.Host = serviceCheck.hostname
	} else {
		enrichedServiceCheck.Host = hostnameFromTags
	}
	return enrichedServiceCheck
}
