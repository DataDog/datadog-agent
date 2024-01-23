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
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

var (
	hostTagPrefix       = "host:"
	entityIDTagPrefix   = "dd.internal.entity_id:"
	entityIDIgnoreValue = "none"
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
	originOptOutEnabled       bool
}

// extractTagsMetadata returns tags (client tags + host tag) and information needed to query tagger (origins, cardinality).
//
// The following tables explain how the origins are chosen.
// originFromUDS is the origin discovered via UDS origin detection (container ID).
// originFromTag is the origin sent by the client via the dd.internal.entity_id tag (non-prefixed pod uid).
// originFromMsg is the origin sent by the client via the container field (non-prefixed container ID).
// entityIDPrecedenceEnabled refers to the dogstatsd_entity_id_precedence parameter.
//
//	---------------------------------------------------------------------------------
//
// | originFromUDS | originFromTag | entityIDPrecedenceEnabled || Result: udsOrigin  |
// |---------------|---------------|---------------------------||--------------------|
// | any           | any           | false                     || originFromUDS      |
// | any           | any           | true                      || empty              |
// | any           | empty         | any                       || originFromUDS      |
//
//	---------------------------------------------------------------------------------
//
//	---------------------------------------------------------------------------------
//
// | originFromTag          | originFromMsg   || Result: originFromClient            |
// |------------------------|-----------------||-------------------------------------|
// | not empty && not none  | any             || pod prefix + originFromTag          |
// | empty                  | empty           || empty                               |
// | none                   | empty           || empty                               |
// | empty                  | not empty       || container prefix + originFromMsg    |
// | none                   | not empty       || container prefix + originFromMsg    |
//
//	---------------------------------------------------------------------------------
func extractTagsMetadata(tags []string, originFromUDS string, originFromMsg []byte, conf enrichConfig) ([]string, string, string, string, string, metrics.MetricSource) {
	host := conf.defaultHostname

	metricSource := metrics.MetricSourceDogstatsd
	n := 0
	originFromTag, cardinality := "", ""
	for _, tag := range tags {
		if strings.HasPrefix(tag, hostTagPrefix) {
			host = tag[len(hostTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, entityIDTagPrefix) {
			originFromTag = tag[len(entityIDTagPrefix):]
			continue
		} else if strings.HasPrefix(tag, CardinalityTagPrefix) {
			cardinality = tag[len(CardinalityTagPrefix):]
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

	udsOrigin := ""
	// We use the UDS socket origin if no origin ID was specify in the tags
	// or 'dogstatsd_entity_id_precedence' is set to False (default false).
	if originFromTag == "" || !conf.entityIDPrecedenceEnabled {
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

	if conf.originOptOutEnabled && cardinality == "none" {
		udsOrigin = ""
		originFromClient = ""
		cardinality = ""
	}

	return tags, host, udsOrigin, originFromClient, cardinality, metricSource
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
	tags, hostnameFromTags, udsOrigin, clientOrigin, cardinality, metricSource := extractTagsMetadata(ddSample.tags, origin, ddSample.containerID, conf)

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
					ListenerID:       listenerID,
					Cardinality:      cardinality,
					Source:           metricSource,
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
		ListenerID:       listenerID,
		Cardinality:      cardinality,
		Source:           metricSource,
	})
}

func enrichEventPriority(priority eventPriority) metricsevent.EventPriority {
	panic("not called")
}

func enrichEventAlertType(dogstatsdAlertType alertType) metricsevent.EventAlertType {
	panic("not called")
}

func enrichEvent(event dogstatsdEvent, origin string, conf enrichConfig) *metricsevent.Event {
	panic("not called")
}

func enrichServiceCheckStatus(status serviceCheckStatus) servicecheck.ServiceCheckStatus {
	panic("not called")
}

func enrichServiceCheck(serviceCheck dogstatsdServiceCheck, origin string, conf enrichConfig) *servicecheck.ServiceCheck {
	panic("not called")
}
