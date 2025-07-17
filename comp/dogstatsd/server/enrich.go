// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

var (
	hostTagPrefix     = "host:"
	entityIDTagPrefix = "dd.internal.entity_id:"
	//nolint:revive // TODO(AML) Fix revive linter
	CardinalityTagPrefix = constants.CardinalityTagPrefix
	jmxCheckNamePrefix   = "dd.internal.jmx_check_name:"

	tlmBlockedPoints = telemetry.NewSimpleCounter("dogstatsd", "listener_blocked_points", "How many points were blocked")
)

// enrichConfig contains static parameters used in various enrichment
// procedures for metrics, events and service checks.
type enrichConfig struct {
	// TODO(remy): this metric prefix / prefix blocklist
	// is independent from the metric name blocklist, that's
	// confusing and should be merged in the same implemnetation instead.
	metricPrefix              string
	metricPrefixBlacklist     []string
	defaultHostname           string
	entityIDPrecedenceEnabled bool
	serverlessMode            bool
}

// extractTagsMetadata returns tags (client tags + host tag) and information needed to query tagger (origins, cardinality).
func extractTagsMetadata(tags []string, originFromUDS string, processID uint32, localData origindetection.LocalData, externalData origindetection.ExternalData, cardinality string, conf enrichConfig) ([]string, string, taggertypes.OriginInfo, metrics.MetricSource) {
	host := conf.defaultHostname
	metricSource := GetDefaultMetricSource()

	// Add Origin Detection metadata
	origin := taggertypes.OriginInfo{
		ContainerIDFromSocket: originFromUDS,
		LocalData:             localData,
		ExternalData:          externalData,
		ProductOrigin:         origindetection.ProductOriginDogStatsD,
		Cardinality:           cardinality,
	}
	origin.LocalData.ProcessID = processID

	n := 0
	for _, tag := range tags {
		if strings.HasPrefix(tag, hostTagPrefix) {
			host = tag[len(hostTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, entityIDTagPrefix) {
			origin.LocalData.PodUID = tag[len(entityIDTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, CardinalityTagPrefix) && origin.Cardinality == "" {
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

// serverlessSourceCustomToRuntime converts Serverless custom metric source to its corresponding runtime metric source
func serverlessSourceCustomToRuntime(metricSource metrics.MetricSource) metrics.MetricSource {
	switch metricSource {
	case metrics.MetricSourceAwsLambdaCustom:
		metricSource = metrics.MetricSourceAwsLambdaRuntime
	case metrics.MetricSourceAzureAppServiceCustom:
		metricSource = metrics.MetricSourceAzureAppServiceRuntime
	case metrics.MetricSourceAzureContainerAppCustom:
		metricSource = metrics.MetricSourceAzureContainerAppRuntime
	case metrics.MetricSourceGoogleCloudRunCustom:
		metricSource = metrics.MetricSourceGoogleCloudRunRuntime
	}
	return metricSource
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

func enrichMetricSample(dest []metrics.MetricSample, ddSample dogstatsdMetricSample, origin string, processID uint32, listenerID string, conf enrichConfig, blocklist *utilstrings.Blocklist) []metrics.MetricSample {
	metricName := ddSample.name
	tags, hostnameFromTags, extractedOrigin, metricSource := extractTagsMetadata(ddSample.tags, origin, processID, ddSample.localData, ddSample.externalData, ddSample.cardinality, conf)

	if !isExcluded(metricName, conf.metricPrefix, conf.metricPrefixBlacklist) {
		metricName = conf.metricPrefix + metricName
	}

	if blocklist != nil && blocklist.Test(metricName) {
		tlmBlockedPoints.Inc()
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

func enrichEventPriority(priority eventPriority) metricsevent.Priority {
	switch priority {
	case priorityNormal:
		return metricsevent.PriorityNormal
	case priorityLow:
		return metricsevent.PriorityLow
	}
	return metricsevent.PriorityNormal
}

func enrichEventAlertType(dogstatsdAlertType alertType) metricsevent.AlertType {
	switch dogstatsdAlertType {
	case alertTypeSuccess:
		return metricsevent.AlertTypeSuccess
	case alertTypeInfo:
		return metricsevent.AlertTypeInfo
	case alertTypeWarning:
		return metricsevent.AlertTypeWarning
	case alertTypeError:
		return metricsevent.AlertTypeError
	}
	return metricsevent.AlertTypeSuccess
}

func enrichEvent(event dogstatsdEvent, origin string, processID uint32, conf enrichConfig) *metricsevent.Event {
	tags, hostnameFromTags, extractedOrigin, _ := extractTagsMetadata(event.tags, origin, processID, event.localData, event.externalData, event.cardinality, conf)

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

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, origin string, processID uint32, conf enrichConfig) *servicecheck.ServiceCheck {
	tags, hostnameFromTags, extractedOrigin, _ := extractTagsMetadata(serviceCheck.tags, origin, processID, serviceCheck.localData, serviceCheck.externalData, serviceCheck.cardinality, conf)

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
