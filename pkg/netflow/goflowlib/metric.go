// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	promClient "github.com/prometheus/client_model/go"
	"sort"
	"strings"
)

const metricPrefix = "datadog.netflow."

type remapperType func(string) string

type mappedMetric struct {
	name           string
	allowedTagKeys []string
	valueRemapper  map[string]remapperType
	keyRemapper    map[string]string
	extraTags      []string
}

func (m mappedMetric) isAllowedTagKey(tagKey string) bool {
	for _, allowedTagKey := range m.allowedTagKeys {
		if tagKey == allowedTagKey {
			return true
		}
	}
	return false
}

var collectorTypeMapper = map[string]string{
	"NetFlowV5": "netflow5",
	"NetFlow":   "netflow",
	"sFlow":     "sflow",
}

var flowsetMapper = map[string]string{
	"DataFlowSet":            "data_flow_set",
	"TemplateFlowSet":        "template_flow_set",
	"OptionsTemplateFlowSet": "options_template_flow_set",
	"OptionsDataFlowSet":     "options_data_flow_set",
}

// metricNameMapping maps goflow prometheus metrics to datadog netflow telemetry metrics
var metricNameMapping = map[string]mappedMetric{
	"flow_decoder_count": {
		name:           "decoder.messages",
		allowedTagKeys: []string{"name", "worker"},
		valueRemapper: map[string]remapperType{
			"name": remapCollectorType,
		},
		keyRemapper: map[string]string{
			"name": "collector_type",
		},
	},
	"flow_decoder_error_count": {
		name:           "decoder.errors",
		allowedTagKeys: []string{"name", "worker"},
		valueRemapper: map[string]remapperType{
			"name": remapCollectorType,
		},
		keyRemapper: map[string]string{
			"name": "collector_type",
		},
	},
	"flow_process_nf_count": {
		name:           "processor.flows",
		allowedTagKeys: []string{"router", "version"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:netflow"},
	},
	"flow_process_nf_flowset_sum": {
		name:           "processor.flowsets",
		allowedTagKeys: []string{"router", "type", "version"},
		valueRemapper: map[string]remapperType{
			"type": remapFlowset,
		},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:netflow"},
	},
	"flow_process_nf_flows_missing": {
		name:           "processor.flows_missing",
		allowedTagKeys: []string{"router", "version", "engine_id", "engine_type"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:netflow"},
	},
	"flow_process_nf_flows_sequence": {
		name:           "processor.flows_sequence",
		allowedTagKeys: []string{"router", "version", "engine_id", "engine_type"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:netflow"},
	},
	"flow_process_nf_packets_missing": {
		name:           "processor.packets_missing",
		allowedTagKeys: []string{"router", "version", "obs_domain_id"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:netflow"},
	},
	"flow_process_nf_packets_sequence": {
		name:           "processor.packets_sequence",
		allowedTagKeys: []string{"router", "version", "obs_domain_id"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:netflow"},
	},
	"flow_traffic_bytes": {
		name:           "traffic.bytes",
		allowedTagKeys: []string{"local_port", "remote_ip", "type"},
		keyRemapper: map[string]string{
			"local_port": "listener_port",
			"remote_ip":  "device_ip",
			"type":       "collector_type",
		},
		valueRemapper: map[string]remapperType{
			"type": remapCollectorType,
		},
	},
	"flow_traffic_packets": {
		name:           "traffic.packets",
		allowedTagKeys: []string{"local_port", "remote_ip", "type"},
		keyRemapper: map[string]string{
			"local_port": "listener_port",
			"remote_ip":  "device_ip",
			"type":       "collector_type",
		},
		valueRemapper: map[string]remapperType{
			"type": remapCollectorType,
		},
	},
	"flow_process_sf_count": {
		name:           "processor.flows",
		allowedTagKeys: []string{"router", "version"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:sflow"},
	},
	"flow_process_sf_errors_count": {
		name:           "processor.errors",
		allowedTagKeys: []string{"router", "error"},
		keyRemapper: map[string]string{
			"router": "device_ip",
		},
		extraTags: []string{"flow_protocol:sflow"},
	},
}

func remapCollectorType(goflowType string) string {
	return collectorTypeMapper[goflowType]
}

func remapFlowset(flowset string) string {
	return flowsetMapper[flowset]
}

// convertMetric converts prometheus metric to datadog compatible metrics
func convertMetric(metric *promClient.Metric, metricFamily *promClient.MetricFamily) (
	metrics.MetricType, // metric type
	string, // metric mame
	float64, // metric value
	[]string, // metric tags
	error,
) {
	var ddMetricType metrics.MetricType
	var floatValue float64
	var tags []string

	origMetricName := metricFamily.GetName()
	aMappedMetric, ok := metricNameMapping[origMetricName]
	if !ok {
		return 0, "", 0, nil, fmt.Errorf("metric mapping not found for %s", origMetricName)
	}

	promMetricType := metricFamily.GetType()
	switch promMetricType {
	case promClient.MetricType_COUNTER:
		floatValue = metric.GetCounter().GetValue()
		ddMetricType = metrics.MonotonicCountType
	case promClient.MetricType_GAUGE:
		floatValue = metric.GetGauge().GetValue()
		ddMetricType = metrics.GaugeType
	default:
		name := promClient.MetricType_name[int32(promMetricType)]
		return 0, "", 0, nil, fmt.Errorf("metric type `%s` (%d) not supported", name, promMetricType)
	}

	for _, labelPair := range metric.GetLabel() {
		tagKey := labelPair.GetName()

		// check is allowed tag key
		if !aMappedMetric.isAllowedTagKey(tagKey) {
			continue
		}

		tagVal := labelPair.GetValue()

		// remap metric Value
		valueRemapperFn, ok := aMappedMetric.valueRemapper[tagKey]
		if ok {
			tagVal = valueRemapperFn(tagVal)
		}

		// remap metric key
		newKey, ok := aMappedMetric.keyRemapper[tagKey]
		if ok {
			tagKey = newKey
		}

		if tagKey != "" && tagVal != "" {
			tags = append(tags, tagKey+":"+tagVal)
		}
	}
	if len(aMappedMetric.extraTags) > 0 {
		tags = append(tags, aMappedMetric.extraTags...)
	}
	return ddMetricType, aMappedMetric.name, floatValue, tags, nil
}

//var sequenceResetThreshold = map[string]uint{
//
//}

type MetricSample struct {
	MetricType metrics.MetricType
	Name       string
	Value      float64
	Tags       []string
}

type MetricConverter struct {
	lastMissingFlowsMetricValue map[string]float64
	lastSequence                map[string]float64
}

func (c MetricConverter) ConvertMetrics(promMetrics []*promClient.MetricFamily) []MetricSample {
	sequenceReset := make(map[string]bool)
	for _, metricFamily := range promMetrics {
		for _, metric := range metricFamily.Metric {
			log.Tracef("Collector metric `%s`: type=`%v` Value=`%v`, label=`%v`", metricFamily.GetName(), metricFamily.GetType().String(), metric.GetCounter().GetValue(), metric.GetLabel())
			metricType, name, value, tags, err := convertMetric(metric, metricFamily)
			if err != nil {
				log.Tracef("Error converting prometheus metric: %s", err)
				continue
			}
			switch metricType {
			case metrics.GaugeType:
				sort.Strings(tags)
				key := strings.Join(tags, ",")

				// TODO: factor
				if metricPrefix+name == "datadog.netflow.processor.flows_sequence" {
					if value-c.lastSequence[key] < -1000 {
						log.Debugf("[countMissing][agg] key=%s, seq=%f reset", key, c.lastSequence[key])
						sequenceReset[key] = true
					}
					c.lastSequence[key] = value
					log.Debugf("[countMissing][agg] key=%s,  seq=%f", key, c.lastSequence[key])
				}
				if metricPrefix+name == "datadog.netflow.processor.packets_sequence" {
					if value-c.lastSequence[key] < -100 {
						log.Debugf("[countMissing][agg] key=%s, seq=%f reset", key, c.lastSequence[key])
						sequenceReset[key] = true
					}
					c.lastSequence[key] = value
					log.Debugf("[countMissing][agg] key=%s, seq=%f", key, c.lastSequence[key])
				}
			}
		}
	}
	var samples []MetricSample
	for _, metricFamily := range promMetrics {
		for _, metric := range metricFamily.Metric {
			log.Tracef("Collector metric `%s`: type=`%v` Value=`%v`, label=`%v`", metricFamily.GetName(), metricFamily.GetType().String(), metric.GetCounter().GetValue(), metric.GetLabel())
			metricType, name, value, tags, err := convertMetric(metric, metricFamily)
			if err != nil {
				log.Tracef("Error converting prometheus metric: %s", err)
				continue
			}
			switch metricType {
			case metrics.GaugeType:
				samples = append(samples, MetricSample{
					MetricType: metrics.GaugeType,
					Name:       metricPrefix + name,
					Value:      value,
					Tags:       tags,
				})
				if metricPrefix+name == "datadog.netflow.processor.flows_missing" ||
					metricPrefix+name == "datadog.netflow.processor.packets_missing" {
					key := c.keyFromTags(tags)
					if sequenceReset[key] {
						c.lastMissingFlowsMetricValue[key] = 0
					}
					diff := value - c.lastMissingFlowsMetricValue[key]
					log.Debugf("[countMissing][agg] key=%s, last=%f, Value=%f, diff=%f, reset=%t", key, c.lastMissingFlowsMetricValue[key], value, diff, sequenceReset[key])
					c.lastMissingFlowsMetricValue[key] = value
					samples = append(samples, MetricSample{
						MetricType: metrics.GaugeType,
						Name:       metricPrefix + name + "_count",
						Value:      diff,
						Tags:       tags,
					})
				}
			case metrics.MonotonicCountType:
				samples = append(samples, MetricSample{
					MetricType: metrics.MonotonicCountType,
					Name:       metricPrefix + name,
					Value:      value,
					Tags:       tags,
				})
			default:
				log.Debugf("cannot submit unsupported type %s", metricType.String())
			}
		}
	}
	return samples
}

func (c MetricConverter) keyFromTags(tags []string) string {
	sortedTags := common.CopyStrings(tags)
	sort.Strings(sortedTags)
	key := strings.Join(sortedTags, ",")
	return key
}

func NewMetricConverter() *MetricConverter {
	return &MetricConverter{
		lastMissingFlowsMetricValue: make(map[string]float64),
		lastSequence:                make(map[string]float64),
	}
}
