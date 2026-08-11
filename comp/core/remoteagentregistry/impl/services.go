// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentregistryimpl implements the remoteagentregistry component interface
package remoteagentregistryimpl

import (
	"context"
	"fmt"
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

		for _, metric := range mf.Metric {
			if metric == nil {
				continue
			}

			labelNames, labelValues := canonicalMetricLabels(metric.Label, remoteAgentName)

			desc := prometheus.NewDesc(*mf.Name, help, labelNames, nil)

			switch *mf.Type {
			case dto.MetricType_COUNTER:
				value := *metric.Counter.Value

				metric, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, value, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry counter metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric
			case dto.MetricType_GAUGE:
				value := *metric.Gauge.Value

				metric, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, value, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry gauge metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric

			case dto.MetricType_SUMMARY:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unimplemented summary aggregation logic", mf.GetName(), remoteAgentName)
				continue

			case dto.MetricType_HISTOGRAM:
				count := metric.Histogram.GetSampleCount()
				sum := metric.Histogram.GetSampleSum()
				buckets := make(map[float64]uint64)
				for _, bucket := range metric.Histogram.GetBucket() {
					buckets[bucket.GetUpperBound()] = bucket.GetCumulativeCount()
				}

				metric, err := prometheus.NewConstHistogram(desc, count, sum, buckets, labelValues...)
				if err != nil {
					log.Warnf("Failed to collect telemetry histogram metric %v for remoteAgent %v: %v", mf.GetName(), remoteAgentName, err)
					continue
				}
				ch <- metric

			default:
				log.Warnf("Dropping metrics %v from remoteAgent %v: unknown metric type %s", mf.GetName(), remoteAgentName, mf.GetType())
			}
		}
	}
}

func canonicalMetricLabels(incoming []*dto.LabelPair, registeredEmitter string) ([]string, []string) {
	labelNames := make([]string, 0, len(incoming)+1)
	labelValues := make([]string, 0, len(incoming)+1)
	labelNames = append(labelNames, emitterMetricTagName)
	labelValues = append(labelValues, registeredEmitter)
	for _, label := range incoming {
		if label.GetName() == emitterMetricTagName {
			continue
		}
		labelNames = append(labelNames, label.GetName())
		labelValues = append(labelValues, label.GetValue())
	}
	return labelNames, labelValues
}
