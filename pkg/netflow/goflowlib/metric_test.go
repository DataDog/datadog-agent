// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/golang/protobuf/proto"
	promClient "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestConvertMetric(t *testing.T) {
	tests := []struct {
		name               string
		metric             *promClient.Metric
		metricFamily       *promClient.MetricFamily
		expectedMetricType metrics.MetricType
		expectedName       string
		expectedValue      float64
		expectedTags       []string
		expectedErr        string
	}{
		{
			name: "FEATURE ignore non allowed field",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("worker"), Value: proto.String("1")},
					{Name: proto.String("notAllowedField"), Value: proto.String("1")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "decoder.messages",
			expectedValue:      10.0,
			expectedTags:       []string{"worker:1"},
			expectedErr:        "",
		},
		{
			name: "FEATURE valueRemapper",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
					{Name: proto.String("worker"), Value: proto.String("1")},
					{Name: proto.String("notAllowedField"), Value: proto.String("1")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "decoder.messages",
			expectedValue:      10.0,
			expectedTags:       []string{"collector_type:netflow5", "worker:1"},
			expectedErr:        "",
		},
		{
			name: "FEATURE keyRemapper",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flows",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "FEATURE submit MonotonicCountType",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flows",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "FEATURE submit GaugeType",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_count"),
				Type: promClient.MetricType_GAUGE.Enum(),
			},
			metric: &promClient.Metric{
				Gauge: &promClient.Gauge{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.GaugeType,
			expectedName:       "processor.flows",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapCollectorType NetFlowV5",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
					{Name: proto.String("worker"), Value: proto.String("1")},
					{Name: proto.String("notAllowedField"), Value: proto.String("1")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "decoder.messages",
			expectedValue:      10.0,
			expectedTags:       []string{"collector_type:netflow5", "worker:1"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapCollectorType NetFlow",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("name"), Value: proto.String("NetFlow")},
					{Name: proto.String("worker"), Value: proto.String("1")},
					{Name: proto.String("notAllowedField"), Value: proto.String("1")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "decoder.messages",
			expectedValue:      10.0,
			expectedTags:       []string{"collector_type:netflow", "worker:1"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapFlowset DataFlowSet",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flowset_sum"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("type"), Value: proto.String("DataFlowSet")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flowsets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow", "type:data_flow_set"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapFlowset TemplateFlowSet",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flowset_sum"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("type"), Value: proto.String("TemplateFlowSet")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flowsets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow", "type:template_flow_set"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapFlowset OptionsTemplateFlowSet",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flowset_sum"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("type"), Value: proto.String("OptionsTemplateFlowSet")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flowsets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow", "type:options_template_flow_set"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapFlowset OptionsDataFlowSet",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flowset_sum"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("type"), Value: proto.String("OptionsDataFlowSet")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flowsets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow", "type:options_data_flow_set"},
			expectedErr:        "",
		},
		{
			name: "REMAPPER remapFlowset UNKNOWN",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flowset_sum"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("type"), Value: proto.String("UNKNOWN")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flowsets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "ERROR metric mapping not found",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_unknown_metric"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
				},
			},
			expectedMetricType: 0,
			expectedName:       "",
			expectedValue:      0,
			expectedTags:       nil,
			expectedErr:        "metric mapping not found for flow_unknown_metric",
		},
		{
			name: "ERROR metric mapping not found",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_UNTYPED.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
				},
			},
			expectedMetricType: 0,
			expectedName:       "",
			expectedValue:      0,
			expectedTags:       nil,
			expectedErr:        "metric type `UNTYPED` (3) not supported",
		},
		{
			name: "METRIC flow_decoder_count",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
					{Name: proto.String("worker"), Value: proto.String("1")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "decoder.messages",
			expectedValue:      10.0,
			expectedTags:       []string{"collector_type:netflow5", "worker:1"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_decoder_error_count",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_decoder_error_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
					{Name: proto.String("worker"), Value: proto.String("1")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "decoder.errors",
			expectedValue:      10.0,
			expectedTags:       []string{"collector_type:netflow5", "worker:1"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_nf_count",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flows",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_nf_flowset_sum",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flowset_sum"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("type"), Value: proto.String("DataFlowSet")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flowsets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow", "type:data_flow_set"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_nf_flows_missing",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flows_missing"),
				Type: promClient.MetricType_GAUGE.Enum(),
			},
			metric: &promClient.Metric{
				Gauge: &promClient.Gauge{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.GaugeType,
			expectedName:       "processor.flows_missing",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_nf_flows_sequence",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_flows_sequence"),
				Type: promClient.MetricType_GAUGE.Enum(),
			},
			metric: &promClient.Metric{
				Gauge: &promClient.Gauge{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.GaugeType,
			expectedName:       "processor.flows_sequence",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_nf_packets_missing",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_packets_missing"),
				Type: promClient.MetricType_GAUGE.Enum(),
			},
			metric: &promClient.Metric{
				Gauge: &promClient.Gauge{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("10")},
				},
			},
			expectedMetricType: metrics.GaugeType,
			expectedName:       "processor.packets_missing",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:10", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_nf_packets_sequence",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_nf_packets_sequence"),
				Type: promClient.MetricType_GAUGE.Enum(),
			},
			metric: &promClient.Metric{
				Gauge: &promClient.Gauge{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("10")},
				},
			},
			expectedMetricType: metrics.GaugeType,
			expectedName:       "processor.packets_sequence",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:10", "flow_protocol:netflow"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_traffic_bytes",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_traffic_bytes"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("remote_ip"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("local_port"), Value: proto.String("2000")},
					{Name: proto.String("type"), Value: proto.String("NetFlowV5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "traffic.bytes",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "listener_port:2000", "collector_type:netflow5"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_traffic_packets",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_traffic_packets"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("remote_ip"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("local_port"), Value: proto.String("2000")},
					{Name: proto.String("type"), Value: proto.String("NetFlowV5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "traffic.packets",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "listener_port:2000", "collector_type:netflow5"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_sf_count",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_sf_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("version"), Value: proto.String("5")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.flows",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "version:5", "flow_protocol:sflow"},
			expectedErr:        "",
		},
		{
			name: "METRIC flow_process_sf_errors_count",
			metricFamily: &promClient.MetricFamily{
				Name: proto.String("flow_process_sf_errors_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
			},
			metric: &promClient.Metric{
				Counter: &promClient.Counter{Value: proto.Float64(10)},
				Label: []*promClient.LabelPair{
					{Name: proto.String("router"), Value: proto.String("1.2.3.4")},
					{Name: proto.String("error"), Value: proto.String("some-error")},
				},
			},
			expectedMetricType: metrics.MonotonicCountType,
			expectedName:       "processor.errors",
			expectedValue:      10.0,
			expectedTags:       []string{"device_ip:1.2.3.4", "error:some-error", "flow_protocol:sflow"},
			expectedErr:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricType, name, value, tags, err := ConvertMetric(tt.metric, tt.metricFamily)
			assert.Equal(t, tt.expectedMetricType, metricType)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedValue, value)
			assert.ElementsMatch(t, tt.expectedTags, tags)
			if err != nil {
				assert.EqualError(t, err, tt.expectedErr)
			}
		})
	}
}
