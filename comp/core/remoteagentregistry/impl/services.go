// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"google.golang.org/grpc"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	registryutil "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/util"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// emitterMetricTagName is the label added to all metrics forwarded from a remote agent
	// to identify which agent produced them. Its value is the registered sanitized display name.
	emitterMetricTagName = "emitter"
)

func (ra *remoteAgentRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetStatusDetailsResponse, error) {
		return remoteAgent.GetStatusDetails(ctx, &pb.GetStatusDetailsRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, in *pb.GetStatusDetailsResponse, err error) remoteagentregistry.StatusData {
		out := remoteagentregistry.StatusData{
			RegisteredAgent: details,
		}

		if err != nil {
			out.FailureReason = fmt.Sprintf("Failed to query for status: %v", err)
			return out
		}

		// converting main section
		if in.MainSection != nil {
			out.MainSection = in.MainSection.Fields
		}

		// converting named sections
		sections := make(map[string]remoteagentregistry.StatusSection, len(in.NamedSections))
		for name, section := range in.NamedSections {
			if section != nil {
				sections[name] = section.Fields
			}
		}
		out.NamedSections = sections

		return out
	}

	return callAgentsForService(ra, StatusServiceName, client, processor)
}

func (ra *remoteAgentRegistry) fillFlare(_ context.Context, builder flarebuilder.FlareBuilder) error {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetFlareFilesResponse, error) {
		return remoteAgent.GetFlareFiles(ctx, &pb.GetFlareFilesRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetFlareFilesResponse, err error) *remoteagentregistry.FlareData {
		if err != nil {
			// The remote agent is registered but unreachable (crashed, gRPC failure, timeout).
			// Surface the error as UNREACHABLE.txt without blocking the rest of the flare.
			log.Warnf("Remote agent %q could not be reached during flare collection: %v", details.DisplayName, err)
			return &remoteagentregistry.FlareData{
				RegisteredAgent: details,
				Files: map[string][]byte{
					"UNREACHABLE.txt": []byte(fmt.Sprintf("%s could not be reached: %v\n", details.DisplayName, err)),
				},
			}
		}
		return &remoteagentregistry.FlareData{
			RegisteredAgent: details,
			Files:           resp.Files,
		}
	}

	// We've collected all the flare data we can, so now we add it to the flare builder.
	for _, flareData := range callAgentsForService(ra, FlareServiceName, client, processor) {
		if flareData == nil {
			continue
		}

		for fileName, fileData := range flareData.Files {
			// The flare builder already logs errors, so we can ignore them here.
			// an error here should not prevent the flare from being created.
			//nolint:errcheck
			builder.AddFile(fmt.Sprintf("%s/%s", flareData.RegisteredAgent.SanitizedDisplayName, registryutil.SanitizeFileName(fileName)), fileData)
		}
	}

	return nil
}

type registryCollector struct {
	registry       *remoteAgentRegistry
	telemetryStore *telemetryStore
}

func newRegistryCollector(registry *remoteAgentRegistry, telemetryStore *telemetryStore) prometheus.Collector {
	return &registryCollector{
		registry:       registry,
		telemetryStore: telemetryStore,
	}
}

func (ra *remoteAgentRegistry) registerCollector() {
	ra.telemetry.RegisterCollector(newRegistryCollector(ra, ra.telemetryStore))
}

func (c *registryCollector) Describe(_ chan<- *prometheus.Desc) {
}

func (c *registryCollector) Collect(ch chan<- prometheus.Metric) {
	c.GetRegisteredAgentsTelemetry(ch)
}

func (c *registryCollector) GetRegisteredAgentsTelemetry(ch chan<- prometheus.Metric) {
	client := func(ctx context.Context, remoteAgent *remoteAgentClient, opts ...grpc.CallOption) (*pb.GetTelemetryResponse, error) {
		return remoteAgent.GetTelemetry(ctx, &pb.GetTelemetryRequest{}, opts...)
	}
	processor := func(details remoteagentregistry.RegisteredAgent, resp *pb.GetTelemetryResponse, err error) struct{} {
		if err != nil {
			log.Warnf("Failed to collect telemetry metrics from remoteAgent %v: %v", details.SanitizedDisplayName, err)
			return struct{}{}
		}
		if promText, ok := resp.Payload.(*pb.GetTelemetryResponse_PromText); ok {
			collectFromPromText(ch, promText.PromText, details.SanitizedDisplayName)
		}
		return struct{}{}
	}

	// We don't need to collect any value since everything is sent through the provided channel
	callAgentsForService(c.registry, TelemetryServiceName, client, processor)
}

// Retrieve the telemetry data in exposition format from the remote agent
func collectFromPromText(ch chan<- prometheus.Metric, promText string, remoteAgentName string) {
	parser := expfmt.NewTextParser(model.LegacyValidation)
	metricFamilies, err := parser.TextToMetricFamilies(strings.NewReader(promText))
	if err != nil {
		log.Warnf("Failed to parse prometheus text: %v", err)
		return
	}

	for _, mf := range metricFamilies {
		help := ""
		if mf.Help != nil {
			help = *mf.Help
		}

		metricType := mf.GetType()
		if metricType == dto.MetricType_SUMMARY {
			for _, metric := range mf.Metric {
				if metric != nil {
					log.Warnf("Dropping metrics %v from remoteAgent %v: unimplemented summary aggregation logic", mf.GetName(), remoteAgentName)
				}
			}
			continue
		}
		if metricType != dto.MetricType_COUNTER && metricType != dto.MetricType_GAUGE && metricType != dto.MetricType_HISTOGRAM {
			for _, metric := range mf.Metric {
				if metric != nil {
					log.Warnf("Dropping metrics %v from remoteAgent %v: unknown metric type %s", mf.GetName(), remoteAgentName, metricType)
				}
			}
			continue
		}

		for _, aggregate := range coalesceCanonicalMetrics(mf.Metric, metricType, mf.GetName(), remoteAgentName) {
			desc := prometheus.NewDesc(*mf.Name, help, aggregate.labelNames, nil)

			switch metricType {
			case dto.MetricType_COUNTER:
				metric, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, aggregate.value, aggregate.labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry counter metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric
			case dto.MetricType_GAUGE:
				metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, aggregate.value, aggregate.labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry gauge metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric
			case dto.MetricType_HISTOGRAM:
				metric, err := prometheus.NewConstHistogram(desc, aggregate.sampleCount, aggregate.sampleSum, aggregate.buckets, aggregate.labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry histogram metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric
			}
		}
	}
}

// canonicalMetricLabelsAndKey returns canonical labels and an order-independent, length-prefixed key.
func canonicalMetricLabelsAndKey(incoming []*dto.LabelPair, registeredEmitter string) ([]string, []string, string) {
	labelNames := make([]string, 0, len(incoming)+1)
	labelValues := make([]string, 0, len(incoming)+1)
	labels := make([]canonicalLabel, 0, len(incoming)+1)
	appendLabel := func(name, value string) {
		labelNames = append(labelNames, name)
		labelValues = append(labelValues, value)
		labels = append(labels, canonicalLabel{name: name, value: value})
	}

	appendLabel(emitterMetricTagName, registeredEmitter)
	for _, label := range incoming {
		if label.GetName() == emitterMetricTagName {
			continue
		}
		appendLabel(label.GetName(), label.GetValue())
	}

	sort.Slice(labels, func(i, j int) bool {
		return labels[i].name < labels[j].name ||
			(labels[i].name == labels[j].name && labels[i].value < labels[j].value)
	})

	var key strings.Builder
	for _, label := range labels {
		key.WriteString(strconv.Itoa(len(label.name)))
		key.WriteByte(':')
		key.WriteString(label.name)
		key.WriteString(strconv.Itoa(len(label.value)))
		key.WriteByte(':')
		key.WriteString(label.value)
	}
	return labelNames, labelValues, key.String()
}

type canonicalMetricAggregate struct {
	labelNames   []string
	labelValues  []string
	value        float64
	sampleCount  uint64
	sampleSum    float64
	bucketBounds []float64
	buckets      map[float64]uint64
}

type canonicalLabel struct {
	name  string
	value string
}

func coalesceCanonicalMetrics(metrics []*dto.Metric, metricType dto.MetricType, metricName, registeredEmitter string) []*canonicalMetricAggregate {
	aggregatesByLabels := make(map[string]*canonicalMetricAggregate, len(metrics))
	aggregates := make([]*canonicalMetricAggregate, 0, len(metrics))
	for _, metric := range metrics {
		if metric == nil {
			continue
		}

		labelNames, labelValues, key := canonicalMetricLabelsAndKey(metric.Label, registeredEmitter)
		aggregate, found := aggregatesByLabels[key]
		if !found {
			aggregate = &canonicalMetricAggregate{
				labelNames:  labelNames,
				labelValues: labelValues,
			}
			if metricType == dto.MetricType_HISTOGRAM {
				aggregate.bucketBounds = histogramBucketBounds(metric.GetHistogram())
				aggregate.buckets = make(map[float64]uint64)
			}
			aggregatesByLabels[key] = aggregate
			aggregates = append(aggregates, aggregate)
		}

		switch metricType {
		case dto.MetricType_COUNTER:
			aggregate.value += metric.GetCounter().GetValue()
		case dto.MetricType_GAUGE:
			aggregate.value += metric.GetGauge().GetValue()
		case dto.MetricType_HISTOGRAM:
			histogram := metric.GetHistogram()
			bucketBounds := histogramBucketBounds(histogram)
			if found && !equalHistogramBucketBounds(aggregate.bucketBounds, bucketBounds) {
				log.Warnf("Dropping colliding histogram metric %q from remote agent %q with incompatible bucket layout %v; keeping first layout %v", metricName, registeredEmitter, bucketBounds, aggregate.bucketBounds)
				continue
			}
			aggregate.sampleCount += histogram.GetSampleCount()
			aggregate.sampleSum += histogram.GetSampleSum()
			for _, bucket := range histogram.GetBucket() {
				aggregate.buckets[bucket.GetUpperBound()] += bucket.GetCumulativeCount()
			}
		}
	}
	return aggregates
}

func histogramBucketBounds(histogram *dto.Histogram) []float64 {
	bounds := make([]float64, 0, len(histogram.GetBucket()))
	for _, bucket := range histogram.GetBucket() {
		bounds = append(bounds, bucket.GetUpperBound())
	}
	return bounds
}

func equalHistogramBucketBounds(first, second []float64) bool {
	if len(first) != len(second) {
		return false
	}
	for i := range first {
		if first[i] != second[i] {
			return false
		}
	}
	return true
}
