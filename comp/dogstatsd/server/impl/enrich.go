// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/constants"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server/internal/wire"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/metricname"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
)

var (
	hostTagPrefix        = "host:"
	entityIDTagPrefix    = "dd.internal.entity_id:"
	CardinalityTagPrefix = constants.CardinalityTagPrefix
	jmxCheckNamePrefix   = "dd.internal.jmx_check_name:"

	tlmFilteredPoints = telemetryimpl.GetCompatComponent().NewSimpleCounter("dogstatsd", "listener_filtered_points", "How many points were filtered out")
)

// enrichConfig contains static parameters used in various enrichment
// procedures for metrics, events and service checks.
type enrichConfig struct {
	// TODO(remy): this metric prefix / prefix filter list
	// is independent from the metric names filter list, that's
	// confusing and should be merged in the same implemnetation instead.
	metricPrefix              string
	metricPrefixBlacklist     []string
	defaultHostname           string
	entityIDPrecedenceEnabled bool
	serverlessMode            bool
	infraTagger               *infratags.MetricTagger
}

// extractTagsMetadata returns tags (client tags + host tag), information needed to query the tagger
// (origins, cardinality), and the JMX check name extracted from dd.internal.jmx_check_name (empty
// string if absent). The JMX check name is returned so callers can pass it directly to
// AppendJMXDogstatsdInfraTags without re-scanning the tag slice.
func extractTagsMetadata(tags []string, originFromUDS string, processID uint32, localData origindetection.LocalData, externalData origindetection.ExternalData, cardinality string, conf enrichConfig) ([]string, string, taggertypes.OriginInfo, metrics.MetricSource, string) {
	host := conf.defaultHostname
	metricSource := GetDefaultMetricSource()
	jmxCheckName := ""

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
		if after, ok := strings.CutPrefix(tag, hostTagPrefix); ok {
			host = after
			continue
		} else if after, ok := strings.CutPrefix(tag, entityIDTagPrefix); ok {
			origin.LocalData.PodUID = after
			continue
		} else if after, ok := strings.CutPrefix(tag, CardinalityTagPrefix); ok && origin.Cardinality == "" {
			origin.Cardinality = after
			continue
		} else if after, ok := strings.CutPrefix(tag, jmxCheckNamePrefix); ok {
			jmxCheckName = after
			metricSource = metrics.JMXCheckNameToMetricSource(jmxCheckName)
			continue
		}
		tags[n] = tag
		n++
	}

	tags = tags[:n]

	return tags, host, origin, metricSource, jmxCheckName
}

// serverlessSourceCustomToRuntime converts Serverless custom metric source to its corresponding runtime metric source
func serverlessSourceCustomToRuntime(metricSource metrics.MetricSource) metrics.MetricSource {
	switch metricSource {
	case metrics.MetricSourceAzureAppServiceCustom:
		metricSource = metrics.MetricSourceAzureAppServiceRuntime
	case metrics.MetricSourceAzureContainerAppCustom:
		metricSource = metrics.MetricSourceAzureContainerAppRuntime
	case metrics.MetricSourceGoogleCloudRunCustom:
		metricSource = metrics.MetricSourceGoogleCloudRunRuntime
	case metrics.MetricSourceAWSMicroVMCustom:
		metricSource = metrics.MetricSourceAWSMicroVMRuntime
	}
	return metricSource
}

func enrichMetricType(dogstatsdMetricType wire.MetricType) metrics.MetricType {
	switch dogstatsdMetricType {
	case wire.Gauge:
		return metrics.GaugeType
	case wire.Count:
		return metrics.CounterType
	case wire.Distribution:
		return metrics.DistributionType
	case wire.Histogram:
		return metrics.HistogramType
	case wire.Set:
		return metrics.SetType
	case wire.Timing:
		return metrics.HistogramType
	}
	return metrics.GaugeType
}

// unitFromMetricType returns the Datadog unit string for a dogstatsd metric type.
// Only wire.Timing carries an implicit unit ("millisecond"); all other types return an empty string.
func unitFromMetricType(mt wire.MetricType) string {
	if mt == wire.Timing {
		return metrics.UnitMilliseconds
	}
	return ""
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

func enrichMetricSample(dest []metrics.MetricSample, ddSample wire.MetricSample, origin string, processID uint32, listenerID string, conf enrichConfig, filterList *metricname.Matcher) []metrics.MetricSample {
	metricName := ddSample.Name
	tags, hostnameFromTags, extractedOrigin, metricSource, jmxCheckName := extractTagsMetadata(ddSample.Tags, origin, processID, ddSample.LocalData, ddSample.ExternalData, ddSample.Cardinality, conf)
	if conf.infraTagger.IsCheckEligible(jmxCheckName) {
		tags = conf.infraTagger.AppendTags(tags)
	}

	if !isExcluded(metricName, conf.metricPrefix, conf.metricPrefixBlacklist) {
		metricName = conf.metricPrefix + metricName
	}

	if filterList != nil && filterList.Test(metricName) {
		tlmFilteredPoints.Inc()
		return []metrics.MetricSample{}
	}

	if conf.serverlessMode { // we don't want to set the host while running in serverless mode
		hostnameFromTags = ""
	}

	mtype := enrichMetricType(ddSample.Type)

	// if 'ddSample.Values' contains values we're enriching a multi-value
	// dogstatsd message and will create a MetricSample per value. If not
	// we will use 'ddSample.Value'and return a single MetricSample
	unit := unitFromMetricType(ddSample.Type)

	if len(ddSample.Values) > 0 {
		for idx := range ddSample.Values {
			dest = append(dest,
				metrics.MetricSample{
					Host:       hostnameFromTags,
					Name:       metricName,
					Tags:       tags,
					Mtype:      mtype,
					Value:      ddSample.Values[idx],
					SampleRate: ddSample.SampleRate,
					RawValue:   ddSample.SetValue,
					Timestamp:  tsToFloatForSamples(ddSample.Timestamp),
					OriginInfo: extractedOrigin,
					ListenerID: listenerID,
					Source:     metricSource,
					Unit:       unit,
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
		Value:      ddSample.Value,
		SampleRate: ddSample.SampleRate,
		RawValue:   ddSample.SetValue,
		Timestamp:  tsToFloatForSamples(ddSample.Timestamp),
		OriginInfo: extractedOrigin,
		ListenerID: listenerID,
		Source:     metricSource,
		Unit:       unit,
	})
}

func enrichEventPriority(priority wire.EventPriority) metricsevent.Priority {
	switch priority {
	case wire.PriorityNormal:
		return metricsevent.PriorityNormal
	case wire.PriorityLow:
		return metricsevent.PriorityLow
	}
	return metricsevent.PriorityNormal
}

func enrichEventAlertType(dogstatsdAlertType wire.AlertType) metricsevent.AlertType {
	switch dogstatsdAlertType {
	case wire.AlertSuccess:
		return metricsevent.AlertTypeSuccess
	case wire.AlertInfo:
		return metricsevent.AlertTypeInfo
	case wire.AlertWarning:
		return metricsevent.AlertTypeWarning
	case wire.AlertError:
		return metricsevent.AlertTypeError
	}
	return metricsevent.AlertTypeSuccess
}

func enrichEvent(event wire.Event, origin string, processID uint32, conf enrichConfig) *metricsevent.Event {
	tags, hostnameFromTags, extractedOrigin, _, _ := extractTagsMetadata(event.Tags, origin, processID, event.LocalData, event.ExternalData, event.Cardinality, conf)

	enrichedEvent := &metricsevent.Event{
		Title:          event.Title,
		Text:           event.Text,
		Ts:             event.Timestamp,
		Priority:       enrichEventPriority(event.Priority),
		Tags:           tags,
		AlertType:      enrichEventAlertType(event.AlertType),
		AggregationKey: event.AggregationKey,
		SourceTypeName: event.SourceType,
		OriginInfo:     extractedOrigin,
	}

	if len(event.Hostname) != 0 {
		enrichedEvent.Host = event.Hostname
	} else {
		enrichedEvent.Host = hostnameFromTags
	}
	return enrichedEvent
}

func enrichServiceCheckStatus(status wire.ServiceCheckStatus) servicecheck.ServiceCheckStatus {
	switch status {
	case wire.ServiceCheckUnknown:
		return servicecheck.ServiceCheckUnknown
	case wire.ServiceCheckOK:
		return servicecheck.ServiceCheckOK
	case wire.ServiceCheckWarning:
		return servicecheck.ServiceCheckWarning
	case wire.ServiceCheckCritical:
		return servicecheck.ServiceCheckCritical
	}
	return servicecheck.ServiceCheckUnknown
}

func enrichServiceCheck(serviceCheck wire.ServiceCheck, origin string, processID uint32, conf enrichConfig) *servicecheck.ServiceCheck {
	tags, hostnameFromTags, extractedOrigin, _, _ := extractTagsMetadata(serviceCheck.Tags, origin, processID, serviceCheck.LocalData, serviceCheck.ExternalData, serviceCheck.Cardinality, conf)

	enrichedServiceCheck := &servicecheck.ServiceCheck{
		CheckName:  serviceCheck.Name,
		Ts:         serviceCheck.Timestamp,
		Status:     enrichServiceCheckStatus(serviceCheck.Status),
		Message:    serviceCheck.Message,
		Tags:       tags,
		OriginInfo: extractedOrigin,
	}

	if len(serviceCheck.Hostname) != 0 {
		enrichedServiceCheck.Host = serviceCheck.Hostname
	} else {
		enrichedServiceCheck.Host = hostnameFromTags
	}
	return enrichedServiceCheck
}
